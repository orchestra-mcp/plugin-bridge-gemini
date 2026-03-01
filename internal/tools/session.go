package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"google.golang.org/protobuf/types/known/structpb"
)

// --- spawn_session ---

// SpawnSessionSchema returns the JSON Schema for the spawn_session tool.
func SpawnSessionSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{
				"type":        "string",
				"description": "Session UUID",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt to send",
			},
			"resume": map[string]any{
				"type":        "boolean",
				"description": "Resume existing session and replay history (default: false)",
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
		"required": []any{"session_id", "prompt"},
	})
	return s
}

// SpawnSession returns a tool handler that spawns a persistent Gemini session.
// Sessions maintain conversation history in memory. When resume=true, the
// existing session's history is replayed to maintain context.
func SpawnSession(bridge *Bridge) plugin.ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "session_id", "prompt"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		sessionID := helpers.GetString(req.Arguments, "session_id")
		resume := helpers.GetBool(req.Arguments, "resume")

		opts, err := parseCommonOpts(req.Arguments)
		if err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		opts.SessionID = sessionID
		opts.Resume = resume

		// Check for existing session to resume.
		existing := bridge.Plugin.GetSession(sessionID)
		if resume && existing != nil {
			// Replay history for resumed session.
			opts.SessionID = existing.ID
		} else if existing != nil && !resume {
			return helpers.ErrorResult("session_exists",
				fmt.Sprintf("session %q already exists; use resume=true to continue it", sessionID)), nil
		}

		resp, err := bridge.Call(ctx, opts)
		if err != nil {
			return helpers.ErrorResult("gemini_error", err.Error()), nil
		}

		// Create or update session with the new exchange.
		if existing == nil {
			session := &Session{
				ID:        sessionID,
				Model:     resp.ModelUsed,
				Status:    "active",
				StartedAt: time.Now().Format(time.RFC3339),
				History: []HistoryMessage{
					{Role: "user", Content: opts.Prompt},
					{Role: "assistant", Content: resp.ResponseText},
				},
				LastResp:  resp,
				TotalIn:   resp.TokensIn,
				TotalOut:  resp.TokensOut,
				TotalCost: resp.CostUSD,
			}
			bridge.Plugin.TrackSession(session)
		} else {
			existing.History = append(existing.History,
				HistoryMessage{Role: "user", Content: opts.Prompt},
				HistoryMessage{Role: "assistant", Content: resp.ResponseText},
			)
			existing.LastResp = resp
			existing.TotalIn += resp.TokensIn
			existing.TotalOut += resp.TokensOut
			existing.TotalCost += resp.CostUSD
			existing.Status = "active"
		}

		return helpers.TextResult(formatChatResponse(resp)), nil
	}
}

// --- kill_session ---

// KillSessionSchema returns the JSON Schema for the kill_session tool.
func KillSessionSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{
				"type":        "string",
				"description": "Session UUID to kill",
			},
		},
		"required": []any{"session_id"},
	})
	return s
}

// KillSession returns a tool handler that ends a Gemini session and removes
// it from the active session map.
func KillSession(bridge *Bridge) plugin.ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "session_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		sessionID := helpers.GetString(req.Arguments, "session_id")

		session := bridge.Plugin.RemoveSession(sessionID)
		if session == nil {
			return helpers.ErrorResult("not_found",
				fmt.Sprintf("no active session found with ID %q", sessionID)), nil
		}

		return helpers.TextResult(fmt.Sprintf(
			"Ended session **%s** (%d messages, %d tokens in / %d tokens out)",
			sessionID, len(session.History), session.TotalIn, session.TotalOut,
		)), nil
	}
}

// --- session_status ---

// SessionStatusSchema returns the JSON Schema for the session_status tool.
func SessionStatusSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{
				"type":        "string",
				"description": "Session UUID to check",
			},
		},
		"required": []any{"session_id"},
	})
	return s
}

// SessionStatus returns a tool handler that reports the current status of a
// Gemini conversation session.
func SessionStatus(bridge *Bridge) plugin.ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "session_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		sessionID := helpers.GetString(req.Arguments, "session_id")

		session := bridge.Plugin.GetSession(sessionID)
		if session == nil {
			return helpers.ErrorResult("not_found",
				fmt.Sprintf("no session found with ID %q", sessionID)), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Session: %s\n\n", sessionID)
		fmt.Fprintf(&b, "- **Status:** %s\n", session.Status)
		fmt.Fprintf(&b, "- **Model:** %s\n", session.Model)
		fmt.Fprintf(&b, "- **Started:** %s\n", session.StartedAt)
		fmt.Fprintf(&b, "- **Messages:** %d\n", len(session.History))
		fmt.Fprintf(&b, "- **Total Tokens:** %d in / %d out\n", session.TotalIn, session.TotalOut)

		if session.TotalCost > 0 {
			fmt.Fprintf(&b, "- **Total Cost:** $%.4f\n", session.TotalCost)
		}

		// Include the last response if available.
		if session.LastResp != nil {
			b.WriteString("\n### Last Response\n\n")
			b.WriteString(formatChatResponse(session.LastResp))
		}

		return helpers.TextResult(b.String()), nil
	}
}

// --- list_active ---

// ListActiveSchema returns the JSON Schema for the list_active tool.
func ListActiveSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
	return s
}

// ListActive returns a tool handler that lists all tracked Gemini sessions
// with their current status.
func ListActive(bridge *Bridge) plugin.ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		sessions := bridge.Plugin.ListSessions()

		if len(sessions) == 0 {
			return helpers.TextResult("## Active Sessions\n\nNo active sessions.\n"), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Active Sessions (%d)\n\n", len(sessions))
		fmt.Fprintf(&b, "| Session ID | Status | Model | Messages | Tokens In | Tokens Out |\n")
		fmt.Fprintf(&b, "|------------|--------|-------|----------|-----------|------------|\n")

		for _, s := range sessions {
			fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d |\n",
				s.ID, s.Status, s.Model, len(s.History), s.TotalIn, s.TotalOut)
		}

		return helpers.TextResult(b.String()), nil
	}
}
