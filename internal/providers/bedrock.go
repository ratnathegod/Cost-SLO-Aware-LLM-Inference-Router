package providers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type BedrockProvider struct {
	client  *bedrockruntime.Client
	modelID string
	// simplistic pricing table per 1k tokens
	pricePer1k map[string]float64
}

func NewBedrockProvider(modelID string, region string) (*BedrockProvider, error) {
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := bedrockruntime.NewFromConfig(awsCfg)
	return &BedrockProvider{
		client:  client,
		modelID: modelID,
		pricePer1k: map[string]float64{
			"anthropic.claude-3-sonnet": 3.00,
			"anthropic.claude-3-haiku":  0.25,
		},
	}, nil
}

func (p *BedrockProvider) Name() string { return "bedrock" }

func (p *BedrockProvider) CostPer1kTokensUSD(model string) float64 {
	if v, ok := p.pricePer1k[model]; ok {
		return v
	}
	return 3.0
}

func (p *BedrockProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, float64, int64, error) {
	// Using Bedrock InvokeModel with a minimal JSON body in Anthropic-compatible format
	// Note: Different model providers may require different bodies; this is a simplified example.
	payload := []byte(fmt.Sprintf(`{"anthropic_version":"bedrock-2023-05-31","messages":[{"role":"user","content":[{"type":"text","text":"%s"}]}]}`, req.Prompt))

	t0 := time.Now()
	out, err := p.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     &req.Model,
		ContentType: strPtr("application/json"),
		Body:        payload,
	})
	if err != nil {
		return CompletionResponse{}, 0, 0, err
	}
	// For brevity, avoid parsing the entire provider-specific response; assume text is in body string
	text := string(out.Body)
	lat := time.Since(t0).Milliseconds()
	return CompletionResponse{Text: text}, p.CostPer1kTokensUSD(req.Model) / 1000.0 * float64(max(req.MaxTok, 50)), lat, nil
}

func strPtr(s string) *string { return &s }
