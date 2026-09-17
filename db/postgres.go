package db

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	keyFieldName   = "key"
	valueFieldName = "value"
)

//go:embed postgres
var sql embed.FS

type sqlTemplate struct {
	path         fs.DirEntry
	embeddedPath string
	key          string
}

func (s *sqlTemplate) render(p *PostgreSQL) (string, error) {
	t, err := template.ParseFS(sql, s.embeddedPath)
	if err != nil {
		return "", fmt.Errorf("error parsing %s template: %w", s.path, err)
	}
	var b bytes.Buffer
	if err = t.Execute(&b, p); err != nil {
		return "", fmt.Errorf("error rendering %s template: %w", s.path, err)
	}
	return b.String(), nil

}

func newSQLTemplate(f fs.DirEntry) sqlTemplate {
	return sqlTemplate{
		path:         f,
		embeddedPath: "postgres/" + f.Name(),
		key:          strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())),
	}
}

// PostgreSQL database interface.
type PostgreSQL struct {
	pool           *pgxpool.Pool
	uri            string
	schema         string
	MetaTableName  string
	KeyFieldName   string
	ValueFieldName string
	Logged         bool
}

func (p *PostgreSQL) renderTemplate(key string) (string, error) {
	ls, err := sql.ReadDir("postgres")
	if err != nil {
		return "", fmt.Errorf("error looking for templates: %w", err)
	}
	for _, f := range ls {
		s := newSQLTemplate(f)
		if s.key != key {
			continue
		}
		return s.render(p)
	}
	return "", fmt.Errorf("template %s not found", key)
}

// Close closes the PostgreSQL connection
func (p *PostgreSQL) Close() { p.pool.Close() }

// MetaTableFullName is the name of the schame and table in dot-notation.
func (p *PostgreSQL) MetaTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.MetaTableName)
}

// Create creates the required database table.
func (p *PostgreSQL) Create() error {
	slog.Info("Creating tables")
	s, err := p.renderTemplate("create")
	if err != nil {
		return fmt.Errorf("error rendering create template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error creating table with: %s\n%w", s, err)
	}
	return nil
}

// Drop drops the database table created by `Create`.
func (p *PostgreSQL) Drop() error {
	slog.Info("Dropping tables")
	s, err := p.renderTemplate("drop")
	if err != nil {
		return fmt.Errorf("error rendering drop template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error dropping table with: %s\n%w", s, err)
	}
	return nil
}

// CopyRelational bulk-loads rows into one of the relational source-of-truth
// tables (migration phase 2) via COPY.
func (p *PostgreSQL) CopyRelational(ctx context.Context, table string, columns []string, rows [][]any) error {
	if _, err := p.pool.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("error copying %d rows to %s: %w", len(rows), table, err)
	}
	return nil
}

// CNPJsByBase returns, for each given CNPJ base, the CNPJs of its
// establishments in table (estabelecimento or its staging counterpart).
func (p *PostgreSQL) CNPJsByBase(ctx context.Context, table string, bases []string) (map[string][]string, error) {
	out := make(map[string][]string, len(bases))
	rows, err := p.pool.Query(ctx, "SELECT cnpj_base, cnpj FROM "+pgx.Identifier{table}.Sanitize()+" WHERE cnpj_base = ANY($1)", bases)
	if err != nil {
		return nil, fmt.Errorf("could not query %s by base: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var base, cnpj string
		if err := rows.Scan(&base, &cnpj); err != nil {
			return nil, fmt.Errorf("could not read %s by base: %w", table, err)
		}
		out[base] = append(out[base], cnpj)
	}
	return out, rows.Err()
}

// splitStatements breaks a rendered SQL script into its statements, dropping
// full-line comments. The scripts it is used on have no semicolons inside
// literals or bodies.
func splitStatements(s string) []string {
	var b strings.Builder
	for l := range strings.Lines(s) {
		if strings.HasPrefix(strings.TrimSpace(l), "--") {
			continue
		}
		b.WriteString(l)
	}
	var stmts []string
	for part := range strings.SplitSeq(b.String(), ";") {
		if t := strings.TrimSpace(part); t != "" {
			stmts = append(stmts, t)
		}
	}
	return stmts
}

// execStatements runs each statement of a script on its own (autocommit), on a
// single connection so session settings such as maintenance_work_mem carry
// over. Sending the whole script in one Exec makes it a single implicit
// transaction: every table rewrite (SET LOGGED/UNLOGGED) then keeps its old copy
// on disk until the very end, adding up to roughly twice the database size, and
// a failure in the last statement rolls back hours of work.
func (p *PostgreSQL) execStatements(ctx context.Context, s string) error {
	c, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("could not acquire a database connection: %w", err)
	}
	defer c.Release()
	for _, stmt := range splitStatements(s) {
		slog.Debug("running statement", "sql", stmt)
		if _, err := c.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// PreLoad runs before starting to load data into the database: it makes the
// tables unlogged and drops the secondary indexes.
func (p *PostgreSQL) PreLoad() error {
	s, err := p.renderTemplate("pre_load")
	if err != nil {
		return fmt.Errorf("error rendering pre-load template: %w", err)
	}
	if err := p.execStatements(context.Background(), s); err != nil {
		return fmt.Errorf("error during pre load: %w", err)
	}
	return nil
}

// PostLoad runs after loading data into the database: it makes the tables
// logged again and rebuilds the secondary indexes.
func (p *PostgreSQL) PostLoad() error {
	s, err := p.renderTemplate("post_load")
	if err != nil {
		return fmt.Errorf("error rendering post-load template: %w", err)
	}
	if err := p.execStatements(context.Background(), s); err != nil {
		return fmt.Errorf("error during post load: %w", err)
	}
	return nil
}

// MetaSave saves a key/value pair in the metadata table.
func (p *PostgreSQL) MetaSave(k, v string) error {
	if len(k) > 16 {
		return fmt.Errorf("metatable can only take keys that are at maximum 16 chars long")
	}
	s, err := p.renderTemplate("meta_save")
	if err != nil {
		return fmt.Errorf("error rendering meta-save template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s, k, v); err != nil {
		return fmt.Errorf("error saving %s to metadata: %w", k, err)
	}
	return nil
}

// NewPostgreSQL creates a new PostgreSQL connection and ping it to make sure it works.
func NewPostgreSQL(uri, schema string, logged bool) (PostgreSQL, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("could not create database config: %w", err)
	}
	cfg.MaxConns = 16
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute
	conn, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("could not connect to the database: %w", err)
	}
	p := PostgreSQL{
		pool:           conn,
		uri:            uri,
		schema:         schema,
		MetaTableName:  metaTableName,
		KeyFieldName:   keyFieldName,
		ValueFieldName: valueFieldName,
		Logged:         logged,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.pool.Ping(ctx); err != nil {
		return PostgreSQL{}, fmt.Errorf("could not connect to postgres: %w", err)
	}
	return p, nil
}
