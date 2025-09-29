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
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/config"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
)

func main() {
	// logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	zerolog.TimeFieldFormat = time.RFC3339

	// config
	cfg := config.Load()

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
	r.Post("/v1/infer", api.HandleInfer(cfg))

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
