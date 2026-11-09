# Agent Instructions

<!-- webtyp:base — copied from devskills skill agents-md; do not edit here -->
## Ecosystem rules — read before any change

`docs/PLAN.md` is the spec: no design judgment; anything unclear is a question in the PR, not a guess.

- Public API: typed (no `any`/`interface{}`), explicit, sequential, minimal; no generics (`[T any]`) or `reflect` without benchmarked allocation/performance justification and owner approval; never export a symbol only a test uses.
- One intent, one path: the old path is deleted in the same change. No silent fallback, `TODO`, commented code or stub.
- Fail at compile time, else a loud diagnostic — never silently. Zero values deny.
- A defect in a dependency is reported, never wrapped or patched here.
- No string literals in logic: named constants. The same logic never lives in two repos: import the package the plan names; if you find an existing one the plan does not name that already does what you are about to write, say so in the PR instead of writing it.
- `cmd/*/main.go`: parse flags, inject, print/exit only. stdout = data, stderr = logs, exit codes are the contract.
- WASM code: `webtyp/fmt`, `webtyp/time`, `webtyp/json` instead of stdlib (imported without alias as `fmt`, `time`, `json` — never `tinytime`); no `map`; `dom.Element` embedded by value.
- `css.go`, `svg.go`, `js.go`, `html.go` carry `//go:build !wasm`; no `ssr.go`, no `front.go`.
- Build tags: `wasm`/`!wasm` only when the API does not exist on that platform; `browser`/`!browser` when an app's file belongs to another binary; a library carries no destination tag (the linker drops what a binary never calls). One main per folder, no destination tag: `web/browser/main.go` → `web/public/browser.wasm`, `web/server/main.go` → `web/server/server`, `web/edge/main.go` → `web/edge/edge.wasm`.
- Code and keys in English; UI text is `lang.Text`, translations live in `lang.json`. Comments and docs: English in `webtyp/*`; in `veltylabs/*` **every README, `docs/*` file, diagram and code comment is in Spanish**, written for a reader with little technical background, and each exported symbol's comment shows a short usage example.
- A translation key is reused, never duplicated as a near-identical phrase (`"not a valid integer"` + `"not a valid decimal number"` repeats `"not a valid"` three ways instead of composing it once) — every library ships to WASM/TinyGo, so each unreused phrase is dictionary rows AND binary bytes, in every consumer, forever. A token that doesn't need translating (a kind/type name like `int`) is passed as data (a variable), never as a string literal — a literal becomes a dictionary key whether or not it needed one.
- Tests cover behaviour and domain rules, never the look (CSS, colours, markup order, class strings). In `tests/` (external package), real stack with fakes only at the edge; run `gotest`.
- Docs (`README.md`, `docs/*`) updated with the API; permanent docs never link `docs/PLAN.md`.
- Before the PR: `git diff --stat main` lists every file of the plan's stages, and nothing the plan did not order is deleted.
<!-- /webtyp:base -->

## This repo

# Agent Guide — `webtyp/agentmemory`

Constraints for agents working on this library. **Read this before any change.**
The current work order is [docs/PLAN.md](docs/PLAN.md); the master index is
[`retrieval/docs/SEMANTIC_SEARCH_MASTER_PLAN.md`](https://github.com/webtyp/retrieval/blob/main/docs/SEMANTIC_SEARCH_MASTER_PLAN.md).

---

## What this library is

The **only** implementation of `webtyp.com/agent.MemoryStore`. It exists so `webtyp/agent`
can stay free of any storage engine — see `SEMANTIC_SEARCH_MASTER_PLAN.md` D7. Concretely: `ConversationStore`,
`SummaryStore` and `ToolLogStore` run on `webtyp.com/orm` + `webtyp.com/ddl` (any
`storage.Conn`: `mem`, `sqlt`, `postgres`, `indexdb`); `KnowledgeStore` runs on
`webtyp.com/vectordb` (semantic search via an injected `embed.Embedder`).

**Its primary runtime is a browser tab compiled with TinyGo, exactly like `agent` itself.**
The host (`go test`, a Go backend) is an equally real target — this is the ONE
implementation that has to work identically on both, per D7. A change green on the host and
red under TinyGo is not done.

---

## Dependencies — exactly these, nothing else without a plan

| Import | Why |
|---|---|
| `webtyp.com/agent` | the `MemoryStore` contract this package implements |
| `webtyp.com/orm` | `ConversationStore`/`SummaryStore`/`ToolLogStore` |
| `webtyp.com/ddl` | schema declaration + `Migrate` |
| `webtyp.com/vectordb` | `KnowledgeStore` |
| `webtyp.com/embed` | only the `Embedder` **port** — injected into `vectordb.Config`, never a concrete adapter constructed here |
| `webtyp.com/model` | `model.Definition`, `model.IDGenerator` |
| `webtyp.com/storage` | `storage.Conn`, `storage.Condition` (via `orm`'s re-exports where possible) |
| `webtyp.com/fmt` | isomorphic fmt/errors/strconv/strings |

Do **not** import `webtyp.com/indexdb`, `webtyp.com/sqlt` or `webtyp.com/postgres` directly —
this package receives a `storage.Conn` already wired to one of them from its caller
(composition root). Importing a concrete backend here would hardcode this library to one
deployment target, exactly what D7 exists to prevent.

---

## The builds that define "done"

```bash
go vet ./...
gotest
gotest -tinygo
GOOS=js GOARCH=wasm go build ./...
tinygo build -target wasm -o /dev/null .
```

`gotest` alone passing means nothing here — this library's whole reason to exist is running
identically under TinyGo. `modernc.org/sqlite`, `encoding/json`, `net/http`: none of them
compile under TinyGo, and none of them belong in this repo regardless of target (see table
above).

---

## Never import these

| Never | Use instead | Why |
|---|---|---|
| `encoding/json`, `fmt`, `errors`, `strconv`, `strings` | `webtyp.com/fmt` | TinyGo/wasm size + isomorphism |
| `context` (stdlib) | `webtyp.com/context` | every webtyp API takes this one |
| `time` | `webtyp.com/time` | |
| `github.com/google/uuid` | inject `model.IDGenerator` (`webtyp.com/unixid` at the composition root) | never construct a concrete generator inside a reusable module — `model/interface.go`'s own doc comment says so |
| `map[K]V` | a slice scanned linearly, or `sort.Search` over a sorted slice | TinyGo's map runtime is a size tax on every binary that imports this; `vectordb` already proves the pattern at its scale (`vectordb/types.go`) |
| `os`, `log` | inject it | a library never touches the process environment |

---

## The two-table split — do not conflate them

`ConversationStore`/`SummaryStore`/`ToolLogStore` are SQL-shaped: fixed columns, `orm.DB`,
`ddl`-declared schema. `KnowledgeStore` is **not** a fourth table — it is `vectordb.Store`,
composed inside this package's constructor, not queried with `orm`. Do not add a `knowledge`
`model.Definition` "to keep it consistent" — `vectordb` already owns its own schema
(`vectordb.Schema()`) and its own persistence; duplicating it here is exactly the
"reimplement the port's job at the leaf" mistake `SEMANTIC_SEARCH_MASTER_PLAN.md` D7 was written to head off.

---

## Layout & tests

- `models.go` — hand-written `model.Definition` values for `Message`, `Episode`, `ToolLog`.
  The `Episode` table stores `agentcontext.Summary` (`summary.go`); the table keeps its old name so
  existing data needs no migration
  (the `definitionc` input). Run `definitionc` from the module root to generate `definition_gen.go` — **do not
  hand-write the generated file**, and do not edit it after generation (`DO NOT EDIT` header).
- `migrate.go` — `Migrate(conn ddl.Execer, compiler ddl.Compiler) error`, deploy-time schema
  reconciliation, NOT called from `New`. Pattern: `webtyp/auth`'s
  `authority/migrate.go` — read it before writing this file.
- `store.go` — `Config`, `New(ctx, cfg) (*Store, error)`, the composed `Store` type.
- One file per `agent` sub-contract implementation: `conversation.go`, `episode.go`,
  `knowledge.go`, `tool_log.go`.
- `tool_index.go` — `ToolIndex` semantic search over tools; lives in memory and does not use `vectordb` because the tool set is small and re-indexed per agent instance.
- Max 500 lines per file; split further by domain if exceeded.
- Publish with `gopush 'message'` — never `git commit`/`git push` directly.

## Common mistakes to avoid

- Constructing a concrete `storage.Conn` backend, an `embed.Embedder` adapter, or an
  `model.IDGenerator` inside this package. All three are injected via `Config` — this
  package composes, it does not choose.
- Calling schema reconciliation from `New`. `New` assumes the schema already exists
  (`Migrate` ran separately, at deploy time) — see `auth/authority/migrate.go`'s doc comment
  for the exact cost this avoids (a network round trip per model on every cold start).
- Giving `KnowledgeStore.SearchKnowledge` an AND-only session filter. A caller in session A
  must see session A's own knowledge **and** global knowledge (`sessionID == ""` at save
  time) — never another session's. `vectordb.Query.IncludeTags` is AND-only, so this needs
  two searches merged, not one — see `docs/PLAN.md` for the exact mechanism.
