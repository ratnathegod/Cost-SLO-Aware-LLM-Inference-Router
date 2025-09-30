package idempotency

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/rs/zerolog/log"
)

const MaxResponseSize = 32 * 1024 // 32KB max response size

// IdempotencyRecord represents a stored idempotency record
type IdempotencyRecord struct {
	TenantID       string    `json:"tenant_id" dynamodbav:"tenant_id"`
	IdempotencyKey string    `json:"idempotency_key" dynamodbav:"idempotency_key"`
	Status         int       `json:"status" dynamodbav:"status"`
	ResponseHash   string    `json:"response_hash" dynamodbav:"response_hash"`
	ResponseBody   string    `json:"response_body" dynamodbav:"response_body"`
	CreatedAt      time.Time `json:"created_at" dynamodbav:"created_at"`
	TTL            int64     `json:"ttl" dynamodbav:"ttl"`
}

// Store handles idempotency key storage and retrieval
type Store struct {
	ddbClient *dynamodb.Client
	tableName string
	enabled   bool
}

func NewStore(tableName string) (*Store, error) {
	store := &Store{
		tableName: tableName,
		enabled:   tableName != "",
	}

	if store.enabled {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Warn().Err(err).Msg("failed to load AWS config, disabling idempotency")
			store.enabled = false
		} else {
			store.ddbClient = dynamodb.NewFromConfig(cfg)
		}
	}

	return store, nil
}

// GetRecord retrieves an existing idempotency record
func (s *Store) GetRecord(ctx context.Context, tenantID, idempotencyKey string) (*IdempotencyRecord, error) {
	if !s.enabled {
		return nil, nil
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "idem#" + tenantID},
			"sk": &types.AttributeValueMemberS{Value: idempotencyKey},
		},
	}

	result, err := s.ddbClient.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil // Not found
	}

	var record IdempotencyRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, err
	}

	return &record, nil
}

// StoreRecord saves an idempotency record
func (s *Store) StoreRecord(ctx context.Context, record IdempotencyRecord) error {
	if !s.enabled {
		return nil
	}

	item := map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "idem#" + record.TenantID},
		"sk": &types.AttributeValueMemberS{Value: record.IdempotencyKey},
	}

	recordMap, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}

	// Merge with pk/sk
	for k, v := range recordMap {
		item[k] = v
	}

	_, err = s.ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// ResponseRecorder captures HTTP responses for idempotency
type ResponseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func NewResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (r *ResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *ResponseRecorder) Write(data []byte) (int, error) {
	if len(r.body)+len(data) <= MaxResponseSize {
		r.body = append(r.body, data...)
	}
	return r.ResponseWriter.Write(data)
}

func (r *ResponseRecorder) Status() int {
	return r.statusCode
}

func (r *ResponseRecorder) Body() []byte {
	return r.body
}

func (r *ResponseRecorder) Hash() string {
	h := sha256.New()
	h.Write(r.body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Middleware provides idempotency support for HTTP handlers
func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only apply to methods that should be idempotent
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		tenant, ok := auth.GetTenantFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		// Check for existing record
		existing, err := s.GetRecord(r.Context(), tenant.TenantID, idempotencyKey)
		if err != nil {
			log.Error().Err(err).Msg("failed to check idempotency record")
			next.ServeHTTP(w, r)
			return
		}

		if existing != nil {
			// Return cached response
			s.replayResponse(w, r, existing)
			return
		}

		// Record new request
		recorder := NewResponseRecorder(w)
		next.ServeHTTP(recorder, r)

		// Store the response
		now := time.Now()
		record := IdempotencyRecord{
			TenantID:       tenant.TenantID,
			IdempotencyKey: idempotencyKey,
			Status:         recorder.Status(),
			ResponseHash:   recorder.Hash(),
			ResponseBody:   string(recorder.Body()),
			CreatedAt:      now,
			TTL:            now.Add(24 * time.Hour).Unix(), // 24 hour TTL
		}

		if err := s.StoreRecord(r.Context(), record); err != nil {
			log.Error().Err(err).Msg("failed to store idempotency record")
		}
	})
}

func (s *Store) replayResponse(w http.ResponseWriter, r *http.Request, record *IdempotencyRecord) {
	// Copy headers from the original response if stored (simplified for now)
	w.Header().Set("Content-Type", "application/json")

	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	w.Header().Set("X-Idempotency-Replay", "true")

	w.WriteHeader(record.Status)

	if record.ResponseBody != "" {
		_, _ = w.Write([]byte(record.ResponseBody))
	}
}
