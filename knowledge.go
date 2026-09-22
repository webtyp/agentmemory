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

// knowledgeMatch pairs a vectordb.Match with the SessionID it should be reported under —
// "" for a global-tag match, sessionID for a session-tag match. vectordb.Match itself has
// no notion of which of our two tag searches produced it, so this has to be recorded before
// the two result sets are merged, not derived after.
type knowledgeMatch struct {
	match     vectordb.Match
	sessionID string
}

func (s *Store) SearchKnowledge(ctx *context.Context, query, sessionID string, limit int) ([]agent.Knowledge, error) {
	// limit<=0 means "unbounded" (agent.NewMemMemory's contract, which every other method
	// in this package already follows) — vectordb.Query.K has no such convention (K<=0
	// falls back to its own default of 4), so approximate "everything" with the corpus
	// size when the caller didn't ask for a specific cap.
	k := limit
	if k <= 0 {
		if k = s.vdb.Len(); k <= 0 {
			k = 1
		}
	}

	globalRaw, err := s.vdb.Search(ctx, vectordb.Query{Text: query, K: k, IncludeTags: []string{tagGlobal}})
	if err != nil {
		return nil, fmt.Err("agentmemory: SearchKnowledge (global): ", err)
	}
	globalMatches := make([]knowledgeMatch, len(globalRaw))
	for i, m := range globalRaw {
		globalMatches[i] = knowledgeMatch{match: m, sessionID: ""}
	}

	var sessionMatches []knowledgeMatch
	if sessionID != "" {
		sessionRaw, err := s.vdb.Search(ctx, vectordb.Query{Text: query, K: k, IncludeTags: []string{sessionTagPrefix + sessionID}})
		if err != nil {
			return nil, fmt.Err("agentmemory: SearchKnowledge (session): ", err)
		}
		sessionMatches = make([]knowledgeMatch, len(sessionRaw))
		for i, m := range sessionRaw {
			sessionMatches[i] = knowledgeMatch{match: m, sessionID: sessionID}
		}
	}

	merged := mergeMatchesByScore(globalMatches, sessionMatches, limit)
	out := make([]agent.Knowledge, len(merged))
	for i, km := range merged {
		src, createdAt := parseKnowledgeMeta(km.match.Meta)
		out[i] = agent.Knowledge{
			ID:        km.match.ID,
			SessionID: km.sessionID,
			Content:   km.match.Text,
			Source:    src,
			CreatedAt: createdAt,
		}
	}
	return out, nil
}

func mergeMatchesByScore(a, b []knowledgeMatch, limit int) []knowledgeMatch {
	combined := make([]knowledgeMatch, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	sort.SliceStable(combined, func(i, j int) bool {
		return combined[i].match.Score > combined[j].match.Score
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
