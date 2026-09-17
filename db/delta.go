package db

import (
	"context"
	"fmt"
	"strings"
)

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
	referenceApplyCols       = []string{"codigo", "descricao"}
	municipioIBGEApplyCols   = []string{"codigo_municipio", "codigo_ibge"}
)

// ApplyDelta makes the live tables equal to the staging tables, touching only
// the differences: hashed entities are upserted (new/changed) and pruned
// (removed); child collections are replaced for the parents whose set changed;
// reference entries are upserted when their value changed and pruned when gone.
func (p *PostgreSQL) ApplyDelta(ctx context.Context) error {
	for _, t := range []struct {
		name string
		cols []string
	}{
		{"cnae", referenceApplyCols},
		{"motivo", referenceApplyCols},
		{"municipio", referenceApplyCols},
		{"pais", referenceApplyCols},
		{"natureza_juridica", referenceApplyCols},
		{"qualificacao", referenceApplyCols},
		{"municipio_ibge", municipioIBGEApplyCols},
	} {
		if err := p.applyReference(ctx, t.name, t.cols[0], t.cols[1]); err != nil {
			return err
		}
	}
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

func (p *PostgreSQL) applyReference(ctx context.Context, name, key, val string) error {
	upsert := fmt.Sprintf(
		`INSERT INTO %s (%s, %s) SELECT %s, %s FROM %s_staging ON CONFLICT (%s) DO UPDATE SET %s = EXCLUDED.%s WHERE %s.%s IS DISTINCT FROM EXCLUDED.%s`,
		name, key, val, key, val, name, key, val, val, name, val, val,
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
