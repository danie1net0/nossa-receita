package db

import (
	"context"
	"fmt"
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

func (p *PostgreSQL) queryCNPJs(ctx context.Context, q string) ([]string, error) {
	rows, err := p.pool.Query(ctx, q)
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
