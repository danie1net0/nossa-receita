package transform

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

// PostgresBatchSize is the number of rows buffered per COPY into the relational
// tables.
const PostgresBatchSize = 8192

// In Oct. 2025 the Federal Revenue started using the country code 367. which is
// not present in Paises.zip. The issue was officially reported to them via
// Fala.BR. They replied but did not seem to care about updating the dataset.
//
// It seems safe to assume this is England:
// 1. Other official documents from the institution uses 367 for England, eg.:
// https://balanca.economia.gov.br/balanca/bd/tabelas/PAIS.csv or
// https://www.cenofisco.com.br/arquivos/BDFlash/IR_IN_RFB_1076.pdf
// 2. Paises.zip contains a CSV ordered by country name and “Inglaterra” would
// match this ordering
//
// The same logic was used to other unmatched country codes:
var extraCounties = map[int]string{
	15:  "Aland, Ilhas",
	150: "Canal, Ilhas do (Guernsey)",
	151: "Canárias, Ilhas",
	200: "Curaçao",
	321: "Guernsey",
	359: "Ilha de Man",
	367: "Inglaterra",
	393: "Jersey",
	449: "Macedônia",
	452: "Madeira, Ilha da",
	498: "Montenegro",
	578: "Palestina",
	678: "Saint Kitts e Nevis",
	699: "Sint Maarten",
	737: "Sérvia",
	994: "A Designar",
}

type database interface {
	PreLoad() error
	CopyRelational(context.Context, string, []string, [][]any) error
	CNPJsByBase(context.Context, string, []string) (map[string][]string, error)
	PostLoad() error
	MetaSave(string, string) error
}

func sources() map[string]*source { // all but Estabelecimentos (this one is loaded later on)
	srcs := []*source{
		newCompanySrc("Cnaes", ';', false, false),
		newCompanySrc("Empresas", ';', false, false),
		newTaxSrc("Imunes e Isentas", "entidades-imunes-e-isentas", ',', true, true),
		newTaxSrc("Lucro Arbitrado", "entidades-lucro-arbitrado", ',', true, true),
		newTaxSrc("Lucro Presumido", "entidades-lucro-presumido", ',', true, true),
		newTaxSrc("Lucro Real", "entidades-lucro-real", ',', true, true),
		newCompanySrc("Motivos", ';', false, false),
		newCompanySrc("Municipios", ';', false, false),
		newCompanySrc("Naturezas", ';', false, false),
		newCompanySrc("Paises", ';', false, false),
		newCompanySrc("Qualificacoes", ';', false, false),
		newCompanySrc("Simples", ';', false, false),
		newCompanySrc("Socios", ';', false, true),
		newIBGESrc("tabmun", ';', false, false),
	}
	m := make(map[string]*source)
	for _, src := range srcs {
		m[src.key] = src
	}
	for _, k := range []string{"mot", "pai", "mun", "tab"} {
		m[k].values = make(map[string][]string)
	}
	return m
}

// progressBarThrottle limits how often the bars are redrawn. The ETL adds to
// them for every chunk or row it processes, and redrawing each time made the
// whole load about ten times slower when the output goes to a file or a pipe.
const progressBarThrottle = 250 * time.Millisecond

func newProgressBar(label string, srcs int) (*progressbar.ProgressBar, error) {
	bar := progressbar.NewOptions(
		srcs, // it has a bug starting At zero, so we compensate for it later
		progressbar.OptionThrottle(progressBarThrottle),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetDescription(label),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)
	return bar, bar.RenderBlank()
}

func saveUpdatedAt(db database, date string) error {
	slog.Info("Saving the updated at date to the database…")
	return db.MetaSave("updated-at", date)
}

func postLoad(db database) error {
	slog.Info("Consolidating the database…")
	if err := db.PostLoad(); err != nil {
		return err
	}
	slog.Info("Database consolidated!")
	return nil
}

func Transform(dir string, db database, batch int) error {
	ibgeMunicipalitiesURL, err := ibgeMunicipalitiesURL()
	if err != nil {
		return fmt.Errorf("could not discover ibge municipalities URL: %w", err)
	}
	return transform(dir, db, batch, ibgeMunicipalitiesURL)
}

func transform(dir string, db database, batch int, ibgeMunicipalitiesURL string) error {
	if err := db.PreLoad(); err != nil {
		return err
	}
	label, err := loadSnapshot(dir, db, batch, ibgeMunicipalitiesURL, "")
	if err != nil {
		return err
	}
	if err := postLoad(db); err != nil {
		return err
	}
	return saveUpdatedAt(db, label)
}

// loadSnapshot extracts the source archive, loads the lookup/relational data
// into the key-value store, then writes the relational tables. The suffix
// targets the tables (e.g. "_staging"). It returns the snapshot label (YYYY-MM).
func loadSnapshot(dir string, db database, batch int, ibgeMunicipalitiesURL, suffix string) (string, error) {
	srcs := sources()
	pth, err := findMainZIP(dir)
	if err != nil {
		return "", fmt.Errorf("could not find main zip file in %s: %w", dir, err)
	}
	tmp, err := os.MkdirTemp("", fmt.Sprintf("minha-receita-%s-*", time.Now().Format("20060102150405")))
	if err != nil {
		return "", fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("could not remove temporary directory", "path", tmp, "error", err)
		}
	}()
	ext := filepath.Join(tmp, "extracted")
	if err := os.Mkdir(ext, 0700); err != nil {
		return "", fmt.Errorf("could not create extraction directory: %w", err)
	}
	bar, err := newProgressBar("[1/3] Extracting main archive", 1)
	if err != nil {
		return "", fmt.Errorf("could not create a progress bar: %w", err)
	}
	if err := unzipMainArchive(pth, ext, bar); err != nil {
		return "", fmt.Errorf("could not extract %s: %w", pth, err)
	}
	kv, err := newBadger(tmp, false)
	if err != nil {
		return "", fmt.Errorf("could not create badger database: %w", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			slog.Warn("could not close badger database", "error", err)
		}
	}()
	bar, err = newProgressBar("[2/3] Loading data to key-value storage", len(srcs))
	if err != nil {
		return "", fmt.Errorf("could not create a progress bar: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var g errgroup.Group
	for _, src := range srcs {
		g.Go(func() error {
			switch src.kind {
			case CompanySrc:
				return loadCSVs(ctx, ext, src, bar, kv, true)
			case TaxSrc:
				return loadCSVs(ctx, dir, src, bar, kv, false)
			case IBGESrc:
				return loadIBGEMunicipalitiesFromURL(ctx, ibgeMunicipalitiesURL, src, bar, kv)
			}
			return fmt.Errorf("unknown source kind %d for %s", src.kind, src.prefix)
		})
	}
	for k, v := range extraCounties {
		g.Go(func() error {
			return kv.put(srcs["pai"], fmt.Sprintf("%d", k), []string{v})
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}
	if err := kv.flush(); err != nil {
		return "", fmt.Errorf("could not flush key-value storage: %w", err)
	}
	log, closeLog, err := newETLLog()
	if err != nil {
		return "", err
	}
	defer closeLog()
	var unknownPais map[string][]string
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		unknownPais, err = loadRelational(egctx, kv, db, srcs, suffix)
		return err
	})
	eg.Go(func() error { return loadEstabelecimentos(egctx, log, srcs, db, batch, ext, suffix) })
	if err := eg.Wait(); err != nil {
		return "", err
	}
	if err := logUnknownPartnerCountries(ctx, log, db, "estabelecimento"+suffix, unknownPais); err != nil {
		return "", err
	}
	return strings.TrimSuffix(filepath.Base(pth), ".zip"), nil
}

// newETLLog creates the JSON log file with the data inconsistencies found in
// the load (read by the report command). The returned function closes it and
// deletes it if nothing was logged.
func newETLLog() (*slog.Logger, func(), error) {
	n := fmt.Sprintf("minha-receita-etl-%s.log", time.Now().Format("20060102150405"))
	f, err := os.Create(n)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create log file %s: %w", n, err)
	}
	closeLog := func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close the log file", "name", n, "error", err)
		}
		i, err := os.Stat(n)
		if err != nil {
			slog.Warn("could not check log file size", "name", n, "error", err)
			return
		}
		if i.Size() != 0 {
			return
		}
		if err := os.Remove(n); err != nil {
			slog.Warn("could not delete empty log file", "name", n, "error", err)
		}
	}
	return slog.New(slog.NewJSONHandler(f, nil)), closeLog, nil
}

func Cleanup() error {
	return filepath.WalkDir(os.TempDir(), func(pth string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "minha-receita-") {
			return nil
		}
		part := strings.Split(d.Name(), "-")
		if len(part) != 4 {
			return nil
		}
		if _, err := time.Parse("20060102150405", part[2]); err != nil {
			return nil
		}
		fmt.Printf("Removing %s\n", pth)
		if err := os.RemoveAll(pth); err != nil {
			return err
		}
		return fs.SkipDir
	})
}
