package agentmemory

import (
	"sort"

	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/time"
	"webtyp.com/vectordb"
)

const tagGlobal = "global"
const sessionTagPrefix = "session:"

func (s *Store) SaveKnowledge(ctx *context.Context, sessionID, content, source string) error {
	tag := tagGlobal
	if sessionID != "" {
		tag = sessionTagPrefix + sessionID
	}
	nowStr := fmt.Sprintf("%d", time.Now()/1_000_000_000)
	meta := fmt.Sprintf("%d:%s,%d:%s,", len(source), source, len(nowStr), nowStr)
	_, err := s.vdb.Add(ctx, vectordb.Doc{Text: content, Meta: meta, Tags: []string{tag}})
	if err != nil {
		return fmt.Err("agentmemory: SaveKnowledge: ", err)
	}
	return nil
}

func (s *Store) SearchKnowledge(ctx *context.Context, query, sessionID string, limit int) ([]agent.Knowledge, error) {
	globalMatches, err := s.vdb.Search(ctx, vectordb.Query{Text: query, K: limit, IncludeTags: []string{tagGlobal}})
	if err != nil {
		return nil, fmt.Err("agentmemory: SearchKnowledge (global): ", err)
	}

	var sessionMatches []vectordb.Match
	if sessionID != "" {
		sessionMatches, err = s.vdb.Search(ctx, vectordb.Query{Text: query, K: limit, IncludeTags: []string{sessionTagPrefix + sessionID}})
		if err != nil {
			return nil, fmt.Err("agentmemory: SearchKnowledge (session): ", err)
		}
	}

	merged := mergeMatchesByScore(globalMatches, sessionMatches, limit)
	out := make([]agent.Knowledge, len(merged))
	for i, m := range merged {
		src, createdAt := parseKnowledgeMeta(m.Meta)
		out[i] = agent.Knowledge{
			ID:        m.ID,
			SessionID: sessionID,
			Content:   m.Text,
			Source:    src,
			CreatedAt: createdAt,
		}
	}
	return out, nil
}

func mergeMatchesByScore(a, b []vectordb.Match, limit int) []vectordb.Match {
	combined := make([]vectordb.Match, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	sort.SliceStable(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})
	if limit > 0 && len(combined) > limit {
		return combined[:limit]
	}
	return combined
}

func parseKnowledgeMeta(meta string) (string, int64) {
	if meta == "" {
		return "", 0
	}
	source, next, err := readField(meta, 0)
	if err != nil {
		return "", 0
	}
	createdAtStr, _, err := readField(meta, next)
	if err != nil {
		return source, 0
	}
	var createdAt int64
	for _, d := range []byte(createdAtStr) {
		if d >= '0' && d <= '9' {
			createdAt = createdAt*10 + int64(d-'0')
		}
	}
	return source, createdAt
}
