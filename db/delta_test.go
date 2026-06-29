package db

import (
	"context"
	"os"
	"testing"

	"codeberg.org/cuducos/minha-receita/testutils"
)

func setUpDelta(t *testing.T) *PostgreSQL {
	t.Helper()
	u := os.Getenv("TEST_POSTGRES_URL")
	if u == "" {
		t.Fatal("expected a postgres uri at TEST_POSTGRES_URL, found nothing")
	}
	pg, err := NewPostgreSQL(u, "public", false)
	if err != nil {
		t.Fatalf("expected no error connecting to postgres, got %s", err)
	}
	if err := pg.Drop(); err != nil {
		t.Fatalf("expected no error dropping tables, got %s", err)
	}
	if err := pg.Create(); err != nil {
		t.Fatalf("expected no error creating tables, got %s", err)
	}
	if err := pg.PrepareStaging(); err != nil {
		t.Fatalf("expected no error preparing staging, got %s", err)
	}
	return &pg
}

// TestComputeDelta covers the CDC scenarios: unchanged, changed estabelecimento
// hash, new CNPJ, removed CNPJ, a changed empresa base fanning out to its
// filiais, a changed socio set, and a changed secondary-CNAE set.
func TestComputeDelta(t *testing.T) {
	pg := setUpDelta(t)
	defer func() {
		if err := pg.Drop(); err != nil {
			t.Errorf("expected no error dropping tables, got %s", err)
		}
		pg.Close()
	}()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pg.pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec failed: %s\nquery: %s", err, q)
		}
	}

	// Live estabelecimentos (all hash \x01).
	for _, e := range [][2]string{
		{"11111111000111", "11111111"}, // A: unchanged
		{"22222222000122", "22222222"}, // B: estabelecimento hash will differ
		{"44444444000144", "44444444"}, // D: removed (absent from staging)
		{"55555555000155", "55555555"}, // E: filial of a base whose empresa changes
		{"55555555000255", "55555555"}, // F: filial of the same base
		{"66666666000166", "66666666"}, // G: socio set changes
		{"77777777000177", "77777777"}, // H: secondary-CNAE set changes
	} {
		exec(`INSERT INTO estabelecimento (cnpj, cnpj_base, hash) VALUES ($1, $2, '\x01')`, e[0], e[1])
	}
	for _, b := range []string{"11111111", "22222222", "44444444", "55555555", "66666666", "77777777"} {
		exec(`INSERT INTO empresa (cnpj_base, hash) VALUES ($1, '\x01')`, b)
	}
	exec(`INSERT INTO socio (cnpj_base, nome_socio) VALUES ('66666666', 'SOCIO ANTIGO')`)
	exec(`INSERT INTO estabelecimento_cnae (cnpj, cnae, ordem) VALUES ('77777777000177', 6201, 0)`)

	// Staging (the new snapshot).
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('11111111000111', '11111111', '\x01')`) // A unchanged
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('22222222000122', '22222222', '\x02')`) // B changed
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('33333333000133', '33333333', '\x01')`) // C new
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('55555555000155', '55555555', '\x01')`) // E (estab unchanged)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('55555555000255', '55555555', '\x01')`) // F (estab unchanged)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('66666666000166', '66666666', '\x01')`) // G (estab unchanged)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('77777777000177', '77777777', '\x01')`) // H (estab unchanged)

	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('11111111', '\x01')`)
	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('22222222', '\x01')`)
	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('33333333', '\x01')`)
	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('55555555', '\x02')`) // base changed -> E, F rebuild
	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('66666666', '\x01')`)
	exec(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ('77777777', '\x01')`)
	exec(`INSERT INTO socio_staging (cnpj_base, nome_socio) VALUES ('66666666', 'SOCIO NOVO')`)             // set changed -> G
	exec(`INSERT INTO estabelecimento_cnae_staging (cnpj, cnae, ordem) VALUES ('77777777000177', 6202, 0)`) // set changed -> H

	delta, err := pg.ComputeDelta(ctx)
	if err != nil {
		t.Fatalf("expected no error computing delta, got %s", err)
	}
	wantRebuild := []string{
		"22222222000122", // B: estabelecimento hash
		"33333333000133", // C: new
		"55555555000155", // E: empresa base changed
		"55555555000255", // F: empresa base changed
		"66666666000166", // G: socio set changed
		"77777777000177", // H: secondary-CNAE set changed
	}
	wantDelete := []string{"44444444000144"} // D: removed
	testutils.AssertArraysHaveSameItems(t, wantRebuild, delta.Rebuild)
	testutils.AssertArraysHaveSameItems(t, wantDelete, delta.Delete)
}
