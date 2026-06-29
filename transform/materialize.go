package transform

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// RefMaps holds the code->description lookups (plus the municipal IBGE-code map)
// used to resolve a Company's denormalized fields during materialization. In
// production they are loaded once from the reference tables.
type RefMaps struct {
	CNAE         map[int]string
	Motivo       map[int]string
	Municipio    map[int]string
	Pais         map[int]string
	Natureza     map[int]string
	Qualificacao map[int]string
	IBGE         map[int]int
}

// RelationalCompany carries the stored (code and literal) values of one CNPJ as
// kept in the relational tables — i.e. the document without its resolved or
// derived fields. The database layer populates it; toCompany turns it back into
// a Company. Dates use *time.Time so the type can cross the package boundary.
type RelationalCompany struct {
	// estabelecimento
	CNPJ                      string
	IdentificadorMatrizFilial *int
	NomeFantasia              string
	SituacaoCadastral         *int
	DataSituacaoCadastral     *time.Time
	MotivoSituacaoCadastral   *int
	NomeCidadeNoExterior      string
	CodigoPais                *int
	DataInicioAtividade       *time.Time
	CNAEFiscal                *int
	DescricaoTipoDeLogradouro string
	Logradouro                string
	Numero                    string
	Complemento               string
	Bairro                    string
	CEP                       string
	UF                        string
	CodigoMunicipio           *int
	Telefone1                 string
	Telefone2                 string
	Fax                       string
	Email                     *string
	SituacaoEspecial          string
	DataSituacaoEspecial      *time.Time
	// empresa (by base)
	RazaoSocial               string
	CodigoNaturezaJuridica    *int
	QualificacaoDoResponsavel *int
	CapitalSocial             *float64
	CodigoPorte               *int
	EnteFederativoResponsavel string
	// simples (by base)
	OpcaoPeloSimples      *bool
	DataOpcaoPeloSimples  *time.Time
	DataExclusaoDoSimples *time.Time
	OpcaoPeloMEI          *bool
	DataOpcaoPeloMEI      *time.Time
	DataExclusaoDoMEI     *time.Time
	// children
	Socios          []RelationalPartner
	CNAESecundarios []int
	Regimes         []RelationalRegime
}

// RelationalPartner carries the stored fields of one partner (socio table row).
type RelationalPartner struct {
	IdentificadorDeSocio                 *int
	NomeSocio                            string
	CNPJCPFDoSocio                       string
	CodigoQualificacaoSocio              *int
	DataEntradaSociedade                 *time.Time
	CodigoPais                           *int
	CPFRepresentanteLegal                string
	NomeRepresentanteLegal               string
	CodigoQualificacaoRepresentanteLegal *int
	CodigoFaixaEtaria                    *int
}

// RelationalRegime carries the stored fields of one tax-regime row.
type RelationalRegime struct {
	Ano                       int
	CNPJDaSCP                 *string
	FormaDeTributacao         string
	QuantidadeDeEscrituracoes int
}

// MaterializeDB is the database surface needed to materialize documents from the
// relational tables. It is implemented by the PostgreSQL backend.
type MaterializeDB interface {
	ReferenceMaps(ctx context.Context) (*RefMaps, error)
	RelationalCompanies(ctx context.Context, ids []string) ([]*RelationalCompany, error)
	SaveDocuments(ctx context.Context, docs map[string]string) error
}

func dateFromTime(t *time.Time) *date {
	if t == nil {
		return nil
	}
	d := date(*t)
	return &d
}

// toCompany turns the stored relational values back into a Company with only the
// stored fields set; resolveCompany then fills the denormalized/derived ones.
func (r *RelationalCompany) toCompany() *Company {
	c := &Company{
		CNPJ:                      r.CNPJ,
		IdentificadorMatrizFilial: r.IdentificadorMatrizFilial,
		NomeFantasia:              r.NomeFantasia,
		SituacaoCadastral:         r.SituacaoCadastral,
		DataSituacaoCadastral:     dateFromTime(r.DataSituacaoCadastral),
		MotivoSituacaoCadastral:   r.MotivoSituacaoCadastral,
		NomeCidadeNoExterior:      r.NomeCidadeNoExterior,
		CodigoPais:                r.CodigoPais,
		DataInicioAtividade:       dateFromTime(r.DataInicioAtividade),
		CNAEFiscal:                r.CNAEFiscal,
		DescricaoTipoDeLogradouro: r.DescricaoTipoDeLogradouro,
		Logradouro:                r.Logradouro,
		Numero:                    r.Numero,
		Complemento:               r.Complemento,
		Bairro:                    r.Bairro,
		CEP:                       r.CEP,
		UF:                        r.UF,
		CodigoMunicipio:           r.CodigoMunicipio,
		Telefone1:                 r.Telefone1,
		Telefone2:                 r.Telefone2,
		Fax:                       r.Fax,
		Email:                     r.Email,
		SituacaoEspecial:          r.SituacaoEspecial,
		DataSituacaoEspecial:      dateFromTime(r.DataSituacaoEspecial),
		RazaoSocial:               r.RazaoSocial,
		CodigoNaturezaJuridica:    r.CodigoNaturezaJuridica,
		QualificacaoDoResponsavel: r.QualificacaoDoResponsavel,
		CodigoPorte:               r.CodigoPorte,
		EnteFederativoResponsavel: r.EnteFederativoResponsavel,
		OpcaoPeloSimples:          r.OpcaoPeloSimples,
		DataOpcaoPeloSimples:      dateFromTime(r.DataOpcaoPeloSimples),
		DataExclusaoDoSimples:     dateFromTime(r.DataExclusaoDoSimples),
		OpcaoPeloMEI:              r.OpcaoPeloMEI,
		DataOpcaoPeloMEI:          dateFromTime(r.DataOpcaoPeloMEI),
		DataExclusaoDoMEI:         dateFromTime(r.DataExclusaoDoMEI),
	}
	if r.CapitalSocial != nil {
		f := float32(*r.CapitalSocial)
		c.CapitalSocial = &f
	}
	for _, p := range r.Socios {
		c.QuadroSocietario = append(c.QuadroSocietario, Partner{
			IdentificadorDeSocio:                 p.IdentificadorDeSocio,
			NomeSocio:                            p.NomeSocio,
			CNPJCPFDoSocio:                       p.CNPJCPFDoSocio,
			CodigoQualificacaoSocio:              p.CodigoQualificacaoSocio,
			DataEntradaSociedade:                 dateFromTime(p.DataEntradaSociedade),
			CodigoPais:                           p.CodigoPais,
			CPFRepresentanteLegal:                p.CPFRepresentanteLegal,
			NomeRepresentanteLegal:               p.NomeRepresentanteLegal,
			CodigoQualificacaoRepresentanteLegal: p.CodigoQualificacaoRepresentanteLegal,
			CodigoFaixaEtaria:                    p.CodigoFaixaEtaria,
		})
	}
	for _, code := range r.CNAESecundarios {
		c.CNAESecundarios = append(c.CNAESecundarios, CNAE{Codigo: code})
	}
	for _, t := range r.Regimes {
		c.RegimeTributario = append(c.RegimeTributario, TaxRegime{
			Ano:                       t.Ano,
			CNPJDaSCP:                 t.CNPJDaSCP,
			FormaDeTributação:         t.FormaDeTributacao,
			QuantidadeDeEscrituracoes: t.QuantidadeDeEscrituracoes,
		})
	}
	return c
}

func lookupStr(m map[int]string, code *int) *string {
	if code == nil {
		return nil
	}
	if v, ok := m[*code]; ok {
		return &v
	}
	return nil
}

// faixaEtaria maps a partner's age-bracket code to its description (shared by
// the ETL enrichment and the materialization).
func faixaEtaria(code int) string {
	switch code {
	case 0:
		return "Não se aplica"
	case 1:
		return "Entre 0 a 12 anos"
	case 2:
		return "Entre 13 a 20 ano"
	case 3:
		return "Entre 21 a 30 anos"
	case 4:
		return "Entre 31 a 40 anos"
	case 5:
		return "Entre 41 a 50 anos"
	case 6:
		return "Entre 51 a 60 anos"
	case 7:
		return "Entre 61 a 70 anos"
	case 8:
		return "Entre 71 a 80 anos"
	case 9:
		return "Maiores de 80 anos"
	default:
		return ""
	}
}

// resolveCompany fills the denormalized and derived fields of a Company whose
// stored (code and literal) fields are already set, turning the relational
// representation back into the document shape produced by the ETL. Reusing the
// existing derivation methods and Company.JSON keeps the materialized document
// identical to the dual-write output.
func resolveCompany(c *Company, ref *RefMaps, log *slog.Logger) error {
	if err := c.descricaoMatrizFilial(log); err != nil {
		return err
	}
	if err := c.descricaoSituacaoCadastral(log); err != nil {
		return err
	}
	if err := c.porte(log); err != nil {
		return err
	}
	c.DescricaoMotivoSituacaoCadastral = lookupStr(ref.Motivo, c.MotivoSituacaoCadastral)
	c.Pais = lookupStr(ref.Pais, c.CodigoPais)
	c.CNAEFiscalDescricao = lookupStr(ref.CNAE, c.CNAEFiscal)
	c.Municipio = lookupStr(ref.Municipio, c.CodigoMunicipio)
	c.NaturezaJuridica = lookupStr(ref.Natureza, c.CodigoNaturezaJuridica)
	c.CodigoMunicipioIBGE = nil
	if c.CodigoMunicipio != nil && *c.CodigoMunicipio != 9707 { // 9707 is the overseas city code
		if v, ok := ref.IBGE[*c.CodigoMunicipio]; ok {
			ibge := v
			c.CodigoMunicipioIBGE = &ibge
		}
	}
	for i := range c.QuadroSocietario {
		p := &c.QuadroSocietario[i]
		p.QualificaoSocio = lookupStr(ref.Qualificacao, p.CodigoQualificacaoSocio)
		p.Pais = lookupStr(ref.Pais, p.CodigoPais)
		p.QualificacaoRepresentanteLegal = lookupStr(ref.Qualificacao, p.CodigoQualificacaoRepresentanteLegal)
		p.FaixaEtaria = nil
		if p.CodigoFaixaEtaria != nil {
			if f := faixaEtaria(*p.CodigoFaixaEtaria); f != "" {
				p.FaixaEtaria = &f
			}
		}
	}
	sort.Slice(c.QuadroSocietario, func(i, j int) bool {
		return c.QuadroSocietario[i].NomeSocio < c.QuadroSocietario[j].NomeSocio
	})
	for i := range c.CNAESecundarios {
		if d, ok := ref.CNAE[c.CNAESecundarios[i].Codigo]; ok {
			c.CNAESecundarios[i].Descricao = d
		}
	}
	sort.Slice(c.RegimeTributario, func(i, j int) bool {
		return c.RegimeTributario[i].Ano < c.RegimeTributario[j].Ano
	})
	return nil
}

// Materialize (re)builds the cnpj JSONB document for the given CNPJs from the
// relational tables and upserts them. It is the regeneration primitive used by
// incremental updates: changing relational data, then materializing the
// affected CNPJs. The relational tables store raw data, so privacy stripping is
// applied here (matching the ETL's privacy flag) rather than at write time.
func Materialize(ctx context.Context, db MaterializeDB, ids []string, privacy bool, log *slog.Logger) error {
	if len(ids) == 0 {
		return nil
	}
	ref, err := db.ReferenceMaps(ctx)
	if err != nil {
		return fmt.Errorf("could not load reference maps: %w", err)
	}
	rcs, err := db.RelationalCompanies(ctx, ids)
	if err != nil {
		return fmt.Errorf("could not load relational companies: %w", err)
	}
	pool := &sync.Pool{New: func() any { return &bytes.Buffer{} }}
	docs := make(map[string]string, len(rcs))
	for _, rc := range rcs {
		c := rc.toCompany()
		if err := resolveCompany(c, ref, log); err != nil {
			return fmt.Errorf("could not resolve company %s: %w", c.CNPJ, err)
		}
		if privacy {
			c.withPrivacy()
		}
		j, err := c.JSON(pool)
		if err != nil {
			return fmt.Errorf("could not marshal company %s: %w", c.CNPJ, err)
		}
		docs[c.CNPJ] = j
	}
	return db.SaveDocuments(ctx, docs)
}
