package agentmemory

import (
	"testing"

	"webtyp.com/agent"
	"webtyp.com/agent/conformance"
	"webtyp.com/context"
	"webtyp.com/embed"
	"webtyp.com/sqlt"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
)

func TestAgentMemoryConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "agentmemory/mem",
		New: func(t *testing.T) agent.MemoryStore {
			ctx := context.Background()
			conn := mem.New()
			compiler := sqlt.NewCompiler()
			err := Migrate(conn, compiler)
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
	compiler := sqlt.NewCompiler()
	if err := Migrate(conn, compiler); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	// Verify idempotency
	if err := Migrate(conn, compiler); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}
