package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog/log"
)

// UsageRecord represents a single API usage record
type UsageRecord struct {
	TenantID            string    `json:"tenant_id" dynamodbav:"tenant_id"`
	Timestamp           time.Time `json:"timestamp" dynamodbav:"timestamp"`
	RequestID           string    `json:"request_id" dynamodbav:"request_id"`
	Provider            string    `json:"provider" dynamodbav:"provider"`
	Model               string    `json:"model" dynamodbav:"model"`
	EstPromptTokens     int64     `json:"est_prompt_tokens" dynamodbav:"est_prompt_tokens"`
	EstCompletionTokens int64     `json:"est_completion_tokens" dynamodbav:"est_completion_tokens"`
	CostUSD             float64   `json:"cost_usd" dynamodbav:"cost_usd"`
	LatencyMs           int64     `json:"latency_ms" dynamodbav:"latency_ms"`
	Status              string    `json:"status" dynamodbav:"status"` // "ok" or "error"
	IdempotencyKey      string    `json:"idempotency_key,omitempty" dynamodbav:"idempotency_key,omitempty"`
}

// DailyAggregate represents daily usage aggregates
type DailyAggregate struct {
	TenantID  string    `json:"tenant_id" dynamodbav:"tenant_id"`
	Date      string    `json:"date" dynamodbav:"date"` // YYYY-MM-DD
	Requests  int64     `json:"requests" dynamodbav:"requests"`
	Successes int64     `json:"successes" dynamodbav:"successes"`
	Failures  int64     `json:"failures" dynamodbav:"failures"`
	TokensIn  int64     `json:"tokens_in" dynamodbav:"tokens_in"`
	TokensOut int64     `json:"tokens_out" dynamodbav:"tokens_out"`
	CostUSD   float64   `json:"cost_usd" dynamodbav:"cost_usd"`
	UpdatedAt time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// Store handles usage tracking and aggregation
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
			log.Warn().Err(err).Msg("failed to load AWS config, disabling usage tracking")
			store.enabled = false
		} else {
			store.ddbClient = dynamodb.NewFromConfig(cfg)
		}
	}

	return store, nil
}

// RecordUsage records a single usage event and updates daily aggregates
func (s *Store) RecordUsage(ctx context.Context, record UsageRecord) error {
	if !s.enabled {
		return nil // Silently skip if not enabled
	}

	// Record detailed usage
	if err := s.writeUsageRecord(ctx, record); err != nil {
		log.Error().Err(err).Msg("failed to write usage record")
		return err
	}

	// Update daily aggregate
	if err := s.updateDailyAggregate(ctx, record); err != nil {
		log.Error().Err(err).Msg("failed to update daily aggregate")
		// Don't return error here - detailed record was successful
	}

	return nil
}

func (s *Store) writeUsageRecord(ctx context.Context, record UsageRecord) error {
	// Create composite sort key: YYYY-MM-DD#HH:mm:ss#<req_id>
	sortKey := record.Timestamp.Format("2006-01-02#15:04:05") + "#" + record.RequestID

	item := map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "usage#" + record.TenantID},
		"sk": &types.AttributeValueMemberS{Value: sortKey},
	}

	// Marshal the record
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

func (s *Store) updateDailyAggregate(ctx context.Context, record UsageRecord) error {
	date := record.Timestamp.Format("2006-01-02")

	updateExpr := "SET #reqs = if_not_exists(#reqs, :zero) + :one, " +
		"#updated = :now, " +
		"#tokens_in = if_not_exists(#tokens_in, :zero) + :prompt_tokens, " +
		"#tokens_out = if_not_exists(#tokens_out, :zero) + :completion_tokens, " +
		"#cost = if_not_exists(#cost, :zero_float) + :cost"

	if record.Status == "ok" {
		updateExpr += ", #successes = if_not_exists(#successes, :zero) + :one"
	} else {
		updateExpr += ", #failures = if_not_exists(#failures, :zero) + :one"
	}

	expressionAttributeNames := map[string]string{
		"#reqs":       "requests",
		"#successes":  "successes",
		"#failures":   "failures",
		"#tokens_in":  "tokens_in",
		"#tokens_out": "tokens_out",
		"#cost":       "cost_usd",
		"#updated":    "updated_at",
	}

	expressionAttributeValues := map[string]types.AttributeValue{
		":zero":              &types.AttributeValueMemberN{Value: "0"},
		":zero_float":        &types.AttributeValueMemberN{Value: "0.0"},
		":one":               &types.AttributeValueMemberN{Value: "1"},
		":prompt_tokens":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", record.EstPromptTokens)},
		":completion_tokens": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", record.EstCompletionTokens)},
		":cost":              &types.AttributeValueMemberN{Value: fmt.Sprintf("%.6f", record.CostUSD)},
		":now":               &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	_, err := s.ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "agg#" + record.TenantID + "#daily"},
			"sk": &types.AttributeValueMemberS{Value: date},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	})

	return err
}

// GetDailyUsage retrieves daily usage aggregates for a tenant
func (s *Store) GetDailyUsage(ctx context.Context, tenantID string, since, until time.Time) ([]DailyAggregate, error) {
	if !s.enabled {
		return nil, nil
	}

	sinceStr := since.Format("2006-01-02")
	untilStr := until.Format("2006-01-02")

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("pk = :pk AND sk BETWEEN :since AND :until"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "agg#" + tenantID + "#daily"},
			":since": &types.AttributeValueMemberS{Value: sinceStr},
			":until": &types.AttributeValueMemberS{Value: untilStr},
		},
	}

	result, err := s.ddbClient.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	var aggregates []DailyAggregate
	for _, item := range result.Items {
		var agg DailyAggregate
		if err := attributevalue.UnmarshalMap(item, &agg); err != nil {
			log.Warn().Err(err).Msg("failed to unmarshal daily aggregate")
			continue
		}

		// Extract date from sk
		if sk, ok := item["sk"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				agg.Date = skStr.Value
			}
		}

		aggregates = append(aggregates, agg)
	}

	return aggregates, nil
}

// GetRecentUsage retrieves recent usage records for a tenant
func (s *Store) GetRecentUsage(ctx context.Context, tenantID string, limit int) ([]UsageRecord, error) {
	if !s.enabled {
		return nil, nil
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "usage#" + tenantID},
		},
		ScanIndexForward: aws.Bool(false), // Descending order (newest first)
		Limit:            aws.Int32(int32(limit)),
	}

	result, err := s.ddbClient.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	var records []UsageRecord
	for _, item := range result.Items {
		var record UsageRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			log.Warn().Err(err).Msg("failed to unmarshal usage record")
			continue
		}
		records = append(records, record)
	}

	return records, nil
}
