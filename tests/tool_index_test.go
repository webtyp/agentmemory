package tests

import (
	"errors"
	"hash/fnv"
	"strings"
	"sync"
	"testing"

	"webtyp.com/agent"
	"webtyp.com/agentcontext"
	"webtyp.com/agentmemory"
	"webtyp.com/context"
	"webtyp.com/embed"
	"webtyp.com/llm"
	"webtyp.com/model"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
	"webtyp.com/vector"
)

type wordsEmbedder struct{ fail bool }

func (wordsEmbedder) Dim() int                 { return 64 }
func (wordsEmbedder) ID() string               { return "words-test" }
func (wordsEmbedder) CountTokens(s string) int { return len(strings.Fields(s)) }
func (wordsEmbedder) Close() error             { return nil }
func (e *wordsEmbedder) Embed(ctx *context.Context, texts []string, dst []float32) error {
	if e.fail {
		return errors.New("embed failed")
	}
	for i, t := range texts {
		v := dst[i*64 : (i+1)*64]
		for j := range v {
			v[j] = 0
		}
		for _, w := range strings.Fields(strings.ToLower(t)) {
			h := fnv.New32a()
			h.Write([]byte(strings.Trim(w, "¿?¡!.,:")))
			v[h.Sum32()%64]++
		}
		vector.Normalize(v)
	}
	return nil
}

type mockTool struct {
	name string
	desc string
}

func (m mockTool) Name() string                                              { return m.name }
func (m mockTool) Description() string                                       { return m.desc }
func (m mockTool) InputSchema() string                                       { return `{"type":"object"}` }
func (m mockTool) Action() model.Action                                      { return model.Read }
func (m mockTool) Execute(ctx *context.Context, argsJSON string) (string, error) { return "ok", nil }

type recordingLLM struct {
	mu       sync.Mutex
	requests []llm.Request
}

func (r *recordingLLM) Generate(ctx *context.Context, req llm.Request) (llm.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	return llm.Response{
		Text:       "Done",
		StopReason: llm.StopEndTurn,
	}, nil
}

type mockTokens struct{}

func (mockTokens) CountTokens(s string) int { return len(s) }

func TestToolIndex_New_NilEmbedder(t *testing.T) {
	_, err := agentmemory.NewToolIndex(nil)
	if err == nil {
		t.Fatal("expected error when passing nil embedder, got nil")
	}
}

func TestToolIndex_Ranking_Limit_Replace_Errors(t *testing.T) {
	ctx := context.Background()
	emb := &wordsEmbedder{}
	idx, err := agentmemory.NewToolIndex(emb)
	if err != nil {
		t.Fatalf("NewToolIndex failed: %v", err)
	}

	// Empty index search
	names, err := idx.SearchTools(ctx, "precios", 5)
	if err != nil {
		t.Fatalf("SearchTools on empty index failed: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names on empty index, got %v", names)
	}

	// Limit <= 0 error
	_, err = idx.SearchTools(ctx, "precios", 0)
	if err == nil {
		t.Fatal("expected error for limit 0, got nil")
	}

	tools := []llm.ToolDef{
		{
			Name:        "business_calendar.list_business_hours",
			Description: "horario de atención del consultorio",
		},
		{
			Name:        "item_catalog.list_items",
			Description: "servicios y precios",
		},
		{
			Name:        "patient_directory.list_patients",
			Description: "buscar pacientes",
		},
	}

	if err := idx.IndexTools(ctx, tools); err != nil {
		t.Fatalf("IndexTools failed: %v", err)
	}

	// Ranking test: query "precios de los servicios" -> item_catalog.list_items first
	ranked, err := idx.SearchTools(ctx, "precios de los servicios", 3)
	if err != nil {
		t.Fatalf("SearchTools failed: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 items, got %d", len(ranked))
	}
	if ranked[0] != "item_catalog.list_items" {
		t.Errorf("expected first item to be item_catalog.list_items, got %s", ranked[0])
	}

	// Limit test: limit 2 returns 2 names, limit 10 returns 3
	lim2, err := idx.SearchTools(ctx, "precios de los servicios", 2)
	if err != nil {
		t.Fatalf("SearchTools limit 2 failed: %v", err)
	}
	if len(lim2) != 2 {
		t.Errorf("expected 2 names, got %d", len(lim2))
	}

	lim10, err := idx.SearchTools(ctx, "precios de los servicios", 10)
	if err != nil {
		t.Fatalf("SearchTools limit 10 failed: %v", err)
	}
	if len(lim10) != 3 {
		t.Errorf("expected 3 names, got %d", len(lim10))
	}

	// Replace test: re-index with 1 tool
	singleTool := []llm.ToolDef{
		{
			Name:        "single_tool",
			Description: "only tool available",
		},
	}
	if err := idx.IndexTools(ctx, singleTool); err != nil {
		t.Fatalf("IndexTools replacement failed: %v", err)
	}

	res, err := idx.SearchTools(ctx, "precios", 5)
	if err != nil {
		t.Fatalf("SearchTools after replace failed: %v", err)
	}
	if len(res) != 1 || res[0] != "single_tool" {
		t.Errorf("expected ['single_tool'], got %v", res)
	}

	// Error handling and keeping previous set
	if err := idx.IndexTools(ctx, tools); err != nil {
		t.Fatalf("IndexTools reset failed: %v", err)
	}

	emb.fail = true

	// Failed IndexTools returns error
	err = idx.IndexTools(ctx, singleTool)
	if err == nil {
		t.Fatal("expected error on IndexTools with failing embedder, got nil")
	}

	// Failed SearchTools returns error
	_, err = idx.SearchTools(ctx, "test", 5)
	if err == nil {
		t.Fatal("expected error on SearchTools with failing embedder")
	}

	// Restore embedder and check previous set was kept
	emb.fail = false
	kept, err := idx.SearchTools(ctx, "precios de los servicios", 3)
	if err != nil {
		t.Fatalf("SearchTools after restored embedder failed: %v", err)
	}
	if len(kept) != 3 || kept[0] != "item_catalog.list_items" {
		t.Errorf("expected previous tool set to be kept, got %v", kept)
	}
}

func TestToolIndex_ConsumerShaped(t *testing.T) {
	ctx := context.Background()
	emb := &wordsEmbedder{}
	idx, err := agentmemory.NewToolIndex(emb)
	if err != nil {
		t.Fatalf("NewToolIndex failed: %v", err)
	}

	localTools := []agent.Tool{
		mockTool{
			name: "business_calendar.list_business_hours",
			desc: "horario de atención del consultorio",
		},
		mockTool{
			name: "item_catalog.list_items",
			desc: "servicios y precios",
		},
		mockTool{
			name: "patient_directory.list_patients",
			desc: "buscar pacientes",
		},
	}

	conn := mem.New()
	idGen, _ := unixid.NewUnixID()
	memStore, err := agentmemory.New(ctx, agentmemory.Config{
		Conn:     conn,
		Embedder: embed.NewMockEmbedder(64),
		IDGen:    idGen,
	})
	if err != nil {
		t.Fatalf("agentmemory.New failed: %v", err)
	}

	llmClient := &recordingLLM{}

	ag, err := agent.New(agent.Config{
		Identity: agentcontext.Identity{Name: "test-agent"},
		LLMs: agent.LLMConfig{
			Primary: llmClient,
		},
		Tokens: mockTokens{},
		Budget: agentcontext.Budget{
			ContextTokens: 8192,
			OutputTokens:  1024,
		},
		Memory:         memStore,
		IDGen:          idGen,
		ToolIndex:      idx,
		PreselectTools: 1,
		LocalTools:     localTools,
	})
	if err != nil {
		t.Fatalf("agent.New failed: %v", err)
	}

	_, err = ag.Run(ctx, "session1", "precios de los servicios")
	if err != nil {
		t.Fatalf("ag.Run failed: %v", err)
	}

	llmClient.mu.Lock()
	reqs := llmClient.requests
	llmClient.mu.Unlock()

	if len(reqs) == 0 {
		t.Fatal("expected at least 1 LLM request")
	}

	firstReq := reqs[0]
	toolNames := make([]string, len(firstReq.Tools))
	for i, td := range firstReq.Tools {
		toolNames[i] = td.Name
	}

	hasSearchTools := false
	hasItemCatalog := false
	for _, name := range toolNames {
		if name == "search_tools" {
			hasSearchTools = true
		}
		if name == "item_catalog.list_items" {
			hasItemCatalog = true
		}
	}

	if !hasSearchTools || !hasItemCatalog || len(toolNames) != 2 {
		t.Errorf("expected exactly search_tools and item_catalog.list_items, got %v", toolNames)
	}
}
