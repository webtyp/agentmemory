---
PLAN: "feat: ToolIndex by meaning — embeds every tool and the message, ranks by cosine (agent.ToolIndex)"
TAG: v0.3.0
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `webtyp.com/agentmemory`: find tools by meaning

## 0. Context

`webtyp.com/agent` asks a **`ToolIndex`** which tools fit a message. It offers those tools to the
model on the first step, and the hybrid agent will let a decision model choose among them. The
contract (`agent`, `interfaces.go`):

```go
type ToolIndex interface {
	// IndexTools replaces the indexed set with tools.
	IndexTools(ctx *context.Context, tools []llm.ToolDef) error
	// SearchTools returns the names of at most limit tools, most relevant first; empty when none match.
	SearchTools(ctx *context.Context, query string, limit int) ([]string, error)
}
```

Today the only implementation is `agent.NewMemToolIndex()`, which ranks by **shared words**. A
clinic's system has about 60 operations, and the decision model takes at most 10 options. So
the candidates must be chosen by **meaning**: "¿La Dra. Soto atiende los jueves?" must find
`work_schedule.get_work_schedule` (description "qué días y horas trabaja cada profesional"),
which shares no word with the message.

This repository already holds an `embed.Embedder` (`store.go`, `Config.Embedder`). The contract
is `webtyp.com/embed`:

```go
type Embedder interface {
	Dim() int
	ID() string
	// Embed writes one vector per text into dst, which MUST have length len(texts)*Dim().
	// Vectors are L2-normalised on the way out.
	Embed(ctx *context.Context, texts []string, dst []float32) error
	CountTokens(text string) int
}
```

Because the vectors are normalized, cosine similarity is the dot product (`vector.Dot` from
`webtyp.com/vector`). The tool set is small (tens) and is re-indexed every time an agent is
built, so the index lives **in memory**. Do not use `vectordb`, which persists to a database.

## Development rules (inline)

- Library code compiles for the browser (`GOOS=js GOARCH=wasm go build ./...`). Never import
  `fmt`, `errors`, `strings`, `strconv` (use `webtyp.com/fmt`), `sort` or `map[K]V` in non-test
  files. `sync` is allowed.
- Tests live in `tests/` (`package tests`) and use only the exported API.
- Max 500 lines per file. No `TODO`. `gotest` green.

## Design gate (api-design — five answers)

1. **Prior art.** LangGraph "bigtool" and Semantic Kernel's function filtering embed tool
   descriptions and retrieve the top-k for a request. Anthropic's tool search tool ranks tools for
   a query the same way. The name+description text is what all of them embed.
2. **Novice-name test.** `agentmemory.NewToolIndex(embedder)` returns a `*agentmemory.ToolIndex`
   that is an `agent.ToolIndex`, the same word in two packages, as with `io.Reader`/`bytes.Reader`.
3. **Complexity ledger.** Concepts +1, call site +1 line (`ToolIndex: agentmemory.NewToolIndex(emb)`),
   ways +0 (the keyword index stays the zero-dependency reference in `agent`).
4. **Where it belongs.** The ecosystem master plan places the semantic `ToolIndex` in
   `agentmemory`, which already owns the embedder.
5. **What it deletes.** Nothing. It adds capability.

## Stage 1 — `tool_index.go`

```go
// ToolIndex finds tools by meaning. It embeds every tool's name and description once, embeds
// each query, and ranks the tools by cosine similarity. It implements agent.ToolIndex.
type ToolIndex struct {
	emb   embed.Embedder
	mu    sync.RWMutex
	names []string  // tool names, in IndexTools order
	vecs  []float32 // len(names) * emb.Dim(), row i is tool i
}

// NewToolIndex builds an empty index over emb.
func NewToolIndex(emb embed.Embedder) (*ToolIndex, error)

var _ agent.ToolIndex = (*ToolIndex)(nil)
```

- `NewToolIndex(nil)` → error `agentmemory: NewToolIndex needs an Embedder` (constant).
- `IndexTools(ctx, tools)`:
  - The text of a tool is `toolText(t)`: its name with every `_` and `.` replaced by a space,
    then `": "`, then its description. For example,
    `business_calendar.list_business_hours` + "Horario…" becomes
    `"business calendar list business hours: Horario…"`.
  - Embed all texts in **one** `Embed` call into a fresh slice of `len(tools)*Dim()`, then replace
    `names` and `vecs` under the write lock.
  - An `Embed` error is returned wrapped as `agentmemory: indexing tools: %w`, and the previous
    set stays. Zero tools leave an empty index.
- `SearchTools(ctx, query, limit)`:
  - `limit <= 0` → error `agentmemory: SearchTools needs a positive limit`.
  - Empty index → `nil, nil`.
  - Embed the query (one text), compute `vector.Dot` against every row, and return the names of
    the `limit` highest scores, highest first. Equal scores keep IndexTools order. Use a simple
    selection over the slice, not `sort`.
  - An `Embed` error is returned wrapped as `agentmemory: searching tools: %w`.

## Stage 2 — tests (`tests/tool_index_test.go`)

A fake embedder that makes texts sharing words similar (bag of words into a fixed dimension):

```go
// wordsEmbedder hashes each lowercase word of a text into one of 64 dimensions and normalizes:
// texts that share words get a higher dot product.
type wordsEmbedder struct{ fail bool }

func (wordsEmbedder) Dim() int                  { return 64 }
func (wordsEmbedder) ID() string                { return "words-test" }
func (wordsEmbedder) CountTokens(s string) int  { return len(strings.Fields(s)) }
func (e wordsEmbedder) Embed(ctx *context.Context, texts []string, dst []float32) error {
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
```

(Tests may use the standard library: `strings`, `errors`, `hash/fnv`.)

- **Ranking:** index three tools, `{"business_calendar.list_business_hours", "horario de
  atención del consultorio"}`, `{"item_catalog.list_items", "servicios y precios"}` and
  `{"patient_directory.list_patients", "buscar pacientes"}`. The query `"precios de los
  servicios"` returns `item_catalog.list_items` first.
- **Limit:** limit 2 returns 2 names; limit 10 returns all 3.
- **Replace:** a second `IndexTools` with one tool → searches only return that tool.
- **Empty and errors:** an empty index returns no names and no error. Limit 0 gives its error. A
  failing embedder gives the wrapped errors, and a failed `IndexTools` keeps the previous set.
- **Consumer-shaped:** `agent.New(agent.Config{…, ToolIndex: idx, LocalTools: <the three tools as
  agent.Tool>, PreselectTools: 1})` with a recording fake `llm.Client`. For the message `"precios
  de los servicios"`, the first request offers `search_tools` plus `item_catalog.list_items` only.
  Copy the minimal fakes you need from `webtyp/agent`'s own `tests/`; they are test code.
- `go get webtyp.com/agent@latest` for the consumer test (≥ v0.9.0 has `PreselectTools`).

## Stage 3 — docs

- `README.md`: "Find tools by meaning: `agentmemory.NewToolIndex(embedder)`", with a 5-line example
  plugging it into `agent.Config.ToolIndex`. Say that bekko is the embedder the browser uses.
- `AGENTS.md`: list `tool_index.go` under the files, with one line saying it is in memory and does
  not use `vectordb`, and why.

## Stages

| Stage | Files | Acceptance |
|---|---|---|
| 1 | `tool_index.go` | builds; `var _ agent.ToolIndex` |
| 2 | `tests/tool_index_test.go` | all listed tests pass |
| 3 | `README.md`, `AGENTS.md` | documented |
| all | — | `gotest` green; `GOOS=js GOARCH=wasm go build ./...` |
