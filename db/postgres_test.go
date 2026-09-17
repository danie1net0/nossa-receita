package db

import (
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
