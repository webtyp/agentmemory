package agentmemory

import (
	"webtyp.com/agentcontext"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/llm"
	"webtyp.com/model"
	"webtyp.com/storage"
)

// EnsureSession is a no-op validation, not a table write. There is no sessions table —
// every row of message/episode/tool_log carries a free-form session_id string.
func (s *Store) EnsureSession(ctx *context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Err("agentmemory: sessionID must not be empty")
	}
	return nil
}

func (s *Store) AppendTurn(ctx *context.Context, sessionID string, t agentcontext.Turn) error {
	if t.ID == "" {
		return fmt.Err("agentmemory: AppendTurn: Turn.ID must not be empty")
	}
	m := &Message{
		Id:         t.ID,
		SessionId:  sessionID,
		Role:       string(t.Message.Role),
		Content:    t.Message.Content,
		ToolName:   t.Message.ToolName,
		ToolCallId: t.Message.ToolCallID,
		ToolCalls:  encodeToolCalls(t.Message.ToolCalls),
		TokenCount: int64(t.Tokens),
		CreatedAt:  t.CreatedAt,
	}
	return s.db.Create(m)
}

func (s *Store) GetTurns(ctx *context.Context, sessionID string, limit int) ([]agentcontext.Turn, error) {
	var rows MessageList
	err := s.db.Query(&Message{}).
		Where(Message_.SessionId).Eq(sessionID).
		OrderBy(Message_.CreatedAt).Desc().
		OrderBy(Message_.Id).Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &Message{} }, func(m model.Model) { rows = append(rows, m.(*Message)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetTurns: ", err)
	}
	// rows come back newest-first (ORDER BY ... DESC); a conversation reads oldest-first.
	out := make([]agentcontext.Turn, len(rows))
	for i, r := range rows {
		calls, err := decodeToolCalls(r.ToolCalls)
		if err != nil {
			return nil, fmt.Err("agentmemory: GetTurns: ", err)
		}
		out[len(rows)-1-i] = agentcontext.Turn{
			ID: r.Id,
			Message: llm.Message{
				Role:       llm.Role(r.Role),
				Content:    r.Content,
				ToolName:   r.ToolName,
				ToolCallID: r.ToolCallId,
				ToolCalls:  calls,
			},
			Tokens:    int(r.TokenCount),
			CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (s *Store) DeleteTurns(ctx *context.Context, sessionID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	return s.db.Delete(&Message{}, storage.Eq(Message_.SessionId, sessionID), storage.In(Message_.Id, idsAny))
}
