package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog/log"
)

// Tenant represents a tenant record
type Tenant struct {
	TenantID        string    `json:"tenant_id" dynamodbav:"tenant_id"`
	Name            string    `json:"name" dynamodbav:"name"`
	APIKeyHash      string    `json:"-" dynamodbav:"api_key_hash"` // Never exposed in JSON
	Salt            string    `json:"-" dynamodbav:"salt"`         // Never exposed in JSON
	Plan            string    `json:"plan" dynamodbav:"plan"`
	RPSLimit        int       `json:"rps_limit" dynamodbav:"rps_limit"`
	DailyTokenLimit int64     `json:"daily_token_limit" dynamodbav:"daily_token_limit"`
	Enabled         bool      `json:"enabled" dynamodbav:"enabled"`
	CreatedAt       time.Time `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// TenantCache provides LRU caching for tenant lookups
type TenantCache struct {
	mu       sync.RWMutex
	cache    map[string]*Tenant
	accessed map[string]time.Time
	ttl      time.Duration
	maxSize  int
}

func NewTenantCache(ttl time.Duration, maxSize int) *TenantCache {
	return &TenantCache{
		cache:    make(map[string]*Tenant),
		accessed: make(map[string]time.Time),
		ttl:      ttl,
		maxSize:  maxSize,
	}
}

func (tc *TenantCache) Get(keyHash string) (*Tenant, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	tenant, exists := tc.cache[keyHash]
	if !exists {
		return nil, false
	}

	if time.Since(tc.accessed[keyHash]) > tc.ttl {
		// Expired, but don't delete here - let cleanup handle it
		return nil, false
	}

	return tenant, true
}

func (tc *TenantCache) Put(keyHash string, tenant *Tenant) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Simple eviction: if over maxSize, remove oldest
	if len(tc.cache) >= tc.maxSize {
		var oldestKey string
		var oldestTime time.Time

		for k, t := range tc.accessed {
			if oldestKey == "" || t.Before(oldestTime) {
				oldestKey = k
				oldestTime = t
			}
		}

		if oldestKey != "" {
			delete(tc.cache, oldestKey)
			delete(tc.accessed, oldestKey)
		}
	}

	tc.cache[keyHash] = tenant
	tc.accessed[keyHash] = time.Now()
}

func (tc *TenantCache) Delete(keyHash string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	delete(tc.cache, keyHash)
	delete(tc.accessed, keyHash)
}

// APIKeyManager handles tenant authentication
type APIKeyManager struct {
	ddbClient   *dynamodb.Client
	tableName   string
	cache       *TenantCache
	fallbackMap map[string]*Tenant
	mu          sync.RWMutex
}

func NewAPIKeyManager(tableName, tenantsJSONPath string) (*APIKeyManager, error) {
	mgr := &APIKeyManager{
		tableName:   tableName,
		cache:       NewTenantCache(60*time.Second, 1000),
		fallbackMap: make(map[string]*Tenant),
	}

	// Initialize DDB client if table name is provided
	if tableName != "" {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Warn().Err(err).Msg("failed to load AWS config, using in-memory fallback")
		} else {
			mgr.ddbClient = dynamodb.NewFromConfig(cfg)
		}
	}

	// Load fallback tenants from JSON if path provided
	if tenantsJSONPath != "" {
		if err := mgr.loadTenantsFromJSON(tenantsJSONPath); err != nil {
			log.Warn().Err(err).Str("path", tenantsJSONPath).Msg("failed to load tenants JSON")
		}
	}

	return mgr, nil
}

func (mgr *APIKeyManager) loadTenantsFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var tenants []Tenant
	if err := json.Unmarshal(data, &tenants); err != nil {
		return err
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	for _, tenant := range tenants {
		// Assume the JSON contains plaintext API keys that need to be hashed
		if tenant.APIKeyHash == "" && tenant.Salt == "" {
			continue // Skip invalid entries
		}
		mgr.fallbackMap[tenant.APIKeyHash] = &tenant
	}

	log.Info().Int("count", len(tenants)).Msg("loaded tenants from JSON")
	return nil
}

// HashAPIKey creates a hash of the API key with a random salt
func HashAPIKey(apiKey, salt string) string {
	h := sha256.New()
	h.Write([]byte(apiKey + salt))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// GenerateAPIKey creates a new random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateSalt creates a random salt for hashing
func GenerateSalt() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ValidateAPIKey checks if an API key is valid and returns the tenant
func (mgr *APIKeyManager) ValidateAPIKey(ctx context.Context, apiKey string) (*Tenant, error) {
	// Try cache first with all possible hashes (we don't know the salt yet)
	// This is inefficient, so we'll need to store by a different key

	// For now, let's hash with empty salt and check fallback first
	keyHash := HashAPIKey(apiKey, "")

	// Check fallback map first
	mgr.mu.RLock()
	if tenant, exists := mgr.fallbackMap[keyHash]; exists {
		mgr.mu.RUnlock()
		return tenant, nil
	}
	mgr.mu.RUnlock()

	// If DDB is available, we need to scan or use a GSI
	// For now, implement a simple scan (not efficient for production)
	if mgr.ddbClient != nil {
		return mgr.validateAPIKeyFromDDB(ctx, apiKey)
	}

	return nil, fmt.Errorf("invalid API key")
}

func (mgr *APIKeyManager) validateAPIKeyFromDDB(ctx context.Context, apiKey string) (*Tenant, error) {
	// This is a simplified implementation - in production you'd want a GSI on api_key_hash
	input := &dynamodb.ScanInput{
		TableName: aws.String(mgr.tableName),
	}

	result, err := mgr.ddbClient.Scan(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, item := range result.Items {
		var tenant Tenant
		if err := attributevalue.UnmarshalMap(item, &tenant); err != nil {
			continue
		}

		// Check if the API key matches this tenant's hash
		expectedHash := HashAPIKey(apiKey, tenant.Salt)
		if expectedHash == tenant.APIKeyHash && tenant.Enabled {
			// Cache the result
			mgr.cache.Put(tenant.APIKeyHash, &tenant)
			return &tenant, nil
		}
	}

	return nil, fmt.Errorf("invalid API key")
}

// CreateTenant creates a new tenant with a generated API key
func (mgr *APIKeyManager) CreateTenant(ctx context.Context, name, plan string, rpsLimit int, dailyTokenLimit int64) (*Tenant, string, error) {
	tenantID := fmt.Sprintf("tenant_%d", time.Now().Unix())
	apiKey, err := GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}

	salt, err := GenerateSalt()
	if err != nil {
		return nil, "", err
	}

	keyHash := HashAPIKey(apiKey, salt)
	now := time.Now()

	tenant := &Tenant{
		TenantID:        tenantID,
		Name:            name,
		APIKeyHash:      keyHash,
		Salt:            salt,
		Plan:            plan,
		RPSLimit:        rpsLimit,
		DailyTokenLimit: dailyTokenLimit,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Store in DDB if available
	if mgr.ddbClient != nil {
		item, err := attributevalue.MarshalMap(tenant)
		if err != nil {
			return nil, "", err
		}

		// Add sort key for DDB structure
		item["sk"] = &types.AttributeValueMemberS{Value: "meta"}

		_, err = mgr.ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(mgr.tableName),
			Item:      item,
		})
		if err != nil {
			return nil, "", err
		}
	} else {
		// Store in fallback map
		mgr.mu.Lock()
		mgr.fallbackMap[keyHash] = tenant
		mgr.mu.Unlock()
	}

	return tenant, apiKey, nil
}

// APIKeyMiddleware provides authentication for API requests
func (mgr *APIKeyManager) APIKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			mgr.writeErrorResponse(w, r, http.StatusUnauthorized, "missing_api_key", "API key required", "X-API-Key header is required")
			return
		}

		tenant, err := mgr.ValidateAPIKey(r.Context(), apiKey)
		if err != nil {
			mgr.writeErrorResponse(w, r, http.StatusUnauthorized, "invalid_api_key", "Invalid API key", "The provided API key is not valid")
			return
		}

		if !tenant.Enabled {
			mgr.writeErrorResponse(w, r, http.StatusUnauthorized, "tenant_disabled", "Tenant disabled", "The tenant account is disabled")
			return
		}

		// Add tenant to request context
		ctx := context.WithValue(r.Context(), "tenant", tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (mgr *APIKeyManager) writeErrorResponse(w http.ResponseWriter, r *http.Request, status int, errorType, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	w.WriteHeader(status)

	response := map[string]interface{}{
		"type":   fmt.Sprintf("https://example.com/errors/%s", errorType),
		"title":  title,
		"detail": detail,
		"status": status,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// GetTenantFromContext extracts tenant from request context
func GetTenantFromContext(ctx context.Context) (*Tenant, bool) {
	tenant, ok := ctx.Value("tenant").(*Tenant)
	return tenant, ok
}
