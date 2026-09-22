package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/fmt"
)

// encodeToolCalls serializes calls into the netstring-framed string Message.ToolCalls stores.
// Empty input returns "".
func encodeToolCalls(calls []agent.ToolCall) string {
	var b fmt.Builder
	for _, c := range calls {
		writeField(&b, c.ID)
		writeField(&b, c.Name)
		writeField(&b, c.Input)
	}
	return b.String()
}

func writeField(b *fmt.Builder, s string) {
	b.WriteString(fmt.Sprintf("%d", len(s)))
	b.WriteByte(':')
	b.WriteString(s)
	b.WriteByte(',')
}

// decodeToolCalls is encodeToolCalls's inverse. Returns an error on malformed framing —
// never guesses or silently truncates.
func decodeToolCalls(s string) ([]agent.ToolCall, error) {
	var calls []agent.ToolCall
	i := 0
	for i < len(s) {
		id, next, err := readField(s, i)
		if err != nil {
			return nil, fmt.Err("agentmemory: decoding tool call id: ", err)
		}
		i = next
		name, next, err := readField(s, i)
		if err != nil {
			return nil, fmt.Err("agentmemory: decoding tool call name: ", err)
		}
		i = next
		input, next, err := readField(s, i)
		if err != nil {
			return nil, fmt.Err("agentmemory: decoding tool call input: ", err)
		}
		i = next
		calls = append(calls, agent.ToolCall{ID: id, Name: name, Input: input})
	}
	return calls, nil
}

// readField reads one "<len>:<content>," field starting at s[i], returning the content and
// the index right after the trailing comma. No strconv/fmt.Sscanf — TinyGo-safe manual
// digit parsing.
func readField(s string, i int) (string, int, error) {
	digitsEnd := i
	for digitsEnd < len(s) && s[digitsEnd] >= '0' && s[digitsEnd] <= '9' {
		digitsEnd++
	}
	if digitsEnd == i {
		return "", 0, fmt.Err("agentmemory: expected length digits at offset ", i)
	}
	n := 0
	for _, d := range []byte(s[i:digitsEnd]) {
		n = n*10 + int(d-'0')
	}
	if digitsEnd >= len(s) || s[digitsEnd] != ':' {
		return "", 0, fmt.Err("agentmemory: expected ':' after length at offset ", digitsEnd)
	}
	start := digitsEnd + 1
	end := start + n
	if end >= len(s) || s[end] != ',' {
		return "", 0, fmt.Err("agentmemory: field length mismatch or missing trailing ',' at offset ", end)
	}
	return s[start:end], end + 1, nil
}
