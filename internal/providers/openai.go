package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	// simple pricing map USD per 1k tokens, can be extended per model
	pricePer1k map[string]float64
}

func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1/chat/completions",
		client:  &http.Client{Timeout: 60 * time.Second},
		pricePer1k: map[string]float64{
			"gpt-4o":      5.00,
			"gpt-4o-mini": 0.60,
			"gpt-4.1":     10.00,
		},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) CostPer1kTokensUSD(model string) float64 {
	if v, ok := p.pricePer1k[model]; ok {
		return v
	}
	return 10.0 // fallback
}

type openaiReq struct {
	Model    string      `json:"model"`
	Messages []oaMessage `json:"messages"`
	MaxTok   int         `json:"max_tokens,omitempty"`
	Stream   bool        `json:"stream,omitempty"`
}
type oaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type openaiResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, float64, int64, error) {
	body := openaiReq{
		Model: req.Model,
		Messages: []oaMessage{
			{Role: "user", Content: req.Prompt},
		},
	}
	if req.MaxTok > 0 {
		body.MaxTok = req.MaxTok
	}

	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(b))
	if err != nil {
		return CompletionResponse{}, 0, 0, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	t0 := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CompletionResponse{}, 0, 0, fmt.Errorf("openai status %d", resp.StatusCode)
	}
	var or openaiResp
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return CompletionResponse{}, 0, 0, err
	}
	text := ""
	if len(or.Choices) > 0 {
		text = or.Choices[0].Message.Content
	}
	lat := time.Since(t0).Milliseconds()
	// We don't precisely know token count here; use list price per 1k as rough estimate for policy purposes
	return CompletionResponse{Text: text}, p.CostPer1kTokensUSD(req.Model) / 1000.0 * float64(max(req.MaxTok, 50)), lat, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
