package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/api"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/config"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/docs"

	// "github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/idempotency"
	// "github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/rate"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
	// "github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/usage"
)

func main() {
	// logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	zerolog.TimeFieldFormat = time.RFC3339

	// config
	cfg := config.Load()

	// validate configuration and log warnings
	warnings := config.ValidateConfig(cfg)
	for _, warning := range warnings {
		log.Warn().Msg(warning)
		if warning == "canary policy requires at least 2 providers, falling back to cheapest" {
			cfg.DefaultPolicy = "cheapest"
		}
	}

	// log effective configuration with secrets masked
	log.Info().Interface("config", cfg.MaskSecrets()).Msg("loaded configuration")

	// Initialize auth component first
	keyManager, err := auth.NewAPIKeyManager(cfg.DDBTenantsTable, cfg.TenantsJSONPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize API key manager")
	}

	// Temporarily disabled for debugging
	// usageStore, err := usage.NewStore(cfg.DDBUsageTable)
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("failed to initialize usage store")
	// }

	// idempotencyStore, err := idempotency.NewStore(cfg.DDBUsageTable)
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("failed to initialize idempotency store")
	// }

	// rateLimiter := rate.NewLimiter()
	// usageHandlers := api.NewUsageHandlers(usageStore)
	// tenantHandlers := api.NewTenantHandlers(keyManager, usageStore)

	r := chi.NewRouter()
	// Observability init
	telemetry.MustRegisterMetrics()
	if shutdown, err := telemetry.InitOTEL(context.Background(), "llm-router", cfg.OtelEndpoint); err != nil {
		log.Warn().Err(err).Msg("OTEL init failed")
	} else {
		defer func() {
			_ = shutdown(context.Background())
		}()
	}

	r.Use(telemetry.RequestIDMiddleware)
	r.Get("/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/v1/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ps := router.GetProviders()
		if len(ps) == 0 {
			http.Error(w, "no providers", http.StatusServiceUnavailable)
			return
		}
		// consider ready if any provider CB is not open
		for _, p := range ps {
			if p.CBStateValue() > 0 { // half-open or closed
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ready"))
				return
			}
		}
		http.Error(w, "all providers tripped", http.StatusServiceUnavailable)
	})
	r.Handle("/metrics", telemetry.MetricsHandler())

	// Test multi-tenant with just auth middleware
	if cfg.EnableUsageTracking || cfg.TenantsJSONPath != "" {
		r.Route("/v1", func(r chi.Router) {
			r.Use(keyManager.APIKeyMiddleware)
			r.Post("/infer", api.HandleInfer(cfg)) // Use basic handler for now
		})
	} else {
		r.Post("/v1/infer", api.HandleInfer(cfg))
	}

	// Documentation routes (public)
	r.Mount("/docs", docs.SwaggerUIHandler())

	// Admin API
	if cfg.AdminToken != "" {
		admin := chi.NewRouter()
		admin.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				const prefix = "Bearer "
				if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix || auth[len(prefix):] != cfg.AdminToken {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
			})
		})

		admin.Get("/status", api.HandleAdminStatus())

		admin.Get("/canary/status", api.HandleCanaryStatus())

		admin.Post("/canary/advance", api.HandleCanaryAdvance())

		admin.Post("/canary/rollback", api.HandleCanaryRollback())

		admin.Post("/policy", api.HandlePolicyUpdate())

		admin.Post("/providers/reload", api.HandleProvidersReload())

		// Tenant management endpoints disabled for debugging
		// admin.Post("/tenants", tenantHandlers.HandleCreateTenant())
		// admin.Get("/tenants/{tenant_id}/usage", tenantHandlers.HandleGetTenantUsage())

		r.Mount("/v1/admin", admin)
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	// graceful shutdown
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	log.Info().Msg("server stopped")
}
