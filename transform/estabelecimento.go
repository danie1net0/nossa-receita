package transform

import (
	"fmt"
	"log/slog"
	"strings"
)

// estabelecimento is one parsed row of the Estabelecimentos files.
type estabelecimento struct {
	CNPJ                      string
	IdentificadorMatrizFilial *int
	NomeFantasia              string
	SituacaoCadastral         *int
	DataSituacaoCadastral     *date
	MotivoSituacaoCadastral   *int
	NomeCidadeNoExterior      string
	CodigoPais                *int
	DataInicioAtividade       *date
	CNAEFiscal                *int
	CNAESecundarios           []int
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
	DataSituacaoEspecial      *date
}

// parseEstabelecimento parses a row of the Estabelecimentos files. Codes with no
// match in the reference files are logged as warnings (read by the report
// command); those checks use the in-memory copy the sources keep of them.
func parseEstabelecimento(log *slog.Logger, srcs map[string]*source, row []string) (*estabelecimento, error) {
	var e estabelecimento
	var err error
	e.CNPJ = strings.Join(row[:3], "")
	log = log.With("cnpj", e.CNPJ)
	e.IdentificadorMatrizFilial, err = toInt(row[3])
	if err != nil {
		return nil, fmt.Errorf("could not parse IdentificadorMatrizFilial for %s: %w", e.CNPJ, err)
	}
	if e.IdentificadorMatrizFilial == nil {
		return nil, fmt.Errorf("company %s missing IdentificadorMatrizFilial", e.CNPJ)
	}
	if v := *e.IdentificadorMatrizFilial; v != 1 && v != 2 {
		log.Error("unknown IdentificadorMatrizFilial", "value", v)
	}
	e.NomeFantasia = row[4]
	e.SituacaoCadastral, err = toInt(row[5])
	if err != nil {
		return nil, fmt.Errorf("could not parse SituacaoCadastral for %s: %w", e.CNPJ, err)
	}
	if e.SituacaoCadastral == nil {
		return nil, fmt.Errorf("company %s missing SituacaoCadastral", e.CNPJ)
	}
	switch *e.SituacaoCadastral {
	case 1, 2, 3, 4, 8:
	default:
		log.Error("unknown SituacaoCadastral", "value", *e.SituacaoCadastral)
	}
	e.DataSituacaoCadastral, err = toDate(row[6])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataSituacaoCadastral for %s: %w", e.CNPJ, err)
	}
	e.MotivoSituacaoCadastral, err = toInt(row[7])
	if err != nil {
		return nil, fmt.Errorf("could not parse MotivoSituacaoCadastral for %s: %w", e.CNPJ, err)
	}
	if !known(srcs["mot"], row[7], 0) {
		log.Warn("unknown MotivoSituacaoCadastral", "code", row[7])
	}
	e.NomeCidadeNoExterior = row[8]
	e.CodigoPais, err = toInt(row[9])
	if err != nil {
		return nil, fmt.Errorf("could not parse CodigoPais for %s: %w", e.CNPJ, err)
	}
	if !known(srcs["pai"], row[9], 0) {
		log.Warn("unknown CodigoPais", "code", row[9])
	}
	e.DataInicioAtividade, err = toDate(row[10])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataInicioAtividade for %s: %w", e.CNPJ, err)
	}
	e.CNAEFiscal, err = toInt(row[11])
	if err != nil {
		return nil, fmt.Errorf("could not parse CNAEFiscal for %s: %w", e.CNPJ, err)
	}
	for code := range strings.SplitSeq(row[12], ",") {
		if code == "" {
			continue
		}
		n, err := toInt(code)
		if err != nil {
			return nil, fmt.Errorf("could not parse CNAESecundarios for %s: %w", e.CNPJ, err)
		}
		e.CNAESecundarios = append(e.CNAESecundarios, *n)
	}
	e.DescricaoTipoDeLogradouro = row[13]
	e.Logradouro = row[14]
	e.Numero = row[15]
	e.Complemento = row[16]
	e.Bairro = row[17]
	e.CEP = row[18]
	e.UF = row[19]
	e.CodigoMunicipio, err = toInt(row[20])
	if err != nil {
		return nil, fmt.Errorf("could not parse CodigoMunicipio for %s: %w", e.CNPJ, err)
	}
	if e.CodigoMunicipio != nil && *e.CodigoMunicipio != 9707 { // 9707 is the overseas city code
		if ibge, ok := srcs["tab"].value(row[20], 3); !ok {
			log.Warn("unknown CodigoMunicipioIBGE", "code", row[20])
		} else if _, err := toInt(ibge); err != nil {
			return nil, fmt.Errorf("could not parse CodigoMunicipioIBGE number for %s: %w", e.CNPJ, err)
		}
	}
	if !known(srcs["mun"], row[20], 0) {
		log.Warn("unknown Municipio", "code", row[20])
	}
	e.Telefone1 = row[21] + row[22]
	e.Telefone2 = row[23] + row[24]
	e.Fax = row[25] + row[26]
	e.Email = &row[27]
	e.SituacaoEspecial = row[28]
	e.DataSituacaoEspecial, err = toDate(row[29])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataSituacaoEspecial for %s: %w", e.CNPJ, err)
	}
	return &e, nil
}

// known reports whether a code is empty (nothing to look up) or present in the
// reference file with a value at column idx.
func known(src *source, code string, idx int) bool {
	if code == "" {
		return true
	}
	_, ok := src.value(code, idx)
	return ok
}
