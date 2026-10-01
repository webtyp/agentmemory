# agentmemory
<img src="docs/img/badges.svg">

MemoryStore implementation for webtyp/agent over orm and ddl: runs on SQL backends and on IndexedDB

## Usage

### Tool Index

Find tools by meaning: `agentmemory.NewToolIndex(embedder)` (bekko is the embedder the browser uses).

```go
idx, err := agentmemory.NewToolIndex(embedder)
if err != nil {
	return err
}
ag, err := agent.New(agent.Config{
	ToolIndex: idx,
})
```

## Documentation

- [Agent guide](AGENTS.md): rules for anyone changing this library.
- [History: SQLite memory study](docs/history/MEMORY_SQLITE.md): superseded. The earlier SQLite +
  `sqlite-vec` design, kept for its memory categories (short-term, episodic, semantic, action),
  its session scoping and its RRF reasoning.
