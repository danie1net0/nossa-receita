package transform

import (
	"log/slog"
	"sort"
)

// refMaps holds the code->description lookups (plus the municipal IBGE-code map)
// used to resolve a Company's denormalized fields during materialization. In
// production they are loaded once from the reference tables.
type refMaps struct {
	cnae         map[int]string
	motivo       map[int]string
	municipio    map[int]string
	pais         map[int]string
	natureza     map[int]string
	qualificacao map[int]string
	ibge         map[int]int
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
func resolveCompany(c *Company, ref *refMaps, log *slog.Logger) error {
	if err := c.descricaoMatrizFilial(log); err != nil {
		return err
	}
	if err := c.descricaoSituacaoCadastral(log); err != nil {
		return err
	}
	if err := c.porte(log); err != nil {
		return err
	}
	c.DescricaoMotivoSituacaoCadastral = lookupStr(ref.motivo, c.MotivoSituacaoCadastral)
	c.Pais = lookupStr(ref.pais, c.CodigoPais)
	c.CNAEFiscalDescricao = lookupStr(ref.cnae, c.CNAEFiscal)
	c.Municipio = lookupStr(ref.municipio, c.CodigoMunicipio)
	c.NaturezaJuridica = lookupStr(ref.natureza, c.CodigoNaturezaJuridica)
	c.CodigoMunicipioIBGE = nil
	if c.CodigoMunicipio != nil && *c.CodigoMunicipio != 9707 { // 9707 is the overseas city code
		if v, ok := ref.ibge[*c.CodigoMunicipio]; ok {
			ibge := v
			c.CodigoMunicipioIBGE = &ibge
		}
	}
	for i := range c.QuadroSocietario {
		p := &c.QuadroSocietario[i]
		p.QualificaoSocio = lookupStr(ref.qualificacao, p.CodigoQualificacaoSocio)
		p.Pais = lookupStr(ref.pais, p.CodigoPais)
		p.QualificacaoRepresentanteLegal = lookupStr(ref.qualificacao, p.CodigoQualificacaoRepresentanteLegal)
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
		if d, ok := ref.cnae[c.CNAESecundarios[i].Codigo]; ok {
			c.CNAESecundarios[i].Descricao = d
		}
	}
	sort.Slice(c.RegimeTributario, func(i, j int) bool {
		return c.RegimeTributario[i].Ano < c.RegimeTributario[j].Ano
	})
	return nil
}
