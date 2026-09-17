package transform

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func loadAllTestSources(t *testing.T, kv *kv, srcs map[string]*source) string {
	t.Helper()
	ctx := context.Background()
	ts := testIBGEMunicipalitiesServer(t)
	defer ts.Close()

	ext := t.TempDir()
	pth := filepath.Join(testdataDir, "2026-01.zip")
	if err := unzipMainArchive(pth, ext, nil); err != nil {
		t.Fatalf("expected no error extracting archive, got %s", err)
	}
	for _, src := range srcs {
		var err error
		switch src.kind {
		case CompanySrc:
			err = loadCSVs(ctx, ext, src, nil, kv, false)
		case TaxSrc:
			err = loadCSVs(ctx, testdataDir, src, nil, kv, false)
		case IBGESrc:
			err = loadIBGEMunicipalitiesFromURL(ctx, ts.URL, src, nil, kv)
		}
		if err != nil {
			t.Fatalf("expected no error loading %s data, got %s", src.prefix, err)
		}
	}
	if err := kv.flush(); err != nil {
		t.Fatalf("expected no error flushing key-value storage, got %s", err)
	}
	return ext
}

var dataSituacaoCadastral = date(time.Date(2004, 5, 22, 0, 0, 0, 0, time.UTC))

func TestParseEstabelecimento(t *testing.T) {
	row := []string{
		"33683111",             // 0 CNPJ Base
		"0002",                 // 1 CNPJ Ordem
		"80",                   // 2 CNPJ DV
		"2",                    // 3 Indentificador Matriz/Filial
		"REGIONAL BRASILIA-DF", // 4 Nome Fantasia
		"02",                   // 5 Situação Cadastral
		"20040522",             // 6 Data Situação Cadastral
		"00",                   // 7 Motivo Situação Cadastral
		"",                     // 8 Nome da cidade no exterior
		"",                     // 9 Pais
		"19670630",             // 10 Data de Início da Ativiade
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
		t.Fatalf("expected no error creating kv, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	srcs := sources()
	loadAllTestSources(t, kv, srcs)
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	got, err := parseEstabelecimento(log, srcs, row)
	if err != nil {
		t.Fatalf("expected no error parsing the establishment, got %s", err)
	}
	if got.CNPJ != "33683111000280" {
		t.Errorf("expected cnpj to be 33683111000280, got %s", got.CNPJ)
	}
	if *got.IdentificadorMatrizFilial != 2 {
		t.Errorf("expected IdentificadorMatrizFilial to be 2, got %v", got.IdentificadorMatrizFilial)
	}
	if got.NomeFantasia != "REGIONAL BRASILIA-DF" {
		t.Errorf("expected NomeFantasia to be REGIONAL BRASILIA-DF, got %s", got.NomeFantasia)
	}
	if *got.SituacaoCadastral != 2 {
		t.Errorf("expected SituacaoCadastral to be 2, got %d", *got.SituacaoCadastral)
	}
	if *got.DataSituacaoCadastral != dataSituacaoCadastral {
		t.Errorf("expected SituacaoCadastral to be %v, got %v", dataSituacaoCadastral, *got.DataSituacaoCadastral)
	}
	if *got.MotivoSituacaoCadastral != 0 {
		t.Errorf("expected MotivoSituacaoCadastral to be 0, got %d", *got.MotivoSituacaoCadastral)
	}
	if got.NomeCidadeNoExterior != "" {
		t.Errorf("expected NomeCidadeNoExterior to be empty, got %s", got.NomeCidadeNoExterior)
	}
	if got.CodigoPais != nil {
		t.Errorf("expected CodigoPais to be nil, got %v", got.CodigoPais)
	}
	if *got.DataInicioAtividade != date(time.Date(1967, 6, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected DataInicioAtividade to be 1967-06-30, got %v", *got.DataInicioAtividade)
	}
	if *got.CNAEFiscal != 6204000 {
		t.Errorf("expected CNAEFiscal to be 6204000, got %d", *got.CNAEFiscal)
	}
	if got.DescricaoTipoDeLogradouro != "AVENIDA" {
		t.Errorf("expected DescricaoTipoDeLogradouro to be AVENIDA, got %s", got.DescricaoTipoDeLogradouro)
	}
	if got.Logradouro != "L2 SGAN" {
		t.Errorf("expected Logradouro to be L2 SGAN, got %s", got.Logradouro)
	}
	if got.Numero != "601" {
		t.Errorf("expected Numero to be 601, got %s", got.Numero)
	}
	if got.Complemento != "MODULO G" {
		t.Errorf("expected Complemento to be MODULO G, got %s", got.Complemento)
	}
	if got.Bairro != "ASA NORTE" {
		t.Errorf("expected Bairro to be ASA NORTE, got %s", got.Bairro)
	}
	if got.CEP != "70836900" {
		t.Errorf("expected CEP to be 70836900, got %s", got.CEP)
	}
	if got.UF != "DF" {
		t.Errorf("expected UF to be DF, got %s", got.UF)
	}
	if *got.CodigoMunicipio != 9701 {
		t.Errorf("expected CodigoMunicipio to be 9701, got %d", *got.CodigoMunicipio)
	}
	if got.Telefone1 != "" {
		t.Errorf("expected Telefone1 to be empty, got %s", got.Telefone1)
	}
	if got.Telefone2 != "" {
		t.Errorf("expected Telefone2 to be empty, got %s", got.Telefone2)
	}
	if got.Fax != "" {
		t.Errorf("expected Fax to be empty, got %s", got.Fax)
	}
	if got.Email == nil || *got.Email != "test@ser.pro" {
		t.Errorf("expected Email to be empty string, got %v", got.Email)
	}
	if got.SituacaoEspecial != "" {
		t.Errorf("expected SituacaoEspecial to be empty, got %s", got.SituacaoEspecial)
	}
	if got.DataSituacaoEspecial != nil {
		t.Errorf("expected DataSituacaoEspecial to be nil, got %v", got.DataSituacaoEspecial)
	}
	if !slices.Equal(got.CNAESecundarios, []int{6201501, 6202300, 6203100, 6209100, 6311900}) {
		t.Errorf("expected the 5 secondary CNAEs in order, got %v", got.CNAESecundarios)
	}
	if logs.Len() != 0 {
		t.Errorf("expected no warnings for known codes, got %s", logs.String())
	}

	t.Run("unknown reference codes are logged", func(t *testing.T) {
		unknown := slices.Clone(row)
		unknown[7] = "99"    // motivo
		unknown[9] = "999"   // país
		unknown[20] = "1234" // município (and its IBGE code)
		var logs bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logs, nil))
		if _, err := parseEstabelecimento(log, srcs, unknown); err != nil {
			t.Fatalf("expected no error parsing the establishment, got %s", err)
		}
		for _, msg := range []string{
			`"msg":"unknown MotivoSituacaoCadastral","cnpj":"33683111000280","code":"99"`,
			`"msg":"unknown CodigoPais","cnpj":"33683111000280","code":"999"`,
			`"msg":"unknown CodigoMunicipioIBGE","cnpj":"33683111000280","code":"1234"`,
			`"msg":"unknown Municipio","cnpj":"33683111000280","code":"1234"`,
		} {
			if !strings.Contains(logs.String(), msg) {
				t.Errorf("expected log to contain %s, got %s", msg, logs.String())
			}
		}
	})
}

func TestUnknownPartnerCountries(t *testing.T) {
	ctx := context.Background()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating kv, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	srcs := sources()
	loadAllTestSources(t, kv, srcs)
	partner := []string{"2", "FULANO", "***123456**", "49", "20200101", "999", "", "", "00", "5"}
	for range 2 {
		if err := kv.put(srcs["soc"], "12345678", partner); err != nil {
			t.Fatalf("expected no error adding a partner, got %s", err)
		}
	}
	if err := kv.flush(); err != nil {
		t.Fatalf("expected no error flushing kv, got %s", err)
	}
	db := &testDB{byBase: map[string][]string{"12345678": {"12345678000190", "12345678000270"}}}
	unknown, err := loadRelational(ctx, kv, db, srcs, "")
	if err != nil {
		t.Fatalf("expected no error loading relational data, got %s", err)
	}
	if !slices.Equal(unknown["12345678"], []string{"999", "999"}) {
		t.Errorf("expected two unknown country codes for base 12345678, got %v", unknown)
	}
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	if err := logUnknownPartnerCountries(ctx, log, db, "estabelecimento", unknown); err != nil {
		t.Fatalf("expected no error logging, got %s", err)
	}
	if n := strings.Count(logs.String(), `"msg":"unknown Pais"`); n != 4 {
		t.Errorf("expected 4 warnings (2 establishments x 2 partners), got %d: %s", n, logs.String())
	}
	for _, cnpj := range []string{"12345678000190", "12345678000270"} {
		if !strings.Contains(logs.String(), `"cnpj":"`+cnpj+`","code":"999"`) {
			t.Errorf("expected a warning for %s, got %s", cnpj, logs.String())
		}
	}
}
