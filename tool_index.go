package agentmemory

import (
	"sync"

	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/embed"
	"webtyp.com/fmt"
	"webtyp.com/llm"
	"webtyp.com/vector"
)

// ToolIndex finds tools by meaning. It embeds every tool's name and description once, embeds
// each query, and ranks the tools by cosine similarity. It implements agent.ToolIndex.
type ToolIndex struct {
	emb   embed.Embedder
	mu    sync.RWMutex
	names []string  // tool names, in IndexTools order
	vecs  []float32 // len(names) * emb.Dim(), row i is tool i
}

var _ agent.ToolIndex = (*ToolIndex)(nil)

// NewToolIndex builds an empty index over emb.
func NewToolIndex(emb embed.Embedder) (*ToolIndex, error) {
	if emb == nil {
		return nil, fmt.Err("agentmemory: NewToolIndex needs an Embedder")
	}
	return &ToolIndex{emb: emb}, nil
}

func toolText(t llm.ToolDef) string {
	nameBytes := []byte(t.Name)
	for i, b := range nameBytes {
		if b == '_' || b == '.' {
			nameBytes[i] = ' '
		}
	}
	return fmt.Sprintf("%s: %s", string(nameBytes), t.Description)
}

func (idx *ToolIndex) IndexTools(ctx *context.Context, tools []llm.ToolDef) error {
	if len(tools) == 0 {
		idx.mu.Lock()
		idx.names = nil
		idx.vecs = nil
		idx.mu.Unlock()
		return nil
	}

	dim := idx.emb.Dim()
	texts := make([]string, len(tools))
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
		texts[i] = toolText(t)
	}

	dst := make([]float32, len(tools)*dim)
	if err := idx.emb.Embed(ctx, texts, dst); err != nil {
		return fmt.Errf("agentmemory: indexing tools: %w", err)
	}

	idx.mu.Lock()
	idx.names = names
	idx.vecs = dst
	idx.mu.Unlock()
	return nil
}

func (idx *ToolIndex) SearchTools(ctx *context.Context, query string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Err("agentmemory: SearchTools needs a positive limit")
	}

	idx.mu.RLock()
	names := idx.names
	vecs := idx.vecs
	idx.mu.RUnlock()

	if len(names) == 0 {
		return nil, nil
	}

	dim := idx.emb.Dim()
	qVec := make([]float32, dim)
	if err := idx.emb.Embed(ctx, []string{query}, qVec); err != nil {
		return nil, fmt.Errf("agentmemory: searching tools: %w", err)
	}

	type item struct {
		name  string
		idx   int
		score float32
	}

	n := len(names)
	items := make([]item, n)
	for i := 0; i < n; i++ {
		row := vecs[i*dim : (i+1)*dim]
		items[i] = item{
			name:  names[i],
			idx:   i,
			score: vector.Dot(qVec, row),
		}
	}

	// Stable selection/sort: highest score first, equal scores preserve IndexTools order (idx order).
	for i := 1; i < n; i++ {
		curr := items[i]
		j := i - 1
		for j >= 0 && (items[j].score < curr.score || (items[j].score == curr.score && items[j].idx > curr.idx)) {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = curr
	}

	k := limit
	if k > n {
		k = n
	}

	res := make([]string, k)
	for i := 0; i < k; i++ {
		res[i] = items[i].name
	}

	return res, nil
}
