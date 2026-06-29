package transform

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// refMapsFromKV builds the reference lookups from the key-value store, standing
// in for the production load from the PostgreSQL reference tables.
func refMapsFromKV(t *testing.T, kv *kv) *RefMaps {
	t.Helper()
	ref := &RefMaps{
		CNAE:         map[int]string{},
		Motivo:       map[int]string{},
		Municipio:    map[int]string{},
		Pais:         map[int]string{},
		Natureza:     map[int]string{},
		Qualificacao: map[int]string{},
		IBGE:         map[int]int{},
	}
	strMaps := map[string]map[int]string{
		"cna": ref.CNAE,
		"mot": ref.Motivo,
		"mun": ref.Municipio,
		"pai": ref.Pais,
		"nat": ref.Natureza,
		"qua": ref.Qualificacao,
	}
	err := kv.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			parts := strings.SplitN(string(item.Key()), "::", 3)
			if len(parts) < 2 {
				continue
			}
			code, err := toInt(parts[0])
			if err != nil || code == nil {
				continue
			}
			var val []string
			if err := item.Value(func(b []byte) error {
				var e error
				val, e = kv.deserialize(b)
				return e
			}); err != nil {
				return err
			}
			if m, ok := strMaps[parts[1]]; ok && len(val) >= 1 {
				m[*code] = val[0]
			} else if parts[1] == "tab" && len(val) >= 4 {
				if ibge, err := toInt(val[3]); err == nil && ibge != nil {
					ref.IBGE[*code] = *ibge
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error building reference maps, got %s", err)
	}
	return ref
}

// newTestCompany builds the reference Company (via the Badger ETL path) for the
// sample CNPJ, loading all test sources into kv along the way.
func newTestCompany(t *testing.T, kv *kv) *Company {
	t.Helper()
	row := []string{
		"33683111",             // 0 CNPJ Base
		"0002",                 // 1 CNPJ Ordem
		"80",                   // 2 CNPJ DV
		"2",                    // 3 Identificador Matriz/Filial
		"REGIONAL BRASILIA-DF", // 4 Nome Fantasia
		"02",                   // 5 Situação Cadastral
		"20040522",             // 6 Data Situação Cadastral
		"00",                   // 7 Motivo Situação Cadastral
		"",                     // 8 Nome da cidade no exterior
		"",                     // 9 Pais
		"19670630",             // 10 Data de Início da Atividade
		"6204000",              // 11 CNAE Fiscal
		"6201501,6202300,6203100,6209100,6311900", // 12 CNAEs Secundários
		"AVENIDA",      // 13 Tipo de Logradouro
		"L2 SGAN",      // 14 Logradouro
		"601",          // 15 Número
		"MODULO G",     // 16 Complemento
		"ASA NORTE",    // 17 Bairro
		"70836900",     // 18 CEP
		"DF",           // 19 UF
		"9701",         // 20 Município
		"",             // 21 DDD 1
		"",             // 22 Telefone 1
		"",             // 23 DDD 2
		"",             // 24 Telefone 2
		"",             // 25 DDD Fax
		"",             // 26 Fax
		"test@ser.pro", // 27 Email
		"",             // 28 Situação Especial
		"",             // 29 Data Situação Especial
	}
	loadAllTestSources(t, kv)
	c, err := newCompany(slog.Default(), sources(), kv, row)
	if err != nil {
		t.Fatalf("expected no error creating a company, got %s", err)
	}
	return c
}

// stripResolved returns a copy of c with every denormalized and derived field
// cleared, i.e. only the fields the relational tables store.
func stripResolved(src *Company) *Company {
	c := *src
	c.QuadroSocietario = append([]Partner(nil), src.QuadroSocietario...)
	c.CNAESecundarios = append([]CNAE(nil), src.CNAESecundarios...)
	c.RegimeTributario = append([]TaxRegime(nil), src.RegimeTributario...)
	c.DescricaoMatrizFilial = nil
	c.DescricaoSituacaoCadastral = nil
	c.DescricaoMotivoSituacaoCadastral = nil
	c.Pais = nil
	c.CNAEFiscalDescricao = nil
	c.Municipio = nil
	c.CodigoMunicipioIBGE = nil
	c.NaturezaJuridica = nil
	c.Porte = nil
	for i := range c.QuadroSocietario {
		c.QuadroSocietario[i].QualificaoSocio = nil
		c.QuadroSocietario[i].Pais = nil
		c.QuadroSocietario[i].QualificacaoRepresentanteLegal = nil
		c.QuadroSocietario[i].FaixaEtaria = nil
	}
	for i := range c.CNAESecundarios {
		c.CNAESecundarios[i].Descricao = ""
	}
	return &c
}

func timeFromDate(d *date) *time.Time {
	if d == nil {
		return nil
	}
	t := time.Time(*d)
	return &t
}

// relationalFromCompany projects a Company onto the stored relational fields,
// mirroring what the database layer reads back from the relational tables.
func relationalFromCompany(c *Company) *RelationalCompany {
	rc := &RelationalCompany{
		CNPJ:                      c.CNPJ,
		IdentificadorMatrizFilial: c.IdentificadorMatrizFilial,
		NomeFantasia:              c.NomeFantasia,
		SituacaoCadastral:         c.SituacaoCadastral,
		DataSituacaoCadastral:     timeFromDate(c.DataSituacaoCadastral),
		MotivoSituacaoCadastral:   c.MotivoSituacaoCadastral,
		NomeCidadeNoExterior:      c.NomeCidadeNoExterior,
		CodigoPais:                c.CodigoPais,
		DataInicioAtividade:       timeFromDate(c.DataInicioAtividade),
		CNAEFiscal:                c.CNAEFiscal,
		DescricaoTipoDeLogradouro: c.DescricaoTipoDeLogradouro,
		Logradouro:                c.Logradouro,
		Numero:                    c.Numero,
		Complemento:               c.Complemento,
		Bairro:                    c.Bairro,
		CEP:                       c.CEP,
		UF:                        c.UF,
		CodigoMunicipio:           c.CodigoMunicipio,
		Telefone1:                 c.Telefone1,
		Telefone2:                 c.Telefone2,
		Fax:                       c.Fax,
		Email:                     c.Email,
		SituacaoEspecial:          c.SituacaoEspecial,
		DataSituacaoEspecial:      timeFromDate(c.DataSituacaoEspecial),
		RazaoSocial:               c.RazaoSocial,
		CodigoNaturezaJuridica:    c.CodigoNaturezaJuridica,
		QualificacaoDoResponsavel: c.QualificacaoDoResponsavel,
		CodigoPorte:               c.CodigoPorte,
		EnteFederativoResponsavel: c.EnteFederativoResponsavel,
		OpcaoPeloSimples:          c.OpcaoPeloSimples,
		DataOpcaoPeloSimples:      timeFromDate(c.DataOpcaoPeloSimples),
		DataExclusaoDoSimples:     timeFromDate(c.DataExclusaoDoSimples),
		OpcaoPeloMEI:              c.OpcaoPeloMEI,
		DataOpcaoPeloMEI:          timeFromDate(c.DataOpcaoPeloMEI),
		DataExclusaoDoMEI:         timeFromDate(c.DataExclusaoDoMEI),
	}
	if c.CapitalSocial != nil {
		f := float64(*c.CapitalSocial)
		rc.CapitalSocial = &f
	}
	for _, p := range c.QuadroSocietario {
		rc.Socios = append(rc.Socios, RelationalPartner{
			IdentificadorDeSocio:                 p.IdentificadorDeSocio,
			NomeSocio:                            p.NomeSocio,
			CNPJCPFDoSocio:                       p.CNPJCPFDoSocio,
			CodigoQualificacaoSocio:              p.CodigoQualificacaoSocio,
			DataEntradaSociedade:                 timeFromDate(p.DataEntradaSociedade),
			CodigoPais:                           p.CodigoPais,
			CPFRepresentanteLegal:                p.CPFRepresentanteLegal,
			NomeRepresentanteLegal:               p.NomeRepresentanteLegal,
			CodigoQualificacaoRepresentanteLegal: p.CodigoQualificacaoRepresentanteLegal,
			CodigoFaixaEtaria:                    p.CodigoFaixaEtaria,
		})
	}
	for _, cnae := range c.CNAESecundarios {
		rc.CNAESecundarios = append(rc.CNAESecundarios, cnae.Codigo)
	}
	for _, r := range c.RegimeTributario {
		rc.Regimes = append(rc.Regimes, RelationalRegime{
			Ano:                       r.Ano,
			CNPJDaSCP:                 r.CNPJDaSCP,
			FormaDeTributacao:         r.FormaDeTributação,
			QuantidadeDeEscrituracoes: r.QuantidadeDeEscrituracoes,
		})
	}
	return rc
}

// TestResolveCompany proves the materialization (resolveCompany over the stored
// relational fields) reproduces the exact document the ETL writes.
func TestResolveCompany(t *testing.T) {
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	expected := newTestCompany(t, kv)
	pool := &sync.Pool{New: func() any { return &bytes.Buffer{} }}
	want, err := expected.JSON(pool)
	if err != nil {
		t.Fatalf("expected no error marshalling the reference company, got %s", err)
	}

	got := stripResolved(expected)
	if err := resolveCompany(got, refMapsFromKV(t, kv), slog.Default()); err != nil {
		t.Fatalf("expected no error resolving the company, got %s", err)
	}
	have, err := got.JSON(pool)
	if err != nil {
		t.Fatalf("expected no error marshalling the resolved company, got %s", err)
	}
	if want != have {
		t.Errorf("resolved document differs from the ETL output:\nwant: %s\nhave: %s", want, have)
	}
}

// TestMaterializeRoundtrip proves that projecting a Company onto the relational
// fields and rebuilding it (toCompany + resolveCompany) reproduces the document.
func TestMaterializeRoundtrip(t *testing.T) {
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	expected := newTestCompany(t, kv)
	pool := &sync.Pool{New: func() any { return &bytes.Buffer{} }}
	want, err := expected.JSON(pool)
	if err != nil {
		t.Fatalf("expected no error marshalling the reference company, got %s", err)
	}

	got := relationalFromCompany(expected).toCompany()
	if err := resolveCompany(got, refMapsFromKV(t, kv), slog.Default()); err != nil {
		t.Fatalf("expected no error resolving the company, got %s", err)
	}
	have, err := got.JSON(pool)
	if err != nil {
		t.Fatalf("expected no error marshalling the materialized company, got %s", err)
	}
	if want != have {
		t.Errorf("materialized document differs from the ETL output:\nwant: %s\nhave: %s", want, have)
	}
}
