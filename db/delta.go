package db

import (
	"context"
	"fmt"
	"strings"
)

// Delta is the result of diffing the staging tables (loaded with the new
// snapshot) against the live tables: the full CNPJs whose document must be
// rebuilt, and those whose document must be removed.
type Delta struct {
	Rebuild []string
	Delete  []string
}

// deleteQuery finds estabelecimentos that exist live but not in staging: their
// CNPJ was removed from the snapshot, so the document must be deleted.
const deleteQuery = `
SELECT e.cnpj
FROM estabelecimento e
LEFT JOIN estabelecimento_staging s ON s.cnpj = e.cnpj
WHERE s.cnpj IS NULL`

// rebuildQuery finds the full CNPJs whose document must be rebuilt: an
// estabelecimento that is new or whose hash changed; or any estabelecimento of a
// base whose empresa/simples row changed or whose socio set changed; or a CNPJ
// whose secondary-CNAE or tax-regime set changed. Removed CNPJs (absent from
// staging) are excluded by the final filter.
const rebuildQuery = `
WITH estab_changed AS (
    SELECT s.cnpj
    FROM estabelecimento_staging s
    LEFT JOIN estabelecimento e ON e.cnpj = s.cnpj
    WHERE e.cnpj IS NULL OR e.hash <> s.hash
),
base_changed AS (
    SELECT s.cnpj_base
    FROM empresa_staging s
    LEFT JOIN empresa e ON e.cnpj_base = s.cnpj_base
    WHERE e.cnpj_base IS NULL OR e.hash <> s.hash
    UNION
    SELECT s.cnpj_base
    FROM simples_staging s
    LEFT JOIN simples e ON e.cnpj_base = s.cnpj_base
    WHERE e.cnpj_base IS NULL OR e.hash <> s.hash
    UNION
    SELECT cnpj_base FROM (
        SELECT cnpj_base, identificador_de_socio, nome_socio, cnpj_cpf_do_socio, codigo_qualificacao_socio, data_entrada_sociedade, codigo_pais, cpf_representante_legal, nome_representante_legal, codigo_qualificacao_representante_legal, codigo_faixa_etaria FROM socio_staging
        EXCEPT
        SELECT cnpj_base, identificador_de_socio, nome_socio, cnpj_cpf_do_socio, codigo_qualificacao_socio, data_entrada_sociedade, codigo_pais, cpf_representante_legal, nome_representante_legal, codigo_qualificacao_representante_legal, codigo_faixa_etaria FROM socio
    ) socio_added
    UNION
    SELECT cnpj_base FROM (
        SELECT cnpj_base, identificador_de_socio, nome_socio, cnpj_cpf_do_socio, codigo_qualificacao_socio, data_entrada_sociedade, codigo_pais, cpf_representante_legal, nome_representante_legal, codigo_qualificacao_representante_legal, codigo_faixa_etaria FROM socio
        EXCEPT
        SELECT cnpj_base, identificador_de_socio, nome_socio, cnpj_cpf_do_socio, codigo_qualificacao_socio, data_entrada_sociedade, codigo_pais, cpf_representante_legal, nome_representante_legal, codigo_qualificacao_representante_legal, codigo_faixa_etaria FROM socio_staging
    ) socio_removed
),
cnae_changed AS (
    SELECT cnpj FROM (
        SELECT cnpj, cnae, ordem FROM estabelecimento_cnae_staging
        EXCEPT
        SELECT cnpj, cnae, ordem FROM estabelecimento_cnae
    ) cnae_added
    UNION
    SELECT cnpj FROM (
        SELECT cnpj, cnae, ordem FROM estabelecimento_cnae
        EXCEPT
        SELECT cnpj, cnae, ordem FROM estabelecimento_cnae_staging
    ) cnae_removed
),
regime_changed AS (
    SELECT cnpj FROM (
        SELECT cnpj, tipo, ano, cnpj_da_scp, forma_de_tributacao, quantidade_de_escrituracoes FROM regime_tributario_staging
        EXCEPT
        SELECT cnpj, tipo, ano, cnpj_da_scp, forma_de_tributacao, quantidade_de_escrituracoes FROM regime_tributario
    ) regime_added
    UNION
    SELECT cnpj FROM (
        SELECT cnpj, tipo, ano, cnpj_da_scp, forma_de_tributacao, quantidade_de_escrituracoes FROM regime_tributario
        EXCEPT
        SELECT cnpj, tipo, ano, cnpj_da_scp, forma_de_tributacao, quantidade_de_escrituracoes FROM regime_tributario_staging
    ) regime_removed
),
affected AS (
    SELECT cnpj FROM estab_changed
    UNION
    SELECT s.cnpj FROM estabelecimento_staging s JOIN base_changed b ON b.cnpj_base = s.cnpj_base
    UNION
    SELECT cnpj FROM cnae_changed
    UNION
    SELECT cnpj FROM regime_changed
)
SELECT cnpj FROM affected
WHERE cnpj IN (SELECT cnpj FROM estabelecimento_staging)`

// PrepareStaging creates (if needed) and truncates the UNLOGGED staging tables.
func (p *PostgreSQL) PrepareStaging() error {
	s, err := p.renderTemplate("staging")
	if err != nil {
		return fmt.Errorf("error rendering staging template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error preparing staging tables: %w", err)
	}
	return nil
}

// ComputeDelta diffs the staging tables against the live tables and returns the
// CNPJs to rebuild and to delete. It only reads; applying the changes is a
// separate step.
func (p *PostgreSQL) ComputeDelta(ctx context.Context) (*Delta, error) {
	rebuild, err := p.queryCNPJs(ctx, rebuildQuery)
	if err != nil {
		return nil, fmt.Errorf("could not compute the rebuild set: %w", err)
	}
	del, err := p.queryCNPJs(ctx, deleteQuery)
	if err != nil {
		return nil, fmt.Errorf("could not compute the delete set: %w", err)
	}
	return &Delta{Rebuild: rebuild, Delete: del}, nil
}

func (p *PostgreSQL) queryCNPJs(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Column lists for applying the delta (must match db/postgres/create.sql and the
// staging tables). Child collections omit the surrogate id.
var (
	empresaApplyCols         = []string{"cnpj_base", "razao_social", "codigo_natureza_juridica", "qualificacao_do_responsavel", "capital_social", "codigo_porte", "ente_federativo_responsavel", "hash"}
	estabelecimentoApplyCols = []string{"cnpj", "cnpj_base", "identificador_matriz_filial", "nome_fantasia", "situacao_cadastral", "data_situacao_cadastral", "motivo_situacao_cadastral", "nome_cidade_no_exterior", "codigo_pais", "data_inicio_atividade", "cnae_fiscal", "descricao_tipo_de_logradouro", "logradouro", "numero", "complemento", "bairro", "cep", "uf", "codigo_municipio", "ddd_telefone_1", "ddd_telefone_2", "ddd_fax", "email", "situacao_especial", "data_situacao_especial", "hash"}
	simplesApplyCols         = []string{"cnpj_base", "opcao_pelo_simples", "data_opcao_pelo_simples", "data_exclusao_do_simples", "opcao_pelo_mei", "data_opcao_pelo_mei", "data_exclusao_do_mei", "hash"}
	socioApplyCols           = []string{"cnpj_base", "identificador_de_socio", "nome_socio", "cnpj_cpf_do_socio", "codigo_qualificacao_socio", "data_entrada_sociedade", "codigo_pais", "cpf_representante_legal", "nome_representante_legal", "codigo_qualificacao_representante_legal", "codigo_faixa_etaria"}
	cnaeApplyCols            = []string{"cnpj", "cnae", "ordem"}
	regimeApplyCols          = []string{"cnpj", "tipo", "ano", "cnpj_da_scp", "forma_de_tributacao", "quantidade_de_escrituracoes"}
)

// ApplyDelta makes the live tables equal to the staging tables, touching only
// the differences: hashed entities are upserted (new/changed) and pruned
// (removed); child collections are replaced for the parents whose set changed.
func (p *PostgreSQL) ApplyDelta(ctx context.Context) error {
	for _, t := range []struct {
		name, key string
		cols      []string
	}{
		{"empresa", "cnpj_base", empresaApplyCols},
		{"estabelecimento", "cnpj", estabelecimentoApplyCols},
		{"simples", "cnpj_base", simplesApplyCols},
	} {
		if err := p.applyHashed(ctx, t.name, t.key, t.cols); err != nil {
			return err
		}
	}
	for _, t := range []struct {
		name, parentKey string
		cols            []string
	}{
		{"socio", "cnpj_base", socioApplyCols},
		{"estabelecimento_cnae", "cnpj", cnaeApplyCols},
		{"regime_tributario", "cnpj", regimeApplyCols},
	} {
		if err := p.applyCollection(ctx, t.name, t.parentKey, t.cols); err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgreSQL) applyHashed(ctx context.Context, name, key string, cols []string) error {
	sel := make([]string, 0, len(cols))
	set := make([]string, 0, len(cols))
	for _, c := range cols {
		sel = append(sel, "s."+c)
		if c != key {
			set = append(set, c+" = EXCLUDED."+c)
		}
	}
	upsert := fmt.Sprintf(
		`INSERT INTO %s (%s) SELECT %s FROM %s_staging s LEFT JOIN %s e ON e.%s = s.%s WHERE e.%s IS NULL OR e.hash <> s.hash ON CONFLICT (%s) DO UPDATE SET %s`,
		name, strings.Join(cols, ", "), strings.Join(sel, ", "), name, name, key, key, key, key, strings.Join(set, ", "),
	)
	if _, err := p.pool.Exec(ctx, upsert); err != nil {
		return fmt.Errorf("could not upsert %s: %w", name, err)
	}
	del := fmt.Sprintf(`DELETE FROM %s e WHERE NOT EXISTS (SELECT 1 FROM %s_staging s WHERE s.%s = e.%s)`, name, name, key, key)
	if _, err := p.pool.Exec(ctx, del); err != nil {
		return fmt.Errorf("could not prune %s: %w", name, err)
	}
	return nil
}

func (p *PostgreSQL) applyCollection(ctx context.Context, name, parentKey string, cols []string) error {
	cl := strings.Join(cols, ", ")
	changedQ := fmt.Sprintf(
		`SELECT DISTINCT d.%s FROM ((SELECT %s FROM %s_staging EXCEPT SELECT %s FROM %s) UNION ALL (SELECT %s FROM %s EXCEPT SELECT %s FROM %s_staging)) d`,
		parentKey, cl, name, cl, name, cl, name, cl, name,
	)
	parents, err := p.queryCNPJs(ctx, changedQ)
	if err != nil {
		return fmt.Errorf("could not find changed parents for %s: %w", name, err)
	}
	if len(parents) == 0 {
		return nil
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s = ANY($1)`, name, parentKey), parents); err != nil {
		return fmt.Errorf("could not delete changed %s: %w", name, err)
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s_staging WHERE %s = ANY($1)`, name, cl, cl, name, parentKey), parents); err != nil {
		return fmt.Errorf("could not insert changed %s: %w", name, err)
	}
	return nil
}
