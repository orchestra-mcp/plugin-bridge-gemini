// Package internal contains the core logic for the bridge.gemini plugin.
// It calls the Google Gemini API directly via the generative-ai-go SDK,
// managing multi-turn chat sessions with in-memory history.
package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// CallGemini sends a prompt to the Google Gemini API and returns a
// ChatResponse. It supports multi-turn conversations via the History field
// in CallOptions.
func CallGemini(ctx context.Context, opts CallOptions) (*ChatResponse, error) {
	var clientOpts []option.ClientOption
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.APIKey))
	}

	client, err := genai.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	defer client.Close()

	model := opts.Model
	if model == "" {
		model = "gemini-2.0-flash"
	}

	gModel := client.GenerativeModel(model)
	if opts.SystemPrompt != "" {
		gModel.SystemInstruction = genai.NewUserContent(genai.Text(opts.SystemPrompt))
	}

	// Build chat session with history.
	cs := gModel.StartChat()
	for _, h := range opts.History {
		role := h.Role
		if role == "assistant" {
			role = "model"
		}
		cs.History = append(cs.History, &genai.Content{
			Parts: []genai.Part{genai.Text(h.Content)},
			Role:  role,
		})
	}

	start := time.Now()
	resp, err := cs.SendMessage(ctx, genai.Text(opts.Prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini send: %w", err)
	}
	durationMs := time.Since(start).Milliseconds()

	// Extract text from response.
	var responseText strings.Builder
	if resp.Candidates != nil && len(resp.Candidates) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			if t, ok := part.(genai.Text); ok {
				responseText.WriteString(string(t))
			}
		}
	}

	// Extract usage metadata.
	var tokensIn, tokensOut int64
	if resp.UsageMetadata != nil {
		tokensIn = int64(resp.UsageMetadata.PromptTokenCount)
		tokensOut = int64(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &ChatResponse{
		ResponseText: responseText.String(),
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		CostUSD:      0,
		ModelUsed:    model,
		DurationMs:   durationMs,
		SessionID:    opts.SessionID,
	}, nil
}

// CallOptions configures a single Gemini API call.
type CallOptions struct {
	SessionID    string
	Prompt       string
	Model        string
	SystemPrompt string
	APIKey       string
	History      []HistoryMessage
}

// HistoryMessage represents a single message in a conversation history.
type HistoryMessage struct {
	Role    string
	Content string
}

// ChatResponse holds the result of a completed Gemini API call.
type ChatResponse struct {
	ResponseText string  `json:"response_text"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	ModelUsed    string  `json:"model_used"`
	DurationMs   int64   `json:"duration_ms"`
	SessionID    string  `json:"session_id"`
}
