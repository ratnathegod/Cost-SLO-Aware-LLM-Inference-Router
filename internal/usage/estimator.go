package usage

import (
	"strings"
	"unicode/utf8"
)

// TokenEstimator provides token counting estimates for different models
type TokenEstimator struct {
	modelRates map[string]float64 // model -> chars per token ratio
}

func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{
		modelRates: map[string]float64{
			// OpenAI models (approximate)
			"gpt-4":         3.5, // ~3.5 chars per token
			"gpt-4o":        3.5,
			"gpt-3.5":       4.0, // ~4 chars per token
			"gpt-3.5-turbo": 4.0,

			// Anthropic Claude models
			"claude-3":        3.8,
			"claude-3-haiku":  3.8,
			"claude-3-sonnet": 3.8,
			"claude-3-opus":   3.8,

			// Default fallback
			"default": 3.8,
		},
	}
}

// EstimateTokens estimates the number of tokens in text for a given model
func (te *TokenEstimator) EstimateTokens(text, model string) int64 {
	if text == "" {
		return 0
	}

	// Get the chars-per-token ratio for the model
	rate, exists := te.modelRates[model]
	if !exists {
		// Try to match by prefix
		for modelPrefix, modelRate := range te.modelRates {
			if strings.HasPrefix(model, modelPrefix) {
				rate = modelRate
				exists = true
				break
			}
		}

		if !exists {
			rate = te.modelRates["default"]
		}
	}

	// Count characters (UTF-8 aware)
	charCount := float64(utf8.RuneCountInString(text))

	// Estimate tokens
	estimatedTokens := charCount / rate

	// Round up to ensure we don't underestimate
	if estimatedTokens != float64(int64(estimatedTokens)) {
		estimatedTokens = float64(int64(estimatedTokens) + 1)
	}

	return int64(estimatedTokens)
}

// EstimatePromptTokens estimates tokens in a prompt
func (te *TokenEstimator) EstimatePromptTokens(prompt, model string) int64 {
	// Add some overhead for system messages, formatting, etc.
	baseTokens := te.EstimateTokens(prompt, model)
	overhead := int64(float64(baseTokens) * 0.1) // 10% overhead
	return baseTokens + overhead
}

// EstimateCompletionTokens provides a rough estimate for completion tokens
// This is much harder to predict accurately, so we use conservative estimates
func (te *TokenEstimator) EstimateCompletionTokens(prompt, model string) int64 {
	promptTokens := te.EstimateTokens(prompt, model)

	// Use heuristics based on prompt length
	var estimatedCompletion int64

	switch {
	case promptTokens < 100:
		// Short prompts typically get short responses
		estimatedCompletion = promptTokens / 2
	case promptTokens < 500:
		// Medium prompts
		estimatedCompletion = promptTokens / 3
	case promptTokens < 2000:
		// Long prompts
		estimatedCompletion = promptTokens / 4
	default:
		// Very long prompts
		estimatedCompletion = promptTokens / 8
	}

	// Ensure minimum estimation
	if estimatedCompletion < 10 {
		estimatedCompletion = 10
	}

	return estimatedCompletion
}

// EstimateMaxTokens estimates the maximum tokens for a request (for pre-flight checks)
func (te *TokenEstimator) EstimateMaxTokens(prompt, model string, maxTokens int) int64 {
	promptTokens := te.EstimatePromptTokens(prompt, model)

	var completionTokens int64
	if maxTokens > 0 {
		completionTokens = int64(maxTokens)
	} else {
		completionTokens = te.EstimateCompletionTokens(prompt, model)
	}

	return promptTokens + completionTokens
}
