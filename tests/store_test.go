package tests

import (
	"testing"

	"webtyp.com/agent"
	"webtyp.com/agent/conformance"
	"webtyp.com/agentcontext"
	"webtyp.com/agentmemory"
	"webtyp.com/context"
	"webtyp.com/ddl"
	"webtyp.com/embed"
	"webtyp.com/model"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
)

type mockDDLCompiler struct {
	Stmts []ddl.Stmt
}

func (c *mockDDLCompiler) CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	c.Stmts = append(c.Stmts, s)
	return s.Table + "_compiled", nil, nil
}

func TestAgentMemoryConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "agentmemory/mem",
		New: func(t *testing.T) agent.MemoryStore {
			ctx := context.Background()
			conn := mem.New() // schemaless: tables appear on first insert, no DDL to run
			idGen, err := unixid.NewUnixID()
			if err != nil {
				t.Fatalf("NewUnixID failed: %v", err)
			}
			store, err := agentmemory.New(ctx, agentmemory.Config{
				Conn:     conn,
				Embedder: embed.NewMockEmbedder(128),
				IDGen:    idGen,
			})
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}
			return store
		},
	})
}

// recordingExecer stands in for a DDL-capable connection: mem is schemaless and rejects raw
// statements, so Migrate is proven against the statements it compiles and executes.
type recordingExecer struct{ Queries []string }

func (e *recordingExecer) Exec(query string, args ...any) error {
	e.Queries = append(e.Queries, query)
	return nil
}

func TestMigrate_CreatesAllTables(t *testing.T) {
	conn := &recordingExecer{}
	compiler := &mockDDLCompiler{}
	if err := agentmemory.Migrate(conn, compiler); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}

	seen := make([]string, 0, len(compiler.Stmts))
	for _, s := range compiler.Stmts {
		seen = append(seen, s.Table)
	}
	for _, want := range []string{"message", "episode", "tool_log"} {
		found := false
		for _, s := range seen {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Migrate never compiled a DDL statement for table %q, got tables: %v", want, seen)
		}
	}

	// Verify idempotency
	if err := agentmemory.Migrate(conn, compiler); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}

func TestAppendTurn_EmptyIDErrors(t *testing.T) {
	ctx := context.Background()
	conn := mem.New()
	idGen, _ := unixid.NewUnixID()
	store, _ := agentmemory.New(ctx, agentmemory.Config{
		Conn:     conn,
		Embedder: embed.NewMockEmbedder(128),
		IDGen:    idGen,
	})
	err := store.AppendTurn(ctx, "s1", agentcontext.Turn{ID: ""})
	if err == nil || err.Error() != "agentmemory: AppendTurn: Turn.ID must not be empty" {
		t.Fatalf("got err %v, want 'agentmemory: AppendTurn: Turn.ID must not be empty'", err)
	}
}

func TestSaveSummary_EmptyIDErrors(t *testing.T) {
	ctx := context.Background()
	conn := mem.New()
	idGen, _ := unixid.NewUnixID()
	store, _ := agentmemory.New(ctx, agentmemory.Config{
		Conn:     conn,
		Embedder: embed.NewMockEmbedder(128),
		IDGen:    idGen,
	})
	err := store.SaveSummary(ctx, "s1", agentcontext.Summary{ID: ""})
	if err == nil || err.Error() != "agentmemory: SaveSummary: Summary.ID must not be empty" {
		t.Fatalf("got err %v, want 'agentmemory: SaveSummary: Summary.ID must not be empty'", err)
	}
}
