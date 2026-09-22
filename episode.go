package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/time"
)

func (s *Store) SaveEpisode(ctx *context.Context, sessionID, summary string, tokenCount int, fromID, toID string) error {
	id := s.idGen.NewID()
	e := &Episode{
		Id:         id,
		SessionId:  sessionID,
		Summary:    summary,
		TokenCount: int64(tokenCount),
		FromMsgId:  fromID,
		ToMsgId:    toID,
		CreatedAt:  time.Now() / 1_000_000_000,
	}
	return s.db.Create(e)
}

func (s *Store) GetEpisodes(ctx *context.Context, sessionID string, limit int) ([]agent.Episode, error) {
	var rows EpisodeList
	err := s.db.Query(&Episode{}).
		Where(Episode_.SessionId).Eq(sessionID).
		OrderBy(Episode_.CreatedAt).Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &Episode{} }, func(m model.Model) { rows = append(rows, m.(*Episode)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetEpisodes: ", err)
	}
	// rows come back newest-first (ORDER BY ... DESC); episodes read oldest-first.
	out := make([]agent.Episode, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = agent.Episode{
			ID:         r.Id,
			SessionID:  r.SessionId,
			Summary:    r.Summary,
			TokenCount: int(r.TokenCount),
			FromMsgID:  r.FromMsgId,
			ToMsgID:    r.ToMsgId,
			CreatedAt:  r.CreatedAt,
		}
	}
	return out, nil
}
