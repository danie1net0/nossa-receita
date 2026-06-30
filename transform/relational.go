package transform

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/dgraph-io/badger/v4"
)

// relationalBatchSize is the number of rows accumulated before a COPY to the
// relational tables.
const relationalBatchSize = 16384

// Column lists for the relational tables. They must match db/postgres/create.sql.
var (
	empresaColumns = []string{
		"cnpj_base", "razao_social", "codigo_natureza_juridica",
		"qualificacao_do_responsavel", "capital_social", "codigo_porte",
		"ente_federativo_responsavel", "hash",
	}
	estabelecimentoColumns = []string{
		"cnpj", "cnpj_base", "identificador_matriz_filial", "nome_fantasia",
		"situacao_cadastral", "data_situacao_cadastral", "motivo_situacao_cadastral",
		"nome_cidade_no_exterior", "codigo_pais", "data_inicio_atividade",
		"cnae_fiscal", "descricao_tipo_de_logradouro", "logradouro", "numero",
		"complemento", "bairro", "cep", "uf", "codigo_municipio",
		"ddd_telefone_1", "ddd_telefone_2", "ddd_fax", "email",
		"situacao_especial", "data_situacao_especial", "hash",
	}
	estabelecimentoCnaeColumns = []string{"cnpj", "cnae", "ordem"}
	simplesColumns             = []string{
		"cnpj_base", "opcao_pelo_simples", "data_opcao_pelo_simples",
		"data_exclusao_do_simples", "opcao_pelo_mei", "data_opcao_pelo_mei",
		"data_exclusao_do_mei", "hash",
	}
	socioColumns = []string{
		"cnpj_base", "identificador_de_socio", "nome_socio", "cnpj_cpf_do_socio",
		"codigo_qualificacao_socio", "data_entrada_sociedade", "codigo_pais",
		"cpf_representante_legal", "nome_representante_legal",
		"codigo_qualificacao_representante_legal", "codigo_faixa_etaria",
	}
	regimeTributarioColumns = []string{
		"cnpj", "tipo", "ano", "cnpj_da_scp", "forma_de_tributacao",
		"quantidade_de_escrituracoes",
	}
	referenceColumns     = []string{"codigo", "descricao"}
	municipioIBGEColumns = []string{"codigo_municipio", "codigo_ibge"}
)

// regimeTipo maps a tax-regime source key to the label stored in the table.
var regimeTipo = map[string]string{
	"imu": "imune-isenta",
	"arb": "arbitrado",
	"pre": "presumido",
	"rea": "real",
}

// hashRow returns the 8-byte big-endian xxhash64 of the given columns. It feeds
// the cheap change-detection diff used by the incremental-update phase.
func hashRow(cols []string) []byte {
	h := xxhash.New()
	for _, c := range cols {
		_, _ = h.WriteString(c)
		_, _ = h.Write([]byte{0}) // separator to avoid ambiguous concatenations
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], h.Sum64())
	return b[:]
}

// The *Value helpers parse a raw CSV string into a value pgx can encode, or nil
// for a SQL NULL. The *Ptr helpers do the same for the typed pointers already
// parsed into a Company.

func intValue(s string) any {
	i, err := toInt(s)
	if err != nil || i == nil {
		return nil
	}
	return *i
}

func floatValue(s string) any {
	f, err := toFloat(s)
	if err != nil || f == nil {
		return nil
	}
	return float64(*f)
}

func boolValue(s string) any {
	b := toBool(s)
	if b == nil {
		return nil
	}
	return *b
}

func dateValue(s string) any {
	d, err := toDate(s)
	if err != nil || d == nil {
		return nil
	}
	return time.Time(*d)
}

func intPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func datePtr(d *date) any {
	if d == nil {
		return nil
	}
	return time.Time(*d)
}

func strPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// estabelecimentoRow builds the estabelecimento row from a Company. It must be
// called BEFORE withPrivacy so the relational store keeps raw data; the raw CSV
// row is hashed for change detection.
func estabelecimentoRow(c *Company, raw []string) []any {
	return []any{
		c.CNPJ,
		c.CNPJ[:8],
		intPtr(c.IdentificadorMatrizFilial),
		c.NomeFantasia,
		intPtr(c.SituacaoCadastral),
		datePtr(c.DataSituacaoCadastral),
		intPtr(c.MotivoSituacaoCadastral),
		c.NomeCidadeNoExterior,
		intPtr(c.CodigoPais),
		datePtr(c.DataInicioAtividade),
		intPtr(c.CNAEFiscal),
		c.DescricaoTipoDeLogradouro,
		c.Logradouro,
		c.Numero,
		c.Complemento,
		c.Bairro,
		c.CEP,
		c.UF,
		intPtr(c.CodigoMunicipio),
		c.Telefone1,
		c.Telefone2,
		c.Fax,
		strPtr(c.Email),
		c.SituacaoEspecial,
		datePtr(c.DataSituacaoEspecial),
		hashRow(raw),
	}
}

// estabelecimentoCnaeRows builds the secondary-CNAE rows for a Company,
// de-duplicated to avoid (cnpj, cnae) primary-key collisions.
func estabelecimentoCnaeRows(c *Company) [][]any {
	if len(c.CNAESecundarios) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(c.CNAESecundarios))
	rows := make([][]any, 0, len(c.CNAESecundarios))
	for _, cnae := range c.CNAESecundarios {
		if _, ok := seen[cnae.Codigo]; ok {
			continue
		}
		seen[cnae.Codigo] = struct{}{}
		rows = append(rows, []any{c.CNPJ, cnae.Codigo, len(rows)}) // ordem preserves source order
	}
	return rows
}

func empresaRow(base string, val []string) []any {
	return []any{
		base, val[0], intValue(val[1]), intValue(val[2]), floatValue(val[3]),
		intValue(val[4]), val[5], hashRow(val),
	}
}

func simplesRow(base string, val []string) []any {
	return []any{
		base, boolValue(val[0]), dateValue(val[1]), dateValue(val[2]),
		boolValue(val[3]), dateValue(val[4]), dateValue(val[5]), hashRow(val),
	}
}

func socioRow(base string, val []string) []any {
	return []any{
		base, intValue(val[0]), val[1], val[2], intValue(val[3]), dateValue(val[4]),
		intValue(val[5]), val[6], val[7], intValue(val[8]), intValue(val[9]),
	}
}

func regimeRow(tipo, cnpj string, val []string) []any {
	var scp any
	if val[1] != "" && val[1] != "0" {
		scp = val[1]
	}
	return []any{cnpj, tipo, intValue(val[0]), scp, val[2], intValue(val[3])}
}

// relBatch accumulates rows for a single table and flushes them via COPY once
// the batch is full.
type relBatch struct {
	table   string
	columns []string
	rows    [][]any
}

func (b *relBatch) add(ctx context.Context, db database, row []any) error {
	b.rows = append(b.rows, row)
	if len(b.rows) >= relationalBatchSize {
		return b.flush(ctx, db)
	}
	return nil
}

func (b *relBatch) flush(ctx context.Context, db database) error {
	if len(b.rows) == 0 {
		return nil
	}
	if err := db.CopyRelational(ctx, b.table, b.columns, b.rows); err != nil {
		return err
	}
	b.rows = b.rows[:0]
	return nil
}

// loadRelational scans the key-value store populated in stage 2 and writes the
// base-keyed, reference and tax-regime relational tables (dual-write). The
// estabelecimento rows come separately, from the stage-3 stream. It is safe to
// run concurrently with writeJSONs: it writes a disjoint set of tables.
func loadRelational(ctx context.Context, kv *kv, db database, suffix string) error {
	// The suffix targets the entity tables (e.g. "_staging" for incremental
	// updates); reference tables are always written to the live tables.
	empresa := &relBatch{table: "empresa" + suffix, columns: empresaColumns}
	simples := &relBatch{table: "simples" + suffix, columns: simplesColumns}
	socio := &relBatch{table: "socio" + suffix, columns: socioColumns}
	regime := &relBatch{table: "regime_tributario" + suffix, columns: regimeTributarioColumns}
	ibge := &relBatch{table: "municipio_ibge", columns: municipioIBGEColumns}
	refs := map[string]*relBatch{
		"cna": {table: "cnae", columns: referenceColumns},
		"mot": {table: "motivo", columns: referenceColumns},
		"mun": {table: "municipio", columns: referenceColumns},
		"pai": {table: "pais", columns: referenceColumns},
		"nat": {table: "natureza_juridica", columns: referenceColumns},
		"qua": {table: "qualificacao", columns: referenceColumns},
	}

	err := kv.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			item := it.Item()
			parts := strings.SplitN(string(item.Key()), "::", 3)
			if len(parts) < 2 {
				continue
			}
			id, srcKey := parts[0], parts[1]
			var val []string
			if err := item.Value(func(b []byte) error {
				var e error
				val, e = kv.deserialize(b)
				return e
			}); err != nil {
				return err
			}
			if err := routeRelational(ctx, db, id, srcKey, val, empresa, simples, socio, regime, ibge, refs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not scan key-value storage for relational load: %w", err)
	}

	batches := []*relBatch{empresa, simples, socio, regime, ibge}
	for _, b := range refs {
		batches = append(batches, b)
	}
	for _, b := range batches {
		if err := b.flush(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

// routeRelational dispatches one key-value entry to the matching table batch,
// skipping malformed rows (those with fewer columns than the source layout).
func routeRelational(ctx context.Context, db database, id, srcKey string, val []string, empresa, simples, socio, regime, ibge *relBatch, refs map[string]*relBatch) error {
	switch srcKey {
	case "emp":
		if len(val) >= 6 {
			return empresa.add(ctx, db, empresaRow(id, val))
		}
	case "sim":
		if len(val) >= 6 {
			return simples.add(ctx, db, simplesRow(id, val))
		}
	case "soc":
		if len(val) >= 10 {
			return socio.add(ctx, db, socioRow(id, val))
		}
	case "cna", "mot", "mun", "pai", "nat", "qua":
		if len(val) >= 1 {
			if cod := intValue(id); cod != nil { // codigo is the primary key, must be non-null
				return refs[srcKey].add(ctx, db, []any{cod, val[0]})
			}
		}
	case "tab":
		if len(val) >= 4 {
			if cod := intValue(id); cod != nil {
				return ibge.add(ctx, db, []any{cod, intValue(val[3])})
			}
		}
	case "imu", "arb", "pre", "rea":
		if len(val) >= 4 {
			return regime.add(ctx, db, regimeRow(regimeTipo[srcKey], id, val))
		}
	}
	return nil
}
