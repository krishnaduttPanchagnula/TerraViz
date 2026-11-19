// Package db persists parsed states and resolved edges in an embedded SQLite
// database. Writes and reads use separate connections so a long read cannot
// block ingestion; the read connection is opened read-only and every query is
// additionally checked to be a single read-only statement.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"terraviz/resolver"
	"terraviz/state"
)

const schema = `
CREATE TABLE IF NOT EXISTS states (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE,
  filename    TEXT NOT NULL DEFAULT '',
  version     INTEGER NOT NULL,
  serial      INTEGER NOT NULL,
  lineage     TEXT NOT NULL DEFAULT '',
  raw         TEXT NOT NULL,
  ingested_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS resources (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  state_id   INTEGER NOT NULL REFERENCES states(id) ON DELETE CASCADE,
  address    TEXT NOT NULL,
  module     TEXT NOT NULL DEFAULT '',
  mode       TEXT NOT NULL DEFAULT '',
  type       TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',
  provider   TEXT NOT NULL DEFAULT '',
  attributes TEXT NOT NULL DEFAULT '{}',
  scope_kind TEXT NOT NULL DEFAULT '',
  scope_id   TEXT NOT NULL DEFAULT '',
  UNIQUE(state_id, address)
);
CREATE TABLE IF NOT EXISTS state_scopes (
  state_id INTEGER NOT NULL REFERENCES states(id) ON DELETE CASCADE,
  kind     TEXT NOT NULL,
  id       TEXT NOT NULL,
  PRIMARY KEY(state_id, kind, id)
);
CREATE TABLE IF NOT EXISTS edges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  batch       INTEGER NOT NULL,
  from_state  TEXT NOT NULL,
  from_addr   TEXT NOT NULL DEFAULT '',
  from_attr   TEXT NOT NULL DEFAULT '',
  to_state    TEXT NOT NULL,
  to_addr     TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL,
  matched     TEXT NOT NULL DEFAULT '',
  confidence  REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS edges_from ON edges(from_state);
CREATE INDEX IF NOT EXISTS edges_to ON edges(to_state);
CREATE INDEX IF NOT EXISTS edges_conf ON edges(confidence);
-- Default-threshold view, matching the CLI. For other thresholds query edges directly.
CREATE VIEW IF NOT EXISTS dependencies AS
SELECT from_state, to_state, MAX(confidence) AS confidence,
       COUNT(*) AS evidence, GROUP_CONCAT(DISTINCT kind) AS kinds
FROM edges WHERE confidence >= 0.5 GROUP BY from_state, to_state;
`

type DB struct {
	w    *sql.DB
	r    *sql.DB
	path string
}

// Open creates (or opens) the database at path and applies the schema.
func Open(path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	w, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, err
	}
	// One writer connection avoids SQLITE_BUSY under concurrent uploads.
	w.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON",
	} {
		if _, err := w.Exec(pragma); err != nil {
			w.Close()
			return nil, err
		}
	}
	if _, err := w.Exec(schema); err != nil {
		w.Close()
		return nil, err
	}
	if err := migrate(w); err != nil {
		w.Close()
		return nil, err
	}

	ro := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}).String()
	r, err := sql.Open("sqlite", ro)
	if err != nil {
		w.Close()
		return nil, err
	}
	return &DB{w: w, r: r, path: abs}, nil
}

// migrate adds columns introduced after the first release. CREATE TABLE
// IF NOT EXISTS alone cannot alter an existing table.
func migrate(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(resources)")
	if err != nil {
		return err
	}
	has := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		has[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, col := range []string{"scope_kind", "scope_id"} {
		if !has[col] {
			if _, err := db.Exec("ALTER TABLE resources ADD COLUMN " + col + " TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DB) Close() error {
	return errors.Join(d.w.Close(), d.r.Close())
}

func (d *DB) Path() string { return d.path }

// Ingest upserts each state by name (replacing its resources), then rebuilds
// the whole edge set from all stored states. All of it is one transaction, so
// readers never see a half-updated graph.
func (d *DB) Ingest(srcs []resolver.Source) error {
	tx, err := d.w.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, s := range srcs {
		clean := state.Sanitized(s.File)
		raw, err := json.Marshal(clean)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO states(name,filename,version,serial,lineage,raw)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(name) DO UPDATE SET filename=excluded.filename,
			version=excluded.version, serial=excluded.serial,
			lineage=excluded.lineage, raw=excluded.raw,
			ingested_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
			s.Name, s.Filename, clean.Version, clean.Serial, clean.Lineage, string(raw))
		if err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow("SELECT id FROM states WHERE name=?", s.Name).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM resources WHERE state_id=?", id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM state_scopes WHERE state_id=?", id); err != nil {
			return err
		}
		stmt, err := tx.Prepare(`INSERT INTO resources(state_id,address,module,mode,type,name,provider,attributes,scope_kind,scope_id)
			VALUES(?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		scopes := map[state.Scope]bool{}
		for _, r := range clean.Resources {
			for _, inst := range r.Instances {
				attrs, err := json.Marshal(inst.Attributes)
				if err != nil {
					stmt.Close()
					return err
				}
				found := state.Scopes(inst.Attributes)
				kind, sid := "", ""
				if len(found) > 0 {
					kind, sid = found[0].Kind, found[0].ID
				}
				for _, sc := range found {
					scopes[sc] = true
				}
				_, err = stmt.Exec(id, r.Address(inst.IndexKey), r.Module, r.Mode,
					r.Type, r.Name, r.Provider, string(attrs), kind, sid)
				if err != nil {
					stmt.Close()
					return err
				}
			}
		}
		stmt.Close()
		for sc := range scopes {
			if _, err := tx.Exec("INSERT OR IGNORE INTO state_scopes(state_id,kind,id) VALUES(?,?,?)",
				id, sc.Kind, sc.ID); err != nil {
				return err
			}
		}
	}

	all, err := loadSources(tx)
	if err != nil {
		return err
	}
	edges := resolver.Resolve(all)
	batch := time.Now().UnixNano()
	if _, err := tx.Exec("DELETE FROM edges"); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO edges(batch,from_state,from_addr,from_attr,
		to_state,to_addr,kind,matched,confidence) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, e := range edges {
		_, err = stmt.Exec(batch, e.From, e.FromAddr, e.FromAttr, e.To, e.ToAddr,
			e.Kind, e.Matched, e.Confidence)
		if err != nil {
			stmt.Close()
			return err
		}
	}
	stmt.Close()
	return tx.Commit()
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func loadSources(q querier) ([]resolver.Source, error) {
	rows, err := q.Query("SELECT name, raw FROM states ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resolver.Source
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		f, err := state.Parse([]byte(raw), name)
		if err != nil {
			return nil, err
		}
		out = append(out, resolver.Source{Name: name, File: f})
	}
	return out, rows.Err()
}

// ResolveAll recomputes edges from the stored states without changing them.
func (d *DB) ResolveAll() error {
	tx, err := d.w.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	all, err := loadSources(tx)
	if err != nil {
		return err
	}
	edges := resolver.Resolve(all)
	batch := time.Now().UnixNano()
	if _, err := tx.Exec("DELETE FROM edges"); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO edges(batch,from_state,from_addr,from_attr,
		to_state,to_addr,kind,matched,confidence) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range edges {
		if _, err := stmt.Exec(batch, e.From, e.FromAddr, e.FromAttr, e.To, e.ToAddr,
			e.Kind, e.Matched, e.Confidence); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type StateInfo struct {
	Name       string        `json:"name"`
	Filename   string        `json:"filename"`
	Provider   string        `json:"provider"`
	Accounts   []state.Scope `json:"accounts"`
	Version    int           `json:"version"`
	Serial     int           `json:"serial"`
	Lineage    string        `json:"lineage"`
	Resources  int           `json:"resources"`
	IngestedAt string        `json:"ingested_at"`
}

func (d *DB) States() ([]StateInfo, error) {
	out := []StateInfo{}
	rows, err := d.r.Query(`SELECT s.name, s.filename,
		CASE
			WHEN (SELECT COUNT(*) FROM resources WHERE state_id=s.id AND type GLOB 'aws_*') > 0 THEN 'aws'
			WHEN (SELECT COUNT(*) FROM resources WHERE state_id=s.id AND type GLOB 'google_*') > 0 THEN 'gcp'
			WHEN (SELECT COUNT(*) FROM resources WHERE state_id=s.id AND type GLOB 'azurerm_*') > 0 THEN 'azure'
			ELSE 'unknown'
		END,
		s.version, s.serial, s.lineage, s.ingested_at,
		(SELECT COUNT(*) FROM resources WHERE state_id=s.id)
		FROM states s ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s StateInfo
		s.Accounts = []state.Scope{}
		if err := rows.Scan(&s.Name, &s.Filename, &s.Provider, &s.Version, &s.Serial,
			&s.Lineage, &s.IngestedAt, &s.Resources); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		scopes, err := d.r.Query(`SELECT kind, id FROM state_scopes
			WHERE state_id=(SELECT id FROM states WHERE name=?) ORDER BY kind, id`, out[i].Name)
		if err != nil {
			return nil, err
		}
		for scopes.Next() {
			var sc state.Scope
			if err := scopes.Scan(&sc.Kind, &sc.ID); err != nil {
				scopes.Close()
				return nil, err
			}
			out[i].Accounts = append(out[i].Accounts, sc)
		}
		scopes.Close()
		if err := scopes.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func splitKinds(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func (d *DB) Deps(min float64) ([]resolver.Dep, error) {
	out := []resolver.Dep{}
	rows, err := d.r.Query(`SELECT from_state, to_state, MAX(confidence), COUNT(*),
		GROUP_CONCAT(DISTINCT kind) FROM edges WHERE confidence >= ?
		GROUP BY from_state, to_state ORDER BY from_state, to_state`, min)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dep resolver.Dep
		var kinds string
		if err := rows.Scan(&dep.From, &dep.To, &dep.Confidence, &dep.Evidence, &kinds); err != nil {
			return nil, err
		}
		dep.Kinds = splitKinds(kinds)
		out = append(out, dep)
	}
	return out, rows.Err()
}

func (d *DB) Edges(min float64, limit int) ([]resolver.Edge, error) {
	out := []resolver.Edge{}
	q := `SELECT from_state,from_addr,from_attr,to_state,to_addr,kind,matched,confidence
		FROM edges WHERE confidence >= ?
		ORDER BY confidence DESC, from_state, to_state`
	args := []any{min}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := d.r.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e resolver.Edge
		if err := rows.Scan(&e.From, &e.FromAddr, &e.FromAttr, &e.To, &e.ToAddr,
			&e.Kind, &e.Matched, &e.Confidence); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ResourceInfo is one managed/data resource instance, with its owning state.
type ResourceInfo struct {
	State     string `json:"state"`
	Address   string `json:"address"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	ScopeKind string `json:"scope_kind"`
	ScopeID   string `json:"scope_id"`
}

// Resources returns every resource instance, or just one state's when state
// is non-empty.
func (d *DB) Resources(state string) ([]ResourceInfo, error) {
	out := []ResourceInfo{}
	rows, err := d.r.Query(`SELECT s.name, r.address, r.type, r.name, r.mode, r.scope_kind, r.scope_id
		FROM resources r JOIN states s ON s.id = r.state_id
		WHERE ? = '' OR s.name = ?
		ORDER BY s.name, r.address`, state, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ResourceInfo
		if err := rows.Scan(&r.State, &r.Address, &r.Type, &r.Name, &r.Mode,
			&r.ScopeKind, &r.ScopeID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type Node struct {
	ID        string `json:"id"`
	Resources int    `json:"resources"`
}

type GraphEdge struct {
	From       string   `json:"from"`
	To         string   `json:"to"`
	Confidence float64  `json:"confidence"`
	Evidence   int      `json:"evidence"`
	Kinds      []string `json:"kinds"`
}

type Graph struct {
	Nodes []Node      `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

func (d *DB) Graph(min float64) (Graph, error) {
	g := Graph{Nodes: []Node{}, Edges: []GraphEdge{}}
	rows, err := d.r.Query(`SELECT name, (SELECT COUNT(*) FROM resources WHERE state_id=states.id)
		FROM states ORDER BY name`)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Resources); err != nil {
			rows.Close()
			return g, err
		}
		g.Nodes = append(g.Nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return g, err
	}

	rows, err = d.r.Query(`SELECT from_state,to_state,MAX(confidence),COUNT(*),
		GROUP_CONCAT(DISTINCT kind) FROM edges WHERE confidence >= ?
		GROUP BY from_state,to_state ORDER BY from_state,to_state`, min)
	if err != nil {
		return g, err
	}
	defer rows.Close()
	for rows.Next() {
		var e GraphEdge
		var kinds string
		if err := rows.Scan(&e.From, &e.To, &e.Confidence, &e.Evidence, &kinds); err != nil {
			return g, err
		}
		e.Kinds = splitKinds(kinds)
		g.Edges = append(g.Edges, e)
	}
	return g, rows.Err()
}

type Result struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

func norm(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		return v
	}
}

func stripSQLComments(s string) string {
	var b strings.Builder
	var quote byte // 0, ', " or `
	inLine, inBlock := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		n := byte(0)
		if i+1 < len(s) {
			n = s[i+1]
		}
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == quote {
				if n == quote { // doubled quote: escaped inside the literal
					b.WriteByte(n)
					i++
				} else {
					quote = 0
				}
			}
		case inLine:
			if c == '\n' {
				inLine = false
				b.WriteByte(c)
			}
		case inBlock:
			if c == '*' && n == '/' {
				inBlock = false
				i++
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
			b.WriteByte(c)
		case c == '-' && n == '-':
			inLine = true
			i++
		case c == '/' && n == '*':
			inBlock = true
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// IsReadOnly rejects writes, multiple statements and non-query pragmas. It is
// a guardrail; the real enforcement is the mode=ro connection used by Query.
func IsReadOnly(q string) error {
	s := strings.TrimSpace(stripSQLComments(q))
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("empty SQL")
	}
	if strings.Contains(s, ";") {
		return errors.New("only one statement is allowed")
	}
	first := strings.ToUpper(strings.Fields(s)[0])
	switch first {
	case "SELECT", "WITH", "EXPLAIN", "VALUES":
		return nil
	default:
		return fmt.Errorf("read-only query required, got %s", first)
	}
}

func (d *DB) Query(q string) (Result, error) {
	res := Result{Columns: []string{}, Rows: [][]any{}}
	if err := IsReadOnly(q); err != nil {
		return res, err
	}
	rows, err := d.r.Query(q)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return res, err
	}
	res.Columns = cols
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return res, err
		}
		row := make([]any, len(cols))
		for i, v := range vals {
			row[i] = norm(v)
		}
		res.Rows = append(res.Rows, row)
	}
	return res, rows.Err()
}
