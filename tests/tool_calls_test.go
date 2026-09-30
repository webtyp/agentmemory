package tests

import (
	"testing"

	"webtyp.com/agentcontext"
	"webtyp.com/agentmemory"
	"webtyp.com/context"
	"webtyp.com/ddl"
	"webtyp.com/embed"
	"webtyp.com/llm"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/storage/mem"
	"webtyp.com/unixid"
)

type mockCompiler struct{}

func (m *mockCompiler) CompileDDL(s ddl.Stmt, _ model.Model) (string, []any, error) {
	return s.Table + "_compiled", nil, nil
}

func TestToolCalls_RoundTrip(t *testing.T) {
	ctx := context.Background()
	conn := mem.New()
	_ = agentmemory.Migrate(conn, &mockCompiler{})
	idGen, _ := unixid.NewUnixID()
	store, _ := agentmemory.New(ctx, agentmemory.Config{
		Conn:     conn,
		Embedder: embed.NewMockEmbedder(128),
		IDGen:    idGen,
	})

	tests := []struct {
		name  string
		calls []llm.ToolCall
	}{
		{
			name:  "empty",
			calls: nil,
		},
		{
			name: "single simple",
			calls: []llm.ToolCall{
				{ID: "c1", Name: "search", Input: `{"q":"golang"}`},
			},
		},
		{
			name: "multiple with special chars in input",
			calls: []llm.ToolCall{
				{ID: "call_123", Name: "eval", Input: `{"code":"x := 1,2:3"}`},
				{ID: "call_456", Name: "fetch", Input: `{"url":"https://example.com/api?a=1&b=2"}`},
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turnID := "turn_" + tt.name
			turn := agentcontext.Turn{
				ID: turnID,
				Message: llm.Message{
					Role:      llm.RoleAssistant,
					Content:   "calling tool",
					ToolCalls: tt.calls,
				},
				Tokens:    10,
				CreatedAt: int64(100 + i),
			}
			if err := store.AppendTurn(ctx, "session_1", turn); err != nil {
				t.Fatalf("AppendTurn failed: %v", err)
			}

			turns, err := store.GetTurns(ctx, "session_1", 100)
			if err != nil {
				t.Fatalf("GetTurns failed: %v", err)
			}

			var found *agentcontext.Turn
			for _, tr := range turns {
				if tr.ID == turnID {
					found = &tr
					break
				}
			}
			if found == nil {
				t.Fatalf("turn %s not found in turns", turnID)
			}

			decoded := found.Message.ToolCalls
			if len(decoded) != len(tt.calls) {
				t.Fatalf("len mismatch: got %d, want %d", len(decoded), len(tt.calls))
			}
			for j := range tt.calls {
				if decoded[j].ID != tt.calls[j].ID ||
					decoded[j].Name != tt.calls[j].Name ||
					decoded[j].Input != tt.calls[j].Input {
					t.Errorf("call[%d] mismatch:\ngot  %+v\nwant %+v", j, decoded[j], tt.calls[j])
				}
			}
		})
	}
}

func TestToolCalls_RowsFromV01StillDecode(t *testing.T) {
	ctx := context.Background()
	conn := mem.New()
	_ = agentmemory.Migrate(conn, &mockCompiler{})
	db := orm.New(conn)

	// Create a message row with hand-built v0.1 netstring encoding:
	// "6:call_1,6:search,14:{"q":"golang"},"
	row := &agentmemory.Message{
		Id:         "msg_v01",
		SessionId:  "session_v01",
		Role:       "assistant",
		Content:    "test content",
		ToolCalls:  "6:call_1,6:search,14:{\"q\":\"golang\"},",
		TokenCount: 5,
		CreatedAt:  1000,
	}
	if err := db.Create(row); err != nil {
		t.Fatalf("db.Create failed: %v", err)
	}

	idGen, _ := unixid.NewUnixID()
	store, _ := agentmemory.New(ctx, agentmemory.Config{
		Conn:     conn,
		Embedder: embed.NewMockEmbedder(128),
		IDGen:    idGen,
	})

	turns, err := store.GetTurns(ctx, "session_v01", 10)
	if err != nil {
		t.Fatalf("GetTurns failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}

	wantCalls := []llm.ToolCall{
		{ID: "call_1", Name: "search", Input: `{"q":"golang"}`},
	}

	gotCalls := turns[0].Message.ToolCalls
	if len(gotCalls) != len(wantCalls) {
		t.Fatalf("got %d tool calls, want %d", len(gotCalls), len(wantCalls))
	}
	if gotCalls[0] != wantCalls[0] {
		t.Errorf("got %+v, want %+v", gotCalls[0], wantCalls[0])
	}
}
