package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
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

func (s *Store) AppendMessage(ctx *context.Context, sessionID string, msg agent.Message) error {
	id := msg.ID
	if id == "" {
		id = s.idGen.NewID()
	}
	m := &Message{
		Id:         id,
		SessionId:  sessionID,
		Role:       msg.Role,
		Content:    msg.Content,
		ToolName:   msg.ToolName,
		ToolCallId: msg.ToolCallID,
		ToolCalls:  encodeToolCalls(msg.ToolCalls),
		TokenCount: int64(msg.TokenCount),
		CreatedAt:  msg.CreatedAt,
	}
	return s.db.Create(m)
}

func (s *Store) GetMessages(ctx *context.Context, sessionID string, limit int) ([]agent.Message, error) {
	var rows MessageList
	err := s.db.Query(&Message{}).
		Where(Message_.SessionId).Eq(sessionID).
		OrderBy(Message_.CreatedAt).Desc().
		OrderBy(Message_.Id).Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &Message{} }, func(m model.Model) { rows = append(rows, m.(*Message)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetMessages: ", err)
	}
	// rows come back newest-first (ORDER BY ... DESC); a conversation reads oldest-first.
	out := make([]agent.Message, len(rows))
	for i, r := range rows {
		calls, err := decodeToolCalls(r.ToolCalls)
		if err != nil {
			return nil, fmt.Err("agentmemory: GetMessages: ", err)
		}
		out[len(rows)-1-i] = agent.Message{
			ID: r.Id, SessionID: r.SessionId, Role: r.Role, Content: r.Content,
			ToolName: r.ToolName, ToolCallID: r.ToolCallId, ToolCalls: calls,
			TokenCount: int(r.TokenCount), CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (s *Store) DeleteMessages(ctx *context.Context, sessionID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	return s.db.Delete(&Message{}, storage.Eq(Message_.SessionId, sessionID), storage.In(Message_.Id, idsAny))
}
