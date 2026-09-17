package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
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

// seedScenario fills the live and staging tables with the CDC cases: A
// unchanged, B changed estabelecimento hash, C new, D removed, E+F filiais of a
// base whose empresa changed, G changed socio set, H changed secondary-CNAE set,
// and reference-only changes: a re-described município (I) and qualificação (J),
// a new IBGE code (K) and an added natureza jurídica.
func seedScenario(t *testing.T, pg *PostgreSQL) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pg.pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec failed: %s\nquery: %s", err, q)
		}
	}
	// Live (all estabelecimento/empresa hash \x01).
	for _, e := range [][2]string{
		{"11111111000111", "11111111"}, // A: unchanged
		{"22222222000122", "22222222"}, // B: estabelecimento hash will differ
		{"44444444000144", "44444444"}, // D: removed
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
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('11111111000111', '11111111', '\x01')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('22222222000122', '22222222', '\x02')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('33333333000133', '33333333', '\x01')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('55555555000155', '55555555', '\x01')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('55555555000255', '55555555', '\x01')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('66666666000166', '66666666', '\x01')`)
	exec(`INSERT INTO estabelecimento_staging (cnpj, cnpj_base, hash) VALUES ('77777777000177', '77777777', '\x01')`)
	for _, b := range []struct{ base, hash string }{
		{"11111111", `\x01`}, {"22222222", `\x01`}, {"33333333", `\x01`},
		{"55555555", `\x02`}, {"66666666", `\x01`}, {"77777777", `\x01`},
	} {
		exec(fmt.Sprintf(`INSERT INTO empresa_staging (cnpj_base, hash) VALUES ($1, '%s')`, b.hash), b.base)
	}
	exec(`INSERT INTO socio_staging (cnpj_base, nome_socio) VALUES ('66666666', 'SOCIO NOVO')`)
	exec(`INSERT INTO estabelecimento_cnae_staging (cnpj, cnae, ordem) VALUES ('77777777000177', 6202, 0)`)

	// Reference-only changes: I, J and K are identical in live and staging.
	for _, suffix := range []string{"", "_staging"} {
		exec(fmt.Sprintf(`INSERT INTO estabelecimento%s (cnpj, cnpj_base, codigo_municipio, hash) VALUES ('88888888000188', '88888888', 2, '\x01')`, suffix)) // I
		exec(fmt.Sprintf(`INSERT INTO estabelecimento%s (cnpj, cnpj_base, hash) VALUES ('99999999000199', '99999999', '\x01')`, suffix))                      // J
		exec(fmt.Sprintf(`INSERT INTO estabelecimento%s (cnpj, cnpj_base, codigo_municipio, hash) VALUES ('12121212000112', '12121212', 1, '\x01')`, suffix)) // K
		for _, b := range []string{"88888888", "99999999", "12121212"} {
			exec(fmt.Sprintf(`INSERT INTO empresa%s (cnpj_base, codigo_natureza_juridica, hash) VALUES ($1, 2062, '\x01')`, suffix), b)
		}
		exec(fmt.Sprintf(`INSERT INTO socio%s (cnpj_base, nome_socio, codigo_qualificacao_socio) VALUES ('99999999', 'SOCIO', 49)`, suffix))
		exec(fmt.Sprintf(`INSERT INTO cnae%s (codigo, descricao) VALUES (6201, 'DESENVOLVIMENTO'), (6202, 'CUSTOMIZAVEL')`, suffix))
		exec(fmt.Sprintf(`INSERT INTO natureza_juridica%s (codigo, descricao) VALUES (2062, 'SOCIEDADE LIMITADA')`, suffix))
		exec(fmt.Sprintf(`INSERT INTO municipio%s (codigo, descricao) VALUES (1, 'BRASILIA')`, suffix))
	}
	exec(`INSERT INTO municipio (codigo, descricao) VALUES (2, 'SANTA HELENA')`)
	exec(`INSERT INTO municipio_staging (codigo, descricao) VALUES (2, 'SANTA HELENA DE GOIAS')`) // I
	exec(`INSERT INTO qualificacao (codigo, descricao) VALUES (49, 'SOCIO')`)
	exec(`INSERT INTO qualificacao_staging (codigo, descricao) VALUES (49, 'SOCIO-ADMINISTRADOR')`) // J
	exec(`INSERT INTO municipio_ibge (codigo_municipio, codigo_ibge) VALUES (1, 5300107)`)
	exec(`INSERT INTO municipio_ibge_staging (codigo_municipio, codigo_ibge) VALUES (1, 5300108)`) // K
	exec(`INSERT INTO natureza_juridica_staging (codigo, descricao) VALUES (9999, 'NOVA SEM USO')`)
}

func TestApplyDelta(t *testing.T) {
	pg := setUpDelta(t)
	defer func() {
		if err := pg.Drop(); err != nil {
			t.Errorf("expected no error dropping tables, got %s", err)
		}
		pg.Close()
	}()
	ctx := context.Background()
	seedScenario(t, pg)

	if err := pg.ApplyDelta(ctx); err != nil {
		t.Fatalf("expected no error applying delta, got %s", err)
	}

	// After apply each live table must equal its staging counterpart.
	for _, tc := range []struct {
		table string
		cols  []string
	}{
		{"empresa", empresaApplyCols},
		{"estabelecimento", estabelecimentoApplyCols},
		{"simples", simplesApplyCols},
		{"socio", socioApplyCols},
		{"estabelecimento_cnae", cnaeApplyCols},
		{"regime_tributario", regimeApplyCols},
		{"cnae", referenceApplyCols},
		{"motivo", referenceApplyCols},
		{"municipio", referenceApplyCols},
		{"pais", referenceApplyCols},
		{"natureza_juridica", referenceApplyCols},
		{"qualificacao", referenceApplyCols},
		{"municipio_ibge", municipioIBGEApplyCols},
	} {
		c := strings.Join(tc.cols, ", ")
		q := fmt.Sprintf(
			`SELECT count(*) FROM ((SELECT %s FROM %s EXCEPT SELECT %s FROM %s_staging) UNION ALL (SELECT %s FROM %s_staging EXCEPT SELECT %s FROM %s)) d`,
			c, tc.table, c, tc.table, c, tc.table, c, tc.table,
		)
		var n int
		if err := pg.pool.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatalf("could not diff %s: %s", tc.table, err)
		}
		if n != 0 {
			t.Errorf("%s differs from staging by %d rows after apply", tc.table, n)
		}
	}
}
