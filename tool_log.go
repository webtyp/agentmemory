package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/time"
)

func (s *Store) LogToolCall(ctx *context.Context, sessionID, toolName, inputJSON, outputText, errText string, durationMS int64) error {
	id := s.idGen.NewID()
	t := &ToolLog{
		Id:         id,
		SessionId:  sessionID,
		ToolName:   toolName,
		InputJson:  inputJSON,
		OutputText: outputText,
		ErrText:    errText,
		DurationMs: durationMS,
		CreatedAt:  time.Now() / 1_000_000_000,
	}
	return s.db.Create(t)
}

func (s *Store) GetToolLogs(ctx *context.Context, sessionID, toolName string, limit int) ([]agent.ToolLog, error) {
	var rows ToolLogList
	qb := s.db.Query(&ToolLog{}).Where(ToolLog_.SessionId).Eq(sessionID)
	if toolName != "" {
		qb = qb.Where(ToolLog_.ToolName).Eq(toolName)
	}
	err := qb.OrderBy(ToolLog_.CreatedAt).Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &ToolLog{} }, func(m model.Model) { rows = append(rows, m.(*ToolLog)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetToolLogs: ", err)
	}
	out := make([]agent.ToolLog, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = agent.ToolLog{
			ID:         r.Id,
			SessionID:  r.SessionId,
			ToolName:   r.ToolName,
			InputJSON:  r.InputJson,
			OutputText: r.OutputText,
			ErrText:    r.ErrText,
			DurationMS: r.DurationMs,
			CreatedAt:  r.CreatedAt,
		}
	}
	return out, nil
}
