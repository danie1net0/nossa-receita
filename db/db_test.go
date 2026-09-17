package db

import (
	"context"
	"testing"
)

func TestCreateMetaAndDescriptions(t *testing.T) {
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
	ctx := context.Background()

	readMeta := func(k string) string {
		t.Helper()
		var v string
		if err := pg.pool.QueryRow(ctx, "SELECT value FROM meta WHERE key = $1", k).Scan(&v); err != nil {
			t.Errorf("expected no error getting metadata, got %s", err)
		}
		return v
	}
	if err := pg.MetaSave("answer", "42"); err != nil {
		t.Errorf("expected no error writing to the metadata table, got %s", err)
	}
	if v := readMeta("answer"); v != "42" {
		t.Errorf("expected 42 as the answer, got %s", v)
	}
	if err := pg.MetaSave("answer", "forty-two"); err != nil {
		t.Errorf("expected no error re-writing to the metadata table, got %s", err)
	}
	if v := readMeta("answer"); v != "forty-two" {
		t.Errorf("expected forty-two as the answer, got %s", v)
	}

	// Create is idempotent and seeds the fixed descriptions exactly once.
	if err := pg.Create(); err != nil {
		t.Fatalf("expected no error re-creating the tables, got %s", err)
	}
	for _, tc := range []struct {
		table string
		code  int
		desc  string
		rows  int
	}{
		{"situacao_cadastral", 2, "ATIVA", 5},
		{"identificador_matriz_filial", 1, "MATRIZ", 2},
		{"porte", 3, "EMPRESA DE PEQUENO PORTE", 4},
		{"faixa_etaria", 9, "Maiores de 80 anos", 10},
	} {
		var n int
		if err := pg.pool.QueryRow(ctx, "SELECT count(*) FROM "+tc.table).Scan(&n); err != nil {
			t.Fatalf("expected no error counting %s, got %s", tc.table, err)
		}
		if n != tc.rows {
			t.Errorf("expected %d rows in %s, got %d", tc.rows, tc.table, n)
		}
		var d string
		if err := pg.pool.QueryRow(ctx, "SELECT descricao FROM "+tc.table+" WHERE codigo = $1", tc.code).Scan(&d); err != nil {
			t.Fatalf("expected no error reading %s, got %s", tc.table, err)
		}
		if d != tc.desc {
			t.Errorf("expected %s for code %d in %s, got %s", tc.desc, tc.code, tc.table, d)
		}
	}
}
