package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"google.golang.org/protobuf/types/known/structpb"
)

// AIPromptSchema returns the JSON Schema for the ai_prompt tool.
func AIPromptSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt to send to Gemini",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Model to use (e.g., gemini-2.0-flash, gemini-2.0-pro)",
			},
			"workspace": map[string]any{
				"type":        "string",
				"description": "Working directory context (informational)",
			},
			"system_prompt": map[string]any{
				"type":        "string",
				"description": "Custom system prompt",
			},
			"env": map[string]any{
				"type":        "string",
				"description": "JSON object of environment variables (e.g., {\"GOOGLE_API_KEY\": \"...\"})",
			},
		},
		"required": []any{"prompt"},
	})
	return s
}

// AIPrompt returns a tool handler that sends a one-shot prompt to the Gemini
// API. The call is always synchronous (direct API, no background process).
func AIPrompt(bridge *Bridge) plugin.ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "prompt"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		opts, err := parseCommonOpts(req.Arguments)
		if err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		// One-shot: generate a temporary session ID for tracking.
		opts.SessionID = generateSessionID()
		opts.Resume = false

		resp, err := bridge.Call(ctx, opts)
		if err != nil {
			return helpers.ErrorResult("gemini_error", err.Error()), nil
		}

		return helpers.TextResult(formatChatResponse(resp)), nil
	}
}

// generateSessionID creates a random hex session ID.
func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("prompt-%x", b)
}

// --- Common helpers ---

// parseCommonOpts extracts the shared spawn options from tool arguments.
func parseCommonOpts(args *structpb.Struct) (SpawnOptions, error) {
	prompt := helpers.GetString(args, "prompt")
	model := helpers.GetString(args, "model")
	workspace := helpers.GetString(args, "workspace")
	systemPrompt := helpers.GetString(args, "system_prompt")
	envRaw := helpers.GetString(args, "env")

	// Default workspace to current working directory.
	if workspace == "" {
		cwd, err := os.Getwd()
		if err == nil {
			workspace = cwd
		}
	}

	// Parse env from JSON string.
	var envMap map[string]string
	if envRaw != "" {
		if err := json.Unmarshal([]byte(envRaw), &envMap); err != nil {
			return SpawnOptions{}, fmt.Errorf("invalid env JSON: %w", err)
		}
	}

	return SpawnOptions{
		Prompt:       prompt,
		Model:        model,
		Workspace:    workspace,
		SystemPrompt: systemPrompt,
		Env:          envMap,
	}, nil
}

// formatChatResponse formats a ChatResponse as a Markdown string for display.
func formatChatResponse(resp *ChatResponse) string {
	var b strings.Builder
	b.WriteString(resp.ResponseText)
	b.WriteString("\n\n---\n")

	if resp.SessionID != "" {
		fmt.Fprintf(&b, "- **Session:** %s\n", resp.SessionID)
	}
	if resp.ModelUsed != "" {
		fmt.Fprintf(&b, "- **Model:** %s\n", resp.ModelUsed)
	}
	if resp.TokensIn > 0 || resp.TokensOut > 0 {
		fmt.Fprintf(&b, "- **Tokens:** %d in / %d out\n", resp.TokensIn, resp.TokensOut)
	}
	if resp.CostUSD > 0 {
		fmt.Fprintf(&b, "- **Cost:** $%.4f\n", resp.CostUSD)
	}
	if resp.DurationMs > 0 {
		fmt.Fprintf(&b, "- **Duration:** %dms\n", resp.DurationMs)
	}

	return b.String()
}
