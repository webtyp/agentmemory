package agentmemory

import (
	"webtyp.com/ddl"
	"webtyp.com/model"
	"webtyp.com/vectordb"
)

// Migrate reconciles every table this package owns — its own 3 plus vectordb's — in one
// call. Deliberately NOT called by New: schema reconciliation is deploy-time work, not
// per-process-start work.
func Migrate(conn ddl.Execer, compiler ddl.Compiler) error {
	models := append([]model.Model{
		&Message{}, &Episode{}, &ToolLog{},
	}, vectordb.Schema()...)
	sorted, err := ddl.TopologicalSort(models)
	if err != nil {
		return err
	}
	return ddl.New(conn, compiler).Sync(sorted...)
}
