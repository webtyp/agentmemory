package agentmemory

import (
	"webtyp.com/agentcontext"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/model"
)

func (s *Store) SaveSummary(ctx *context.Context, sessionID string, summary agentcontext.Summary) error {
	if summary.ID == "" {
		return fmt.Err("agentmemory: SaveSummary: Summary.ID must not be empty")
	}
	e := &Episode{
		Id:         summary.ID,
		SessionId:  sessionID,
		Summary:    summary.Text,
		TokenCount: int64(summary.Tokens),
		FromMsgId:  summary.FromTurnID,
		ToMsgId:    summary.ToTurnID,
		CreatedAt:  summary.CreatedAt,
	}
	return s.db.Create(e)
}

func (s *Store) GetSummaries(ctx *context.Context, sessionID string, limit int) ([]agentcontext.Summary, error) {
	var rows EpisodeList
	err := s.db.Query(&Episode{}).
		Where(Episode_.SessionId).Eq(sessionID).
		OrderBy(Episode_.CreatedAt).Desc().
		OrderBy(Episode_.Id).Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &Episode{} }, func(m model.Model) { rows = append(rows, m.(*Episode)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetSummaries: ", err)
	}
	// rows come back newest-first (ORDER BY ... DESC); summaries read oldest-first.
	out := make([]agentcontext.Summary, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = agentcontext.Summary{
			ID:         r.Id,
			Text:       r.Summary,
			Tokens:     int(r.TokenCount),
			FromTurnID: r.FromMsgId,
			ToTurnID:   r.ToMsgId,
			CreatedAt:  r.CreatedAt,
		}
	}
	return out, nil
}
