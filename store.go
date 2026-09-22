package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/embed"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/storage"
	"webtyp.com/vectordb"
)

// Config is the data a caller assembles once, at composition-root time. Every field is
// injected — this package constructs no concrete storage.Conn, no concrete Embedder, no
// concrete IDGenerator.
type Config struct {
	Conn      storage.Conn      // required — same Conn Migrate ran DDL against
	Embedder  embed.Embedder    // required — text → vectors for KnowledgeStore
	IDGen     model.IDGenerator // required
	ShardSize int               // vectordb passthrough; 0 → vectordb's default (1024)
	MaxDocs   int               // vectordb passthrough; 0 → unbounded
	MaxBytes  int64             // vectordb passthrough; 0 → derive from navigator.storage.estimate()
}

type Store struct {
	db    *orm.DB
	vdb   *vectordb.Store
	idGen model.IDGenerator
}

// New assembles a Store. It does NOT reconcile schema — call Migrate first, once, at deploy
// time.
func New(ctx *context.Context, cfg Config) (*Store, error) {
	if cfg.Conn == nil || cfg.Embedder == nil || cfg.IDGen == nil {
		return nil, fmt.Err("agentmemory: Conn, Embedder and IDGen are all required")
	}
	vdb, err := vectordb.New(ctx, vectordb.Config{
		Conn:      cfg.Conn,
		Embedder:  cfg.Embedder,
		IDGen:     cfg.IDGen,
		ShardSize: cfg.ShardSize,
		MaxDocs:   cfg.MaxDocs,
		MaxBytes:  cfg.MaxBytes,
	})
	if err != nil {
		return nil, fmt.Err("agentmemory: creating vectordb.Store: ", err)
	}
	return &Store{
		db:    orm.New(cfg.Conn),
		vdb:   vdb,
		idGen: cfg.IDGen,
	}, nil
}

// Ensure Store will implement agent.MemoryStore when all methods are defined.
var _ agent.MemoryStore = (*Store)(nil)
