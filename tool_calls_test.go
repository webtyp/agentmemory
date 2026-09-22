package agentmemory

import (
	"testing"

	"webtyp.com/agent"
)

func TestToolCalls_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		calls []agent.ToolCall
	}{
		{
			name:  "empty",
			calls: nil,
		},
		{
			name: "single simple",
			calls: []agent.ToolCall{
				{ID: "c1", Name: "search", Input: `{"q":"golang"}`},
			},
		},
		{
			name: "multiple with special chars in input",
			calls: []agent.ToolCall{
				{ID: "call_123", Name: "eval", Input: `{"code":"x := 1,2:3"}`},
				{ID: "call_456", Name: "fetch", Input: `{"url":"https://example.com/api?a=1&b=2"}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeToolCalls(tt.calls)
			decoded, err := decodeToolCalls(encoded)
			if err != nil {
				t.Fatalf("decodeToolCalls failed: %v", err)
			}
			if len(decoded) != len(tt.calls) {
				t.Fatalf("len mismatch: got %d, want %d", len(decoded), len(tt.calls))
			}
			for i := range tt.calls {
				if decoded[i].ID != tt.calls[i].ID ||
					decoded[i].Name != tt.calls[i].Name ||
					decoded[i].Input != tt.calls[i].Input {
					t.Errorf("call[%d] mismatch:\ngot  %+v\nwant %+v", i, decoded[i], tt.calls[i])
				}
			}
		})
	}
}

func TestToolCalls_Malformed(t *testing.T) {
	malformedInputs := []string{
		"invalid",
		"100:short,",
		"abc:123,",
		"5:hello",                 // missing trailing comma
		"99999999999999999999:x,", // length prefix overflows int well before it could ever be a valid offset
	}
	for _, in := range malformedInputs {
		_, err := decodeToolCalls(in)
		if err == nil {
			t.Errorf("expected error for malformed input %q, got nil", in)
		}
	}
}
