---
PLAN: "feat: agentmemory — MemoryStore over orm+ddl (SQL) and vectordb (semantic knowledge)"
TAG: v0.1.0
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 8513538180190184454
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Índice maestro: https://github.com/webtyp/agent/blob/main/docs/MASTER_PLAN.md — fase 4,
> D7. Depende de `webtyp/agent`'s propio plan de segregación
> (`webtyp/agent` PR de "MemoryStore segregado en 4 contratos") — **si ese PR no está
> mergeado todavía, esperá**: este repo importa `webtyp.com/agent` y necesita los 4 contratos
> nuevos (`ConversationStore`, `EpisodeStore`, `KnowledgeStore`, `ToolLogStore`), no la
> interfaz plana vieja.
>
> **Nota de idioma:** la prosa va en español; los bloques de código mantienen sus comentarios
> en inglés.

# Plan — `webtyp/agentmemory`

## Responsabilidad única

La **única** implementación de `agent.MemoryStore`. `ConversationStore`/`EpisodeStore`/
`ToolLogStore` corren sobre `webtyp.com/orm` + `webtyp.com/ddl` (cualquier `storage.Conn`).
`KnowledgeStore` corre sobre `webtyp.com/vectordb` (búsqueda semántica real, vía un
`embed.Embedder` inyectado — nunca uno construido acá, ver `AGENTS.md`).

## Design gate

**1. Prior art.** `webtyp/auth` es el mejor precedente interno: modelos declarados a mano en
`models.go` (`model.Definition`), generados con `ormc`, migrados con un `Migrate(conn,
compiler)` separado de `New` (`authority/migrate.go`) — este plan copia esa forma exacta,
tabla por tabla. Para la parte semántica, `vectordb` ya resuelve documentos+kNN+filtros; este
plan lo compone, no lo reimplementa.

**2. Novice-name test.** `agentmemory.New(ctx, Config{Conn, Embedder, IDGen}) (*Store,
error)` y `agentmemory.Migrate(conn, compiler) error` — mismos verbos que `auth`, mismo
patrón `Config` inyectado que `vectordb.Config`. Un desarrollador que ya usó cualquiera de
los dos no tiene nada nuevo que aprender.

**3. Complexity ledger.**
```
Conceptos nuevos para el desarrollador     +1 (este repo — pero es la ranura vacía que
                                               MASTER_PLAN.md fase 4 ya reservaba)
Formas de implementar agent.MemoryStore     2 (agent.NewMemMemory, de referencia, y esta,
                                               real — ninguna otra debería existir)
Tablas SQL nuevas                           3 (message, episode, tool_log — knowledge no es
                                               una tabla propia, ver "El corte" abajo)
```

**4. Dónde vive.** Acá — es exactamente el repo que `MASTER_PLAN.md` §5 reserva para esto.

**5. Qué borra.** Nada existente — cierra una ranura vacía. Hacia adelante, es lo único que
puede implementar `agent.MemoryStore` en producción; `agent.NewMemMemory()` (el backend de
referencia) no se usa fuera de los tests de `agent` mismo.

## El corte: 3 tablas SQL + `vectordb`, no 4 tablas

`Knowledge` no es una tabla que este repo declare. Es `vectordb.Store` — sus propios
`model.Model` (`vectordb.Schema()`) y su propia persistencia (documentos, arena de vectores,
shards). Este repo **compone** un `*vectordb.Store` dentro de su `Store`, no le agrega una
tabla paralela. Si te encontrás escribiendo un `KnowledgeModel`, pará: es la reimplementación
que D7 del índice maestro existe para prevenir.

## Cambio 1 — `models.go`: los 3 modelos SQL, a mano, para `ormc`

```go
package agentmemory

import "webtyp.com/model"

var MessageModel = model.Definition{
	Name: "message",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "role", Type: model.Text()},
		{Name: "content", Type: model.Text()},
		{Name: "tool_name", Type: model.Text(), OmitEmpty: true},
		{Name: "tool_call_id", Type: model.Text(), OmitEmpty: true},
		{Name: "tool_calls", Type: model.Text(), OmitEmpty: true}, // netstring-encoded, Cambio 3
		{Name: "token_count", Type: model.Int()},
		{Name: "created_at", Type: model.Int()},
	},
}

var EpisodeModel = model.Definition{
	Name: "episode",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "summary", Type: model.Text()},
		{Name: "token_count", Type: model.Int()},
		{Name: "from_msg_id", Type: model.Text()},
		{Name: "to_msg_id", Type: model.Text()},
		{Name: "created_at", Type: model.Int()},
	},
}

var ToolLogModel = model.Definition{
	Name: "tool_log",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "tool_name", Type: model.Text()},
		{Name: "input_json", Type: model.Text()},
		{Name: "output_text", Type: model.Text(), OmitEmpty: true},
		{Name: "err_text", Type: model.Text(), OmitEmpty: true},
		{Name: "duration_ms", Type: model.Int()},
		{Name: "created_at", Type: model.Int()},
	},
}
```

Ningún campo es `PK: true` más de una vez por modelo, ningún `Ref` — las tres tablas son
independientes entre sí (no hay FK: `session_id` es un string libre, no una tabla de
sesiones — el índice maestro nunca pide una, y `agent.NewMemMemory` tampoco la tiene).

Corré `ormc` desde la raíz del módulo. Genera `models_orm.go` — **no lo edites a mano**, es
`DO NOT EDIT`.

## Cambio 2 — `migrate.go`: reconciliación de esquema, separada de `New`

Copiá el patrón exacto de
[`auth/authority/migrate.go`](https://github.com/webtyp/auth/blob/main/authority/migrate.go)
— léelo primero, es corto. Adaptado a este repo:

```go
package agentmemory

import (
	"webtyp.com/ddl"
	"webtyp.com/model"
	"webtyp.com/vectordb"
)

// Migrate reconciles every table this package owns — its own 3 plus vectordb's — in one
// call. Deliberately NOT called by New: schema reconciliation is deploy-time work, not
// per-process-start work (same reasoning as auth/authority/migrate.go's doc comment — read
// it, don't re-derive it).
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
```

`&Message{}` etc. son los structs que `ormc` genera en `models_orm.go` a partir de
`MessageModel` — no los declares vos, ya existen después del Cambio 1.

## Cambio 3 — `tool_calls.go`: codec netstring para `[]agent.ToolCall`

`Message.ToolCalls` es `[]agent.ToolCall{ID, Name, Input}` — una lista, no cabe en una
columna de ancho fijo. `webtyp.com/json` sólo sabe codificar tipos `model.Encodable`
(el patrón `Fielder`), y `ToolCall` no lo es ni debería serlo por esto — así que esto se
codifica a mano, con framing por longitud (estilo netstring) para que ningún carácter dentro
de `Input` (que es JSON crudo, puede contener cualquier cosa) rompa el parseo:

```
por cada ToolCall, en orden, sin separador ENTRE calls (el framing ya lo delimita):
  "<len(ID)>:<ID>,<len(Name)>:<Name>,<len(Input)>:<Input>,"
```

Ejemplo, una llamada `{ID:"c1", Name:"search", Input:`{"q":"x"}`}`:

```
2:c1,6:search,10:{"q":"x"},
```

```go
package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/fmt"
)

// encodeToolCalls serializes calls into the netstring-framed string Message.tool_calls
// stores. Empty input returns "".
func encodeToolCalls(calls []agent.ToolCall) string {
	var b fmt.Builder
	for _, c := range calls {
		writeField(&b, c.ID)
		writeField(&b, c.Name)
		writeField(&b, c.Input)
	}
	return b.String() // check the exact method fmt.Conv/Builder exposes to read the
	                   // accumulated string — String() or a similarly named getter; adjust
	                   // if the real API differs, the FRAMING is what must not change.
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
	if end > len(s) || s[end] != ',' {
		return "", 0, fmt.Err("agentmemory: field length mismatch or missing trailing ',' at offset ", end)
	}
	return s[start:end], end + 1, nil
}
```

Verificá el nombre exacto del método que `fmt.Builder`/`fmt.Conv` (`webtyp.com/fmt/builder.go`)
expone para leer el contenido acumulado (`String()` u otro) — ese detalle puede diferir de
lo escrito arriba; lo que **no** puede cambiar es el formato de framing, porque
`TestToolCalls_RoundTrip` (Tests, abajo) lo fija byte a byte.

## Cambio 4 — `store.go`: `Config`, `New`, el tipo `Store`

```go
package agentmemory

import (
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
// concrete IDGenerator (see AGENTS.md "Common mistakes to avoid").
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
// time (see migrate.go's doc comment for why).
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

// Ensure Store implements the full contract at compile time.
var _ interface {
	// (spelled out so a missing method fails HERE, not at some far-off call site)
} = (*Store)(nil)
```

Ese último bloque `var _ interface{...} = (*Store)(nil)` — reemplazalo por
`var _ agent.MemoryStore = (*Store)(nil)` una vez que `conversation.go`/`episode.go`/
`knowledge.go`/`tool_log.go` (Cambios 5–8) existan; ponerlo antes de que los 11 métodos
existan sólo generaría un error de compilación que no dice nada nuevo. Importá
`webtyp.com/agent` en este archivo recién en ese momento.

## Cambio 5 — `conversation.go`: `ConversationStore`

```go
package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/storage"
)

// EnsureSession is a no-op validation, not a table write. There is no sessions table — every
// row of message/episode/tool_log just carries a free-form session_id string (same design
// agent.NewMemMemory uses, and the same one MASTER_PLAN.md's memory categorization assumed
// all along: "session_id IS NULL" scoping, never a sessions table).
func (s *Store) EnsureSession(ctx *context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Err("agentmemory: sessionID must not be empty")
	}
	return nil
}

func (s *Store) AppendMessage(ctx *context.Context, sessionID string, msg agent.Message) error {
	id := msg.ID
	if id == "" {
		id = s.idGen.NewID()
	}
	m := &Message{
		Id:         id,
		SessionId:  sessionID,
		Role:       msg.Role,
		Content:    msg.Content,
		ToolName:   msg.ToolName,
		ToolCallId: msg.ToolCallID,
		ToolCalls:  encodeToolCalls(msg.ToolCalls),
		TokenCount: int64(msg.TokenCount),
		CreatedAt:  msg.CreatedAt,
	}
	return s.db.Create(m)
}

func (s *Store) GetMessages(ctx *context.Context, sessionID string, limit int) ([]agent.Message, error) {
	var rows MessageList
	err := s.db.Query(&Message{}).
		Where("session_id").Eq(sessionID).
		OrderBy("created_at").Desc().
		Limit(limit).
		ReadAll(func() model.Model { return &Message{} }, func(m model.Model) { rows = append(rows, m.(*Message)) })
	if err != nil {
		return nil, fmt.Err("agentmemory: GetMessages: ", err)
	}
	// rows come back newest-first (ORDER BY ... DESC); a conversation reads oldest-first.
	out := make([]agent.Message, len(rows))
	for i, r := range rows {
		calls, err := decodeToolCalls(r.ToolCalls)
		if err != nil {
			return nil, fmt.Err("agentmemory: GetMessages: ", err)
		}
		out[len(rows)-1-i] = agent.Message{
			ID: r.Id, SessionID: r.SessionId, Role: r.Role, Content: r.Content,
			ToolName: r.ToolName, ToolCallID: r.ToolCallId, ToolCalls: calls,
			TokenCount: int(r.TokenCount), CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (s *Store) DeleteMessages(ctx *context.Context, sessionID string, ids []string) error {
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	return s.db.Delete(&Message{}, storage.Eq("session_id", sessionID), storage.In("id", idsAny))
}
```

**Verificá la firma real de `orm.QB.ReadAll`** (`orm/qb.go:133`) contra lo escrito arriba —
el segundo argumento (`onRow func(model.Model)`) y el primero (`new func() model.Model`)
están citados de memoria del código ya leído; si el orden o los nombres de parámetro
difieren, seguí el código real de `orm/qb.go`, no esta plantilla. Mismo cuidado con
`db.Delete(m, cond, rest ...storage.Condition)` (`orm/db.go:202`): confirmá que acepta
múltiples condiciones ANDed antes de asumirlo.

**`Query(&Message{}).Where("session_id")` usa el nombre de **columna**, no el de campo Go**
(`session_id`, no `SessionId`) — así lo hace `qb.go`'s `Clause` (recibe un `column string`).
Usá el helper generado `Message_.SessionId` (`ormc` genera ese struct de nombres de columna,
mismo patrón que `User_.Email` en `auth/models_orm.go`) en vez de escribir el string a mano,
para que un rename de columna rompa la build en vez de fallar en silencio en runtime.

## Cambio 6 — `episode.go`: `EpisodeStore`

Mismo patrón exacto que `conversation.go` — `SaveEpisode` es un `Create`, `GetEpisodes` es un
`Query().Where("session_id").Eq(...).OrderBy("created_at").Desc().Limit(limit).ReadAll(...)`
con la misma inversión a orden cronológico antes de devolver. No hay `Delete` en
`EpisodeStore` (el contrato no lo pide).

## Cambio 7 — `knowledge.go`: `KnowledgeStore`, sobre `vectordb`

**El problema que este archivo resuelve:** `vectordb.Query.IncludeTags` es AND — no puede
expresar "global O de mi sesión" en una sola llamada. La solución es dos búsquedas, fusionadas
por score.

**Tags al guardar:** `SaveKnowledge` etiqueta el documento con `"global"` si `sessionID ==
""`, o con `"session:" + sessionID` si no. El `source` y el momento de creación no tienen
columna en `vectordb.Doc` (sólo `ID, Text, Meta, Tags`) — van en `Meta` como JSON armado a
mano (dos campos, no hace falta `model.Encodable` para esto):

```go
package agentmemory

import (
	"webtyp.com/agent"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/time"
	"webtyp.com/vectordb"
)

const tagGlobal = "global"
const sessionTagPrefix = "session:"

func (s *Store) SaveKnowledge(ctx *context.Context, sessionID, content, source string) error {
	tag := tagGlobal
	if sessionID != "" {
		tag = sessionTagPrefix + sessionID
	}
	meta := fmt.Sprintf(`{"source":%s,"created_at":%d}`, fmt.Sprintf("%q", source), time.Now().Unix())
	_, err := s.vdb.Add(ctx, vectordb.Doc{Text: content, Meta: meta, Tags: []string{tag}})
	if err != nil {
		return fmt.Err("agentmemory: SaveKnowledge: ", err)
	}
	return nil
}

func (s *Store) SearchKnowledge(ctx *context.Context, query, sessionID string, limit int) ([]agent.Knowledge, error) {
	globalMatches, err := s.vdb.Search(ctx, vectordb.Query{Text: query, K: limit, IncludeTags: []string{tagGlobal}})
	if err != nil {
		return nil, fmt.Err("agentmemory: SearchKnowledge (global): ", err)
	}

	var sessionMatches []vectordb.Match
	if sessionID != "" {
		sessionMatches, err = s.vdb.Search(ctx, vectordb.Query{Text: query, K: limit, IncludeTags: []string{sessionTagPrefix + sessionID}})
		if err != nil {
			return nil, fmt.Err("agentmemory: SearchKnowledge (session): ", err)
		}
	}

	merged := mergeMatchesByScore(globalMatches, sessionMatches, limit)
	out := make([]agent.Knowledge, len(merged))
	for i, m := range merged {
		src, createdAt := parseKnowledgeMeta(m.Meta)
		out[i] = agent.Knowledge{ID: m.ID, SessionID: sessionID, Content: m.Text, Source: src, CreatedAt: createdAt}
	}
	return out, nil
}
```

`mergeMatchesByScore(a, b []vectordb.Match, limit int) []vectordb.Match` — concatená, ordená
por `Score` descendente (sort estable, `sort.SliceStable`), truncá a `limit`. Sin `map[K]V`.

`parseKnowledgeMeta(meta string) (source string, createdAt int64)` — parseo manual del JSON de
dos campos armado arriba (`{"source":"...","created_at":123}`); no traigas
`encoding/json`/`webtyp.com/json` para esto, es un formato fijo que este mismo archivo
produce. Si preferís no escribir un parser de JSON a mano, la alternativa consistente con el
resto del ecosistema es NO usar JSON para `Meta` — un formato propio de dos campos
(netstring, como el Cambio 3) es igual de válido y más corto de escribir; elegí uno y
documentalo en el comentario de `SaveKnowledge`, pero sea cual sea, **tiene que ser el mismo
formato en las dos puntas** (mismo archivo, así que es difícil errar esto, pero decilo
explícito para quien lea el diff).

**`agent.Knowledge.SessionID`** en el resultado es el `sessionID` **del que buscó**, no el
del documento — el contrato no distingue si un resultado vino del filtro global o del de
sesión, y no hace falta: el llamador ya sabe en qué sesión está buscando.

## Cambio 8 — `tool_log.go`: `ToolLogStore`

Mismo patrón que `episode.go`, con un filtro adicional opcional: si `toolName != ""`,
`GetToolLogs` agrega `.Where("tool_name").Eq(toolName)` a la query (ANDed con el filtro de
`session_id`, vía `Where(...).Eq(...)` seguido de otro `Where(...).Eq(...)` — confirmá en
`orm/qb.go` si dos `Where` consecutivos se ANDan automáticamente o si hace falta encadenar
distinto; el código real de `qb.go` manda sobre esta frase).

## Tests

Sin bajar un modelo real ni un backend real: `mem` (o `sqlt`/`:memory:`, lo que ya tenga
conformance más simple de armar en este repo) para SQL, y `embed.MockEmbedder` (ya existe,
`webtyp.com/embed`) para `vectordb` — determinístico, sin red, sin artifact.

| Test | Verifica |
|---|---|
| `TestToolCalls_RoundTrip` | `decodeToolCalls(encodeToolCalls(x)) == x` para: cero llamadas, una, varias, y una con `Input` conteniendo `,`, `:` y dígitos — el caso que un delimitador ingenuo rompería |
| `TestAgentMemoryConformance` | `conformance.Run` (de `webtyp.com/agent/conformance` — el paquete que el plan de `agent` deja publicado) contra un `*Store` real, con `Conn` de `storage/mem` (o el backend que ya tenga fixture en este repo) y `embed.MockEmbedder` |
| `TestMigrate_CreatesAllTables` | `Migrate` corrido dos veces no falla (idempotente — es lo que `ddl.Sync` ya garantiza; este test lo confirma para el conjunto de 3 tablas + `vectordb.Schema()`, no para `ddl` en sí) |

`TestAgentMemoryConformance` es la puerta real de esta fase — si pasa, `agent.MemoryStore`
tiene una implementación de producción funcionando sobre SQL. La mitad de `indexdb` de esa
misma puerta (`MASTER_PLAN.md` fase 4: "esa suite pasa contra un backend SQL y contra
indexdb") corre bajo `gotest -tinygo`/`tinygo test -target wasm` con `storage/indexdb` como
`Conn` — mismo test, mismo `TestAgentMemoryConformance`, el `Conn` que cambia según el target
de build, no el código de test.

## Checklist de aceptación

```bash
go vet ./...
gotest
gotest -tinygo
GOOS=js GOARCH=wasm go build ./...
tinygo build -target wasm -o /dev/null .
grep -rn "map\[" --include="*.go" . | grep -v _test.go                        # → vacío
grep -rn '"encoding/json"\|"errors"\|"strconv"' --include="*.go" . | grep -v _test.go   # → vacío
go list -m all | grep -i sqlite                                                # → vacío (no linkea un motor concreto)
```
