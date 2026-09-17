package transform

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/text/encoding/charmap"
)

// loadEstabelecimentos streams the Estabelecimentos CSVs, parses each row (with
// the key-value store, which also logs codes missing from the reference files
// for the report command) and writes the estabelecimento and secondary-CNAE
// relational tables.
func loadEstabelecimentos(ctx context.Context, srcs map[string]*source, kv *kv, db database, batch int, ext string, suffix string) error {
	bar, err := newProgressBar("[3/3] Loading establishments", 1)
	if err != nil {
		return fmt.Errorf("could not create a progress bar: %w", err)
	}
	defer func() {
		bar.AddMax(-1) // compensate for the extra byte added when creating the bar
	}()
	n := fmt.Sprintf("minha-receita-etl-%s.log", time.Now().Format("20060102150405"))
	f, err := os.Create(n)
	if err != nil {
		return fmt.Errorf("could not create log file %s: %w", n, err)
	}
	defer func() {
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
	}()
	log := slog.New(slog.NewJSONHandler(f, nil))
	src := newCompanySrc("Estabelecimentos", ';', false, false)
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	estabTable := "estabelecimento" + suffix
	cnaeTable := "estabelecimento_cnae" + suffix
	pths, err := os.ReadDir(ext)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", ext, err)
	}
	var producers errgroup.Group
	for _, pth := range pths {
		if !strings.HasPrefix(pth.Name(), src.prefix) || filepath.Ext(pth.Name()) != ".zip" {
			continue
		}
		p := pth
		producers.Go(func() error {
			pth := filepath.Join(ext, p.Name())
			a, err := zip.OpenReader(pth)
			if err != nil {
				return fmt.Errorf("could not open %s: %w", pth, err)
			}
			defer func() {
				if err := a.Close(); err != nil {
					slog.Warn("could not close archive", "path", pth, "error", err)
				}
			}()
			for _, z := range a.File {
				bar.AddMax64(int64(z.UncompressedSize64))
			}
			var g errgroup.Group
			for _, z := range a.File {
				if z.FileInfo().IsDir() {
					continue
				}
				g.Go(func() error {
					f, err := z.Open()
					if err != nil {
						return fmt.Errorf("could not read %s from %s: %w", z.Name, pth, err)
					}
					defer func() {
						if err := f.Close(); err != nil {
							slog.Warn("Could not close csv reader", "path", pth, "name", z.Name, "error", err)
						}
					}()
					b := countReader{f, 0}
					r := csv.NewReader(charmap.ISO8859_15.NewDecoder().Reader(&b))
					r.Comma = src.sep
					var prev int64
					var estabBuf, cnaeBuf [][]any
					flushRel := func() error {
						if len(estabBuf) > 0 {
							if err := db.CopyRelational(ctx, estabTable, estabelecimentoColumns, estabBuf); err != nil {
								return err
							}
							estabBuf = estabBuf[:0]
						}
						if len(cnaeBuf) > 0 {
							if err := db.CopyRelational(ctx, cnaeTable, estabelecimentoCnaeColumns, cnaeBuf); err != nil {
								return err
							}
							cnaeBuf = cnaeBuf[:0]
						}
						return nil
					}
					for {
						select {
						case <-ctx.Done():
							return context.Cause(ctx)
						default:
							row, err := r.Read()
							if err != nil {
								if errors.Is(err, io.EOF) {
									return flushRel()
								}
								return fmt.Errorf("error reading %s: %w", pth, err)
							}
							if len(row) < 2 {
								return fmt.Errorf("unexpected row with %d columns in %s", len(row), src.prefix)
							}
							for n := range row {
								row[n] = cleanupColumn(row[n])
							}
							c, err := newCompany(log, srcs, kv, row)
							if err != nil {
								return fmt.Errorf("could not create company %v: %w", row[:3], err)
							}
							estabBuf = append(estabBuf, estabelecimentoRow(c, row))
							cnaeBuf = append(cnaeBuf, estabelecimentoCnaeRows(c)...)
							if len(estabBuf) >= batch {
								if err := flushRel(); err != nil {
									return err
								}
							}
							s := b.read - prev
							if s > 0 {
								if err := bar.Add64(s); err != nil {
									slog.Warn("could not update the progress bar", "error", err)
								}
							}
							prev = b.read
						}
					}
				})
			}
			if err := g.Wait(); err != nil {
				return err
			}
			if e := os.Remove(pth); e != nil {
				slog.Warn("could not remove", "path", pth, "error", e)
			}
			return nil
		})
	}
	return producers.Wait()
}
