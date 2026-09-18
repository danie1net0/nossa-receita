package db

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
)

func setUpPostgres() (*PostgreSQL, error) {
	u := os.Getenv("TEST_POSTGRES_URL")
	if u == "" {
		return nil, fmt.Errorf("expected a posgres uri at TEST_POSTGRES_URL, found nothing")
	}
	db, err := NewPostgreSQL(u, "public", false)
	if err != nil {
		return nil, fmt.Errorf("expected no error connecting to postgres, got %w", err)
	}
	if err := db.Drop(); err != nil {
		return nil, fmt.Errorf("expected no error dropping the tables, got %w", err)
	}
	if err := db.Create(); err != nil {
		return nil, fmt.Errorf("expected no error creating the tables, got %w", err)
	}
	if err := db.PreLoad(); err != nil {
		return nil, fmt.Errorf("expected no error pre load on postgres, got %w", err)
	}
	if err := db.PostLoad(); err != nil {
		return nil, fmt.Errorf("expected no error post load on postgres, got %w", err)
	}
	return &db, nil
}

func TestSplitStatements(t *testing.T) {
	s := `
SET maintenance_work_mem TO '512MB';

-- a comment; with a semicolon
ALTER TABLE cnpj SET LOGGED;
CREATE INDEX IF NOT EXISTS idx_a
    ON a (b);
`
	got := splitStatements(s)
	exp := []string{
		"SET maintenance_work_mem TO '512MB'",
		"ALTER TABLE cnpj SET LOGGED",
		"CREATE INDEX IF NOT EXISTS idx_a\n    ON a (b)",
	}
	if !slices.Equal(got, exp) {
		t.Errorf("expected %q, got %q", exp, got)
	}
}

func TestLoadDropsAndRestoresPrimaryKeys(t *testing.T) {
	pg, err := setUpPostgres()
	if err != nil {
		t.Fatalf("expected no error setting up postgres, got %s", err)
	}
	defer func() {
		if err := pg.Drop(); err != nil {
			t.Errorf("expected no error dropping the tables, got %s", err)
		}
		pg.Close()
	}()
	tables := []string{"empresa", "estabelecimento", "simples", "socio", "estabelecimento_cnae", "regime_tributario"}
	count := func() int {
		t.Helper()
		var n int
		q := "SELECT count(*) FROM pg_constraint WHERE contype = 'p' AND conrelid::regclass::text = ANY($1)"
		if err := pg.pool.QueryRow(context.Background(), q, tables).Scan(&n); err != nil {
			t.Fatalf("expected no error counting primary keys, got %s", err)
		}
		return n
	}
	if n := count(); n != len(tables) {
		t.Errorf("expected %d primary keys after post load, got %d", len(tables), n)
	}
	if err := pg.PreLoad(); err != nil {
		t.Fatalf("expected no error on pre load, got %s", err)
	}
	if n := count(); n != 0 {
		t.Errorf("expected no primary keys during the load, got %d", n)
	}
	if err := pg.PostLoad(); err != nil {
		t.Fatalf("expected no error on post load, got %s", err)
	}
	if n := count(); n != len(tables) {
		t.Errorf("expected %d primary keys after post load, got %d", len(tables), n)
	}
}
