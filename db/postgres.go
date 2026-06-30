package db

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	cursorFieldName = "cursor"
	idFieldName     = "id"
	jsonFieldName   = "json"
	keyFieldName    = "key"
	valueFieldName  = "value"
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

type ExtraIndex struct {
	IsRoot bool
	Name   string
	Value  string
}

func (e *ExtraIndex) NestedPath() string {
	if e.IsRoot {
		slog.Error("cannot not parse nested path for index at the root of the json", "index", e.Value)
		return ""
	}
	p := strings.SplitN(e.Value, ".", 2)
	if len(p) != 2 {
		slog.Error("could not parse nested path", "index", e.Value)
		return ""
	}
	return fmt.Sprintf("$.%s[*].%s", p[0], p[1])
}

// PostgreSQL database interface.
type PostgreSQL struct {
	pool              *pgxpool.Pool
	uri               string
	schema            string
	getCompanyQuery   string
	getNeighborsQuery string
	metaReadQuery     string
	CompanyTableName  string
	GraphTableName    string
	MetaTableName     string
	CursorFieldName   string
	IDFieldName       string
	JSONFieldName     string
	KeyFieldName      string
	ValueFieldName    string
	ExtraIndexes      []ExtraIndex
	Logged            bool
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

// CompanyTableFullName is the name of the schame and table in dot-notation.
func (p *PostgreSQL) CompanyTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.CompanyTableName)
}

// GraphTableFullName is the name of the schema and table in dot-notation.
func (p *PostgreSQL) GraphTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.GraphTableName)
}

// MetaTableFullName is the name of the schame and table in dot-notation.
func (p *PostgreSQL) MetaTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.MetaTableName)
}

// Create creates the required database table.
func (p *PostgreSQL) Create() error {
	slog.Info("Creating", "table", p.CompanyTableFullName())
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
	slog.Info("Dropping", "table", p.CompanyTableFullName())
	s, err := p.renderTemplate("drop")
	if err != nil {
		return fmt.Errorf("error rendering drop template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error dropping table with: %s\n%w", s, err)
	}
	return nil
}

// CreateCompanies performs a copy to create a batch of companies in the
// database. It expects an array and each item should be another array with only
// two items: the ID and the JSON field values.
func (p *PostgreSQL) CreateCompanies(ctx context.Context, batch [][]string) error {
	b := make([][]any, len(batch))
	for i, r := range batch {
		b[i] = []any{r[0], r[1]}
	}
	_, err := p.pool.CopyFrom(
		ctx,
		pgx.Identifier{p.CompanyTableName},
		[]string{idFieldName, jsonFieldName},
		pgx.CopyFromRows(b),
	)
	if err != nil {
		return fmt.Errorf("error while importing data to postgres: %w", err)
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

var _ transform.MaterializeDB = (*PostgreSQL)(nil)

// ReferenceMaps loads the code->description lookups (and the municipal IBGE-code
// map) from the reference tables, used to resolve documents during
// materialization.
func (p *PostgreSQL) ReferenceMaps(ctx context.Context) (*transform.RefMaps, error) {
	ref := &transform.RefMaps{
		CNAE:         map[int]string{},
		Motivo:       map[int]string{},
		Municipio:    map[int]string{},
		Pais:         map[int]string{},
		Natureza:     map[int]string{},
		Qualificacao: map[int]string{},
		IBGE:         map[int]int{},
	}
	for tbl, m := range map[string]map[int]string{
		"cnae":              ref.CNAE,
		"motivo":            ref.Motivo,
		"municipio":         ref.Municipio,
		"pais":              ref.Pais,
		"natureza_juridica": ref.Natureza,
		"qualificacao":      ref.Qualificacao,
	} {
		rows, err := p.pool.Query(ctx, "SELECT codigo, descricao FROM "+tbl)
		if err != nil {
			return nil, fmt.Errorf("could not query %s: %w", tbl, err)
		}
		for rows.Next() {
			var code int
			var desc *string
			if err := rows.Scan(&code, &desc); err != nil {
				rows.Close()
				return nil, fmt.Errorf("could not scan %s: %w", tbl, err)
			}
			if desc != nil {
				m[code] = *desc
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error reading %s: %w", tbl, err)
		}
	}
	rows, err := p.pool.Query(ctx, "SELECT codigo_municipio, codigo_ibge FROM municipio_ibge")
	if err != nil {
		return nil, fmt.Errorf("could not query municipio_ibge: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cm int
		var ibge *int
		if err := rows.Scan(&cm, &ibge); err != nil {
			return nil, fmt.Errorf("could not scan municipio_ibge: %w", err)
		}
		if ibge != nil {
			ref.IBGE[cm] = *ibge
		}
	}
	return ref, rows.Err()
}

// RelationalCompanies reads the stored relational rows for the given CNPJs and
// returns them assembled per CNPJ (without resolved/derived fields).
func (p *PostgreSQL) RelationalCompanies(ctx context.Context, ids []string) ([]*transform.RelationalCompany, error) {
	byCNPJ := make(map[string]*transform.RelationalCompany, len(ids))
	byBase := make(map[string][]*transform.RelationalCompany)

	rows, err := p.pool.Query(ctx, `SELECT cnpj, cnpj_base, identificador_matriz_filial, nome_fantasia, situacao_cadastral, data_situacao_cadastral, motivo_situacao_cadastral, nome_cidade_no_exterior, codigo_pais, data_inicio_atividade, cnae_fiscal, descricao_tipo_de_logradouro, logradouro, numero, complemento, bairro, cep, uf, codigo_municipio, ddd_telefone_1, ddd_telefone_2, ddd_fax, email, situacao_especial, data_situacao_especial FROM estabelecimento WHERE cnpj = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("could not query estabelecimento: %w", err)
	}
	for rows.Next() {
		rc := &transform.RelationalCompany{}
		var base string
		if err := rows.Scan(&rc.CNPJ, &base, &rc.IdentificadorMatrizFilial, &rc.NomeFantasia, &rc.SituacaoCadastral, &rc.DataSituacaoCadastral, &rc.MotivoSituacaoCadastral, &rc.NomeCidadeNoExterior, &rc.CodigoPais, &rc.DataInicioAtividade, &rc.CNAEFiscal, &rc.DescricaoTipoDeLogradouro, &rc.Logradouro, &rc.Numero, &rc.Complemento, &rc.Bairro, &rc.CEP, &rc.UF, &rc.CodigoMunicipio, &rc.Telefone1, &rc.Telefone2, &rc.Fax, &rc.Email, &rc.SituacaoEspecial, &rc.DataSituacaoEspecial); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan estabelecimento: %w", err)
		}
		byCNPJ[rc.CNPJ] = rc
		byBase[base] = append(byBase[base], rc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading estabelecimento: %w", err)
	}

	bases := make([]string, 0, len(byBase))
	for b := range byBase {
		bases = append(bases, b)
	}

	rows, err = p.pool.Query(ctx, `SELECT cnpj_base, razao_social, codigo_natureza_juridica, qualificacao_do_responsavel, capital_social, codigo_porte, ente_federativo_responsavel FROM empresa WHERE cnpj_base = ANY($1)`, bases)
	if err != nil {
		return nil, fmt.Errorf("could not query empresa: %w", err)
	}
	for rows.Next() {
		var base, razao, ente string
		var natureza, qual, porte *int
		var capital *float64
		if err := rows.Scan(&base, &razao, &natureza, &qual, &capital, &porte, &ente); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan empresa: %w", err)
		}
		for _, rc := range byBase[base] {
			rc.RazaoSocial = razao
			rc.CodigoNaturezaJuridica = natureza
			rc.QualificacaoDoResponsavel = qual
			rc.CapitalSocial = capital
			rc.CodigoPorte = porte
			rc.EnteFederativoResponsavel = ente
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading empresa: %w", err)
	}

	rows, err = p.pool.Query(ctx, `SELECT cnpj_base, opcao_pelo_simples, data_opcao_pelo_simples, data_exclusao_do_simples, opcao_pelo_mei, data_opcao_pelo_mei, data_exclusao_do_mei FROM simples WHERE cnpj_base = ANY($1)`, bases)
	if err != nil {
		return nil, fmt.Errorf("could not query simples: %w", err)
	}
	for rows.Next() {
		var base string
		var optSimples, optMEI *bool
		var dOptSimples, dExcSimples, dOptMEI, dExcMEI *time.Time
		if err := rows.Scan(&base, &optSimples, &dOptSimples, &dExcSimples, &optMEI, &dOptMEI, &dExcMEI); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan simples: %w", err)
		}
		for _, rc := range byBase[base] {
			rc.OpcaoPeloSimples = optSimples
			rc.DataOpcaoPeloSimples = dOptSimples
			rc.DataExclusaoDoSimples = dExcSimples
			rc.OpcaoPeloMEI = optMEI
			rc.DataOpcaoPeloMEI = dOptMEI
			rc.DataExclusaoDoMEI = dExcMEI
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading simples: %w", err)
	}

	rows, err = p.pool.Query(ctx, `SELECT cnpj_base, identificador_de_socio, nome_socio, cnpj_cpf_do_socio, codigo_qualificacao_socio, data_entrada_sociedade, codigo_pais, cpf_representante_legal, nome_representante_legal, codigo_qualificacao_representante_legal, codigo_faixa_etaria FROM socio WHERE cnpj_base = ANY($1)`, bases)
	if err != nil {
		return nil, fmt.Errorf("could not query socio: %w", err)
	}
	sociosByBase := make(map[string][]transform.RelationalPartner)
	for rows.Next() {
		var base string
		var pr transform.RelationalPartner
		if err := rows.Scan(&base, &pr.IdentificadorDeSocio, &pr.NomeSocio, &pr.CNPJCPFDoSocio, &pr.CodigoQualificacaoSocio, &pr.DataEntradaSociedade, &pr.CodigoPais, &pr.CPFRepresentanteLegal, &pr.NomeRepresentanteLegal, &pr.CodigoQualificacaoRepresentanteLegal, &pr.CodigoFaixaEtaria); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan socio: %w", err)
		}
		sociosByBase[base] = append(sociosByBase[base], pr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading socio: %w", err)
	}
	for base, socios := range sociosByBase {
		for _, rc := range byBase[base] {
			rc.Socios = socios
		}
	}

	rows, err = p.pool.Query(ctx, `SELECT cnpj, cnae FROM estabelecimento_cnae WHERE cnpj = ANY($1) ORDER BY cnpj, ordem`, ids)
	if err != nil {
		return nil, fmt.Errorf("could not query estabelecimento_cnae: %w", err)
	}
	for rows.Next() {
		var cnpj string
		var cnae int
		if err := rows.Scan(&cnpj, &cnae); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan estabelecimento_cnae: %w", err)
		}
		if rc, ok := byCNPJ[cnpj]; ok {
			rc.CNAESecundarios = append(rc.CNAESecundarios, cnae)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading estabelecimento_cnae: %w", err)
	}

	rows, err = p.pool.Query(ctx, `SELECT cnpj, ano, cnpj_da_scp, forma_de_tributacao, quantidade_de_escrituracoes FROM regime_tributario WHERE cnpj = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("could not query regime_tributario: %w", err)
	}
	for rows.Next() {
		var cnpj string
		var reg transform.RelationalRegime
		if err := rows.Scan(&cnpj, &reg.Ano, &reg.CNPJDaSCP, &reg.FormaDeTributacao, &reg.QuantidadeDeEscrituracoes); err != nil {
			rows.Close()
			return nil, fmt.Errorf("could not scan regime_tributario: %w", err)
		}
		if rc, ok := byCNPJ[cnpj]; ok {
			rc.Regimes = append(rc.Regimes, reg)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading regime_tributario: %w", err)
	}

	out := make([]*transform.RelationalCompany, 0, len(byCNPJ))
	for _, id := range ids {
		if rc, ok := byCNPJ[id]; ok {
			out = append(out, rc)
		}
	}
	return out, nil
}

// SaveDocuments upserts the materialized JSON documents, updating in place so
// the cursor (and thus keyset pagination) is preserved.
func (p *PostgreSQL) SaveDocuments(ctx context.Context, docs map[string]string) error {
	if len(docs) == 0 {
		return nil
	}
	q := fmt.Sprintf(
		"INSERT INTO %s (%s, %s) VALUES ($1, $2) ON CONFLICT (%s) DO UPDATE SET %s = EXCLUDED.%s",
		p.CompanyTableName, idFieldName, jsonFieldName, idFieldName, jsonFieldName, jsonFieldName,
	)
	b := &pgx.Batch{}
	for id, j := range docs {
		b.Queue(q, id, j)
	}
	br := p.pool.SendBatch(ctx, b)
	defer func() {
		if err := br.Close(); err != nil {
			slog.Warn("could not close materialization batch", "error", err)
		}
	}()
	for range docs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("could not upsert materialized document: %w", err)
		}
	}
	return nil
}

// GetCompany returns the JSON of a company based on a CNPJ number.
func (p *PostgreSQL) GetCompany(ctx context.Context, id string) (string, error) {
	rows, err := p.pool.Query(ctx, p.getCompanyQuery, id)
	if err != nil {
		return "", fmt.Errorf("error looking for cnpj %s: %w", id, err)
	}
	j, err := pgx.CollectOneRow(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("error reading cnpj %s: %w", id, err)
	}
	return j, nil
}

// quotedList renders validated string params (see pagination.isValid: only
// A-Z, 0-9 and *) as a SQL list, e.g. 'SP', 'RJ'.
func quotedList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = "'" + v + "'"
	}
	return strings.Join(q, ", ")
}

func intList(vs []uint32) string {
	s := make([]string, len(vs))
	for i, v := range vs {
		s[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(s, ", ")
}

// searchQuery filters by joining the relational tables (native B-tree indexes on
// uf, município, CNAE and partner) and returns the matching JSONB documents.
// Param values are interpolated, which is safe because NewQuery validates them.
func (p *PostgreSQL) searchQuery(q *Query) *sqlbuilder.SelectBuilder {
	b := sqlbuilder.PostgreSQL.NewSelectBuilder()
	b.Select("c."+p.CursorFieldName, "c."+p.JSONFieldName)
	b.From(p.CompanyTableFullName() + " AS c")
	b.Join("estabelecimento AS e", "e.cnpj = c."+p.IDFieldName)
	if len(q.NaturezaJuridica) > 0 {
		b.JoinWithOption(sqlbuilder.LeftJoin, "empresa AS em", "em.cnpj_base = e.cnpj_base")
	}
	b.OrderByAsc("c." + p.CursorFieldName)
	b.Limit(int(q.Limit))
	if q.Cursor != nil {
		if c, err := q.CursorAsInt(); err == nil {
			b.Where(b.GreaterThan("c."+p.CursorFieldName, c))
		}
	}
	if len(q.UF) > 0 {
		b.Where(fmt.Sprintf("e.uf IN (%s)", quotedList(q.UF)))
	}
	if len(q.Municipio) > 0 {
		l := intList(q.Municipio)
		b.Where(fmt.Sprintf(
			"(e.codigo_municipio IN (%s) OR e.codigo_municipio IN (SELECT codigo_municipio FROM municipio_ibge WHERE codigo_ibge IN (%s)))",
			l, l,
		))
	}
	if len(q.NaturezaJuridica) > 0 {
		b.Where(fmt.Sprintf("em.codigo_natureza_juridica IN (%s)", intList(q.NaturezaJuridica)))
	}
	if len(q.CNAEFiscal) > 0 {
		b.Where(fmt.Sprintf("e.cnae_fiscal IN (%s)", intList(q.CNAEFiscal)))
	}
	if len(q.CNAE) > 0 {
		l := intList(q.CNAE)
		b.Where(fmt.Sprintf(
			"(e.cnae_fiscal IN (%s) OR EXISTS (SELECT 1 FROM estabelecimento_cnae ec WHERE ec.cnpj = c.%s AND ec.cnae IN (%s)))",
			l, p.IDFieldName, l,
		))
	}
	if len(q.CNPF) > 0 {
		b.Where(fmt.Sprintf(
			"EXISTS (SELECT 1 FROM socio s WHERE s.cnpj_base = e.cnpj_base AND s.cnpj_cpf_do_socio IN (%s))",
			quotedList(q.CNPF),
		))
	}
	return b
}

type postgresRecord struct {
	Cursor  int
	Company string
}

// Search returns paginated results with JSON for companies bases on a search
// query
func (p *PostgreSQL) Search(ctx context.Context, q *Query) (string, error) {
	s, a := p.searchQuery(q).Build()
	slog.Debug("paginated search", "query", s, "args", a)
	rows, err := p.pool.Query(ctx, s, a...)
	if err != nil {
		return "", fmt.Errorf("error searching for %#v: %w", q, err)
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[postgresRecord])
	if err != nil {
		return "", fmt.Errorf("error reading search result for %#v: %w", q, err)
	}
	var cs []string
	for _, r := range rs {
		cs = append(cs, r.Company)
	}
	var cur string
	if len(rs) == int(q.Limit) {
		cur = fmt.Sprintf("%d", rs[len(rs)-1].Cursor)
	}
	return newPage(cs, cur), nil

}

// PreLoad runs before starting to load data into the database. Currently it
// disables autovacuum on PostgreSQL.
func (p *PostgreSQL) PreLoad() error {
	s, err := p.renderTemplate("pre_load")
	if err != nil {
		return fmt.Errorf("error rendering pre-load template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error during pre load: %s\n%w", s, err)
	}
	return nil
}

// PostLoad runs after loading data into the database. Currently it re-enables
// autovacuum on PostgreSQL.
func (p *PostgreSQL) PostLoad() error {
	s, err := p.renderTemplate("post_load")
	if err != nil {
		return fmt.Errorf("error rendering post-load template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error during post load: %s\n%w", s, err)
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

// MetaRead reads a key/value pair from the metadata table.
func (p *PostgreSQL) MetaRead(k string) (string, error) {
	rows, err := p.pool.Query(context.Background(), p.metaReadQuery, k)
	if err != nil {
		return "", fmt.Errorf("error looking for metadata key %s: %w", k, err)
	}
	v, err := pgx.CollectOneRow(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("error reading for metadata key %s: %w", k, err)
	}
	return v, nil
}

// CreateExtraIndexes responsible for creating additional indexes in the database
func (p *PostgreSQL) CreateExtraIndexes(idxs []string) error {
	if err := transform.ValidateIndexes(idxs); err != nil {
		return fmt.Errorf("index name error: %w", err)
	}
	for _, idx := range idxs {
		i := ExtraIndex{
			IsRoot: !strings.Contains(idx, "."),
			Name:   fmt.Sprintf("json.%s", idx),
			Value:  idx,
		}
		p.ExtraIndexes = append(p.ExtraIndexes, i)
	}
	s, err := p.renderTemplate("extra_indexes")
	if err != nil {
		return fmt.Errorf("error rendering extra-indexes template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("expected the error to create indexe: %w", err)
	}
	slog.Info(fmt.Sprintf("%d Indexes successfully created in the table %s", len(idxs), p.CompanyTableName))
	return nil
}

// AllCompanies returns a paginated list of CNPJ numbers from the database.
func (p *PostgreSQL) AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error) {
	b := sqlbuilder.PostgreSQL.NewSelectBuilder().
		Select(p.CursorFieldName, p.IDFieldName).
		From(p.CompanyTableFullName()).
		OrderByAsc(p.CursorFieldName).
		Limit(int(limit))

	if cursor != nil {
		c, err := strconv.Atoi(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		b.Where(b.GreaterThan(p.CursorFieldName, c))
	}

	sql, args := b.Build()
	q, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("error listing CNPJs: %w", err)
	}

	rows, err := pgx.CollectRows(q, pgx.RowToStructByPos[postgresRecord])
	if err != nil {
		return nil, nil, fmt.Errorf("error reading CNPJs: %w", err)
	}

	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Company
	}

	if len(rows) < int(limit) {
		return ids, nil, nil
	}

	cur := fmt.Sprintf("%d", rows[len(rows)-1].Cursor)
	return ids, &cur, nil
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
		pool:             conn,
		uri:              uri,
		schema:           schema,
		CompanyTableName: companyTableName,
		GraphTableName:   graphTableName,
		MetaTableName:    metaTableName,
		CursorFieldName:  cursorFieldName,
		IDFieldName:      idFieldName,
		JSONFieldName:    jsonFieldName,
		KeyFieldName:     keyFieldName,
		ValueFieldName:   valueFieldName,
		Logged:           logged,
	}
	p.getCompanyQuery, err = p.renderTemplate("get")
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("error rendering get template: %w", err)
	}
	p.getNeighborsQuery, err = p.renderTemplate("get_neighbors")
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("error rendering get_neighbors template: %w", err)
	}
	p.metaReadQuery, err = p.renderTemplate("meta_read")
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("error rendering meta-read template: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.pool.Ping(ctx); err != nil {
		return PostgreSQL{}, fmt.Errorf("could not connect to postgres: %w", err)
	}
	return p, nil
}

// CreateGraphTable creates the graph table.
func (p *PostgreSQL) CreateGraphTable() error {
	slog.Info("Creating graph table")
	s, err := p.renderTemplate("graph")
	if err != nil {
		return fmt.Errorf("error rendering graph template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error creating graph table with: %s\n%w", s, err)
	}
	return nil
}

// GetRelated returns the adjacent nodes of a node in the graph.
func (p *PostgreSQL) GetRelated(ctx context.Context, id string) ([]GraphEdge, error) {
	rows, err := p.pool.Query(ctx, p.getNeighborsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("error looking for neighbors of %s: %w", id, err)
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[GraphEdge])
	if err != nil {
		return nil, fmt.Errorf("error reading neighbors of %s: %w", id, err)
	}
	return rs, nil
}
