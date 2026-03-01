package internal

import (
	"context"
	"sync"

	"github.com/orchestra-mcp/sdk-go/plugin"
	"github.com/orchestra-mcp/plugin-bridge-gemini/internal/tools"
)

// BridgePlugin manages Gemini API sessions and registers all bridge tools.
type BridgePlugin struct {
	sessions map[string]*tools.Session
	mu       sync.RWMutex
}

// NewBridgePlugin creates a new BridgePlugin with an empty session map.
func NewBridgePlugin() *BridgePlugin {
	return &BridgePlugin{
		sessions: make(map[string]*tools.Session),
	}
}

// RegisterTools registers all 5 bridge tools with the plugin builder.
func (bp *BridgePlugin) RegisterTools(builder *plugin.PluginBuilder) {
	bridge := &tools.Bridge{
		Call:   bp.callAdapter,
		Plugin: bp,
	}

	// --- Prompt tool (1) ---
	builder.RegisterTool("ai_prompt",
		"Send a one-shot prompt to the Gemini API and return the response",
		tools.AIPromptSchema(), tools.AIPrompt(bridge))

	// --- Session tools (4) ---
	builder.RegisterTool("spawn_session",
		"Spawn a persistent Gemini conversation session with a prompt",
		tools.SpawnSessionSchema(), tools.SpawnSession(bridge))

	builder.RegisterTool("kill_session",
		"End a Gemini conversation session",
		tools.KillSessionSchema(), tools.KillSession(bridge))

	builder.RegisterTool("session_status",
		"Check the status of a Gemini conversation session",
		tools.SessionStatusSchema(), tools.SessionStatus(bridge))

	builder.RegisterTool("list_active",
		"List all active Gemini conversation sessions",
		tools.ListActiveSchema(), tools.ListActive(bridge))
}

// callAdapter converts from tools.SpawnOptions to internal CallOptions and
// calls the Gemini API. It also replays session history when resuming.
func (bp *BridgePlugin) callAdapter(ctx context.Context, opts tools.SpawnOptions) (*tools.ChatResponse, error) {
	// Build history from existing session if resuming.
	var history []HistoryMessage
	if opts.Resume {
		if session := bp.GetSession(opts.SessionID); session != nil {
			for _, h := range session.History {
				history = append(history, HistoryMessage{
					Role:    h.Role,
					Content: h.Content,
				})
			}
		}
	}

	// Extract API key from env.
	apiKey := ""
	if opts.Env != nil {
		apiKey = opts.Env["GOOGLE_API_KEY"]
	}

	callOpts := CallOptions{
		SessionID:    opts.SessionID,
		Prompt:       opts.Prompt,
		Model:        opts.Model,
		SystemPrompt: opts.SystemPrompt,
		APIKey:       apiKey,
		History:      history,
	}

	resp, err := CallGemini(ctx, callOpts)
	if err != nil {
		return nil, err
	}

	return convertResp(resp), nil
}

// --- BridgePluginInterface implementation ---

// TrackSession adds a session to the active map.
func (bp *BridgePlugin) TrackSession(s *tools.Session) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.sessions[s.ID] = s
}

// GetSession returns the session for the given ID, or nil if not found.
func (bp *BridgePlugin) GetSession(sessionID string) *tools.Session {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.sessions[sessionID]
}

// RemoveSession removes and returns the session for the given ID.
func (bp *BridgePlugin) RemoveSession(sessionID string) *tools.Session {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	session, ok := bp.sessions[sessionID]
	if !ok {
		return nil
	}
	delete(bp.sessions, sessionID)
	return session
}

// ListSessions returns a snapshot of all active sessions.
func (bp *BridgePlugin) ListSessions() []*tools.Session {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	sessions := make([]*tools.Session, 0, len(bp.sessions))
	for _, s := range bp.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// CloseAll marks all sessions as completed. Called during shutdown.
func (bp *BridgePlugin) CloseAll() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	for id, s := range bp.sessions {
		s.Status = "completed"
		delete(bp.sessions, id)
	}
}

// --- Type conversion helpers ---

func convertResp(resp *ChatResponse) *tools.ChatResponse {
	if resp == nil {
		return nil
	}
	return &tools.ChatResponse{
		ResponseText: resp.ResponseText,
		TokensIn:     resp.TokensIn,
		TokensOut:    resp.TokensOut,
		CostUSD:      resp.CostUSD,
		ModelUsed:    resp.ModelUsed,
		DurationMs:   resp.DurationMs,
		SessionID:    resp.SessionID,
	}
}
