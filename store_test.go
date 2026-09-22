package agentmemory

import (
	"testing"

	"webtyp.com/agent"
	"webtyp.com/agent/conformance"
	"webtyp.com/context"
	"webtyp.com/ddl"
	"webtyp.com/embed"
	"webtyp.com/model"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
)

// mockDDLCompiler stands in for a real dialect (sqlt, postgres) so this repo's tests never
// import one directly — AGENTS.md: "Do not import webtyp.com/indexdb, webtyp.com/sqlt or
// webtyp.com/postgres directly." mem.Conn ignores the compiled SQL entirely (it
// auto-vivifies tables on first Create), so Migrate against mem was never going to validate
// real dialect SQL either way — this at least confirms Migrate reaches CompileDDL for every
// table it owns, which the previous sqlt+mem pairing didn't actually prove despite looking
// like it did. Pattern copied from webtyp/ddl's own ddl_test.go:mockDDLCompiler.
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
			conn := mem.New()
			err := Migrate(conn, &mockDDLCompiler{})
			if err != nil {
				t.Fatalf("Migrate failed: %v", err)
			}
			idGen, err := unixid.NewUnixID()
			if err != nil {
				t.Fatalf("NewUnixID failed: %v", err)
			}
			store, err := New(ctx, Config{
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

func TestMigrate_CreatesAllTables(t *testing.T) {
	conn := mem.New()
	compiler := &mockDDLCompiler{}
	if err := Migrate(conn, compiler); err != nil {
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
	if err := Migrate(conn, compiler); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}
