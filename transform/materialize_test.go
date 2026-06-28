package transform

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

// refMapsFromKV builds the reference lookups from the key-value store, standing
// in for the production load from the PostgreSQL reference tables.
func refMapsFromKV(t *testing.T, kv *kv) *refMaps {
	t.Helper()
	ref := &refMaps{
		cnae:         map[int]string{},
		motivo:       map[int]string{},
		municipio:    map[int]string{},
		pais:         map[int]string{},
		natureza:     map[int]string{},
		qualificacao: map[int]string{},
		ibge:         map[int]int{},
	}
	strMaps := map[string]map[int]string{
		"cna": ref.cnae,
		"mot": ref.motivo,
		"mun": ref.municipio,
		"pai": ref.pais,
		"nat": ref.natureza,
		"qua": ref.qualificacao,
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
					ref.ibge[*code] = *ibge
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

// stripResolved returns a deep-enough copy of c with every denormalized and
// derived field cleared, i.e. only the fields the relational tables store.
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

// TestResolveCompany proves the materialization (resolveCompany over the stored
// relational fields) reproduces the exact document the ETL writes.
func TestResolveCompany(t *testing.T) {
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
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	loadAllTestSources(t, kv)
	log := slog.Default()
	expected, err := newCompany(log, sources(), kv, row)
	if err != nil {
		t.Fatalf("expected no error creating a company, got %s", err)
	}
	pool := &sync.Pool{New: func() any { return &bytes.Buffer{} }}
	want, err := expected.JSON(pool)
	if err != nil {
		t.Fatalf("expected no error marshalling the reference company, got %s", err)
	}

	got := stripResolved(expected)
	if err := resolveCompany(got, refMapsFromKV(t, kv), log); err != nil {
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
