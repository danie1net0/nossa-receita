package transform

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/avast/retry-go/v4"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

// The Receita Federal serves the CNPJ open data as two public Nextcloud shares
// (reachable over WebDAV): the monthly CNPJ files, partitioned by YYYY-MM, and
// the tax-regime files (entidades-*.zip), which sit at the share root. The share
// tokens rotate occasionally — override the defaults with the environment
// variables below (paste the .../index.php/s/<token> URL from a browser).
const (
	cnpjShareEnv   = "RECEITA_CNPJ_SHARE_URL"
	regimeShareEnv = "RECEITA_REGIME_SHARE_URL"

	defaultCNPJShareURL   = "https://arquivos.receitafederal.gov.br/index.php/s/YggdBLfdninEJX9"
	defaultRegimeShareURL = "https://arquivos.receitafederal.gov.br/index.php/s/MPPfFit7g7zdA8C"

	downloadRetries = 5
)

var yearMonth = regexp.MustCompile(`^\d{4}-\d{2}$`)

// share is a public Nextcloud share accessed over its public WebDAV endpoint,
// using the share token as the HTTP basic-auth username.
type share struct {
	base  string
	token string
}

func newShare(shareURL string) (*share, error) {
	u, err := url.Parse(shareURL)
	if err != nil {
		return nil, fmt.Errorf("invalid share URL %q: %w", shareURL, err)
	}
	p := strings.TrimRight(u.Path, "/")
	token := p[strings.LastIndex(p, "/")+1:]
	if u.Host == "" || token == "" || token == "s" {
		return nil, fmt.Errorf("could not extract a share token from %q (expected .../index.php/s/<token>)", shareURL)
	}
	return &share{base: u.Scheme + "://" + u.Host + "/public.php/webdav", token: token}, nil
}

func shareURL(env, def string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	return def
}

type davEntry struct {
	name  string
	isDir bool
	size  int64
}

// propfind lists the immediate children of path within the share.
func (s *share) propfind(ctx context.Context, path string) ([]davEntry, error) {
	u := s.base + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.token, "")
	req.Header.Set("Depth", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("could not close PROPFIND response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND %s returned %s", path, resp.Status)
	}
	var ms struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				ContentLength int64     `xml:"prop>getcontentlength"`
				Collection    *struct{} `xml:"prop>resourcetype>collection"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("could not parse PROPFIND response for %s: %w", path, err)
	}
	self := strings.TrimRight(path, "/")
	var out []davEntry
	for _, r := range ms.Responses {
		href := strings.TrimRight(r.Href, "/")
		name := href[strings.LastIndex(href, "/")+1:]
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		if name == "" || name == self {
			continue // the collection itself
		}
		var e davEntry
		e.name = name
		for _, ps := range r.Propstat {
			if ps.Collection != nil {
				e.isDir = true
			}
			if ps.ContentLength > 0 {
				e.size = ps.ContentLength
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// latestMonth returns the most recent YYYY-MM directory in the share.
func (s *share) latestMonth(ctx context.Context) (string, error) {
	entries, err := s.propfind(ctx, "")
	if err != nil {
		return "", err
	}
	var months []string
	for _, e := range entries {
		if e.isDir && yearMonth.MatchString(e.name) {
			months = append(months, e.name)
		}
	}
	if len(months) == 0 {
		return "", fmt.Errorf("no YYYY-MM directories found in the share")
	}
	sort.Strings(months)
	return months[len(months)-1], nil
}

// downloadFile downloads remotePath into dest, resuming is not supported but a
// matching-size file is skipped, and the write is atomic (via a .part file).
func (s *share) downloadFile(ctx context.Context, remotePath, dest string, size int64, bar *progressbar.ProgressBar) error {
	if fi, err := os.Stat(dest); err == nil && size > 0 && fi.Size() == size {
		slog.Info("already downloaded, skipping", "file", filepath.Base(dest))
		if bar != nil {
			_ = bar.Add64(size)
		}
		return nil
	}
	return retry.Do(
		func() error {
			u := s.base + "/" + strings.TrimLeft(remotePath, "/")
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth(s.token, "")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					slog.Warn("could not close download response body", "error", err)
				}
			}()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("GET %s returned %s", remotePath, resp.Status)
			}
			tmp := dest + ".part"
			f, err := os.Create(tmp)
			if err != nil {
				return err
			}
			w := io.Writer(f)
			if bar != nil {
				w = io.MultiWriter(f, bar)
			}
			if _, err := io.Copy(w, resp.Body); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			return os.Rename(tmp, dest)
		},
		retry.Attempts(downloadRetries),
		retry.Context(ctx),
		retry.OnRetry(func(n uint, err error) {
			slog.Warn("retrying download", "file", filepath.Base(dest), "attempt", n+1, "error", err)
		}),
	)
}

// Download fetches a monthly CNPJ snapshot into dir, ready for transform/update:
// the CNPJ files are bundled into dir/YYYY-MM.zip (matching the layout the ETL
// expects) and the tax-regime entidades-*.zip files are placed next to it.
func Download(dir, month string, parallel int, skipMain, skipRegime, listOnly bool) error {
	ctx := context.Background()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", dir, err)
	}
	if parallel < 1 {
		parallel = 1
	}

	if !skipMain {
		cnpj, err := newShare(shareURL(cnpjShareEnv, defaultCNPJShareURL))
		if err != nil {
			return err
		}
		if month == "" {
			if month, err = cnpj.latestMonth(ctx); err != nil {
				return fmt.Errorf("could not discover the latest month: %w", err)
			}
		}
		files, err := cnpj.propfind(ctx, month)
		if err != nil {
			return fmt.Errorf("could not list %s: %w", month, err)
		}
		files = onlyZips(files)
		slog.Info("CNPJ snapshot", "month", month, "files", len(files), "total", humanTotal(files))
		if listOnly {
			printList(month+"/", files)
		} else if err := downloadCNPJ(ctx, cnpj, month, files, dir, parallel); err != nil {
			return err
		}
	}

	if !skipRegime {
		regime, err := newShare(shareURL(regimeShareEnv, defaultRegimeShareURL))
		if err != nil {
			return err
		}
		entries, err := regime.propfind(ctx, "")
		if err != nil {
			return fmt.Errorf("could not list tax-regime files: %w", err)
		}
		var files []davEntry
		for _, e := range onlyZips(entries) {
			if strings.HasPrefix(e.name, "entidades-") {
				files = append(files, e)
			}
		}
		slog.Info("tax-regime files", "files", len(files), "total", humanTotal(files))
		if listOnly {
			printList("", files)
		} else if err := downloadRegime(ctx, regime, files, dir, parallel); err != nil {
			return err
		}
	}
	return nil
}

func downloadCNPJ(ctx context.Context, s *share, month string, files []davEntry, dir string, parallel int) error {
	dest := filepath.Join(dir, month+".zip")
	if _, err := os.Stat(dest); err == nil {
		slog.Info("snapshot bundle already present, skipping CNPJ download", "file", dest)
		return nil
	}
	tmp, err := os.MkdirTemp(dir, "."+month+"-download-*")
	if err != nil {
		return fmt.Errorf("could not create download workspace: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("could not remove download workspace", "path", tmp, "error", err)
		}
	}()
	bar := progressbar.DefaultBytes(totalSize(files), "downloading CNPJ")
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallel)
	for _, f := range files {
		g.Go(func() error {
			return s.downloadFile(gctx, month+"/"+f.name, filepath.Join(tmp, f.name), f.size, bar)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return bundle(files, tmp, dest)
}

func downloadRegime(ctx context.Context, s *share, files []davEntry, dir string, parallel int) error {
	bar := progressbar.DefaultBytes(totalSize(files), "downloading regime")
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallel)
	for _, f := range files {
		g.Go(func() error {
			return s.downloadFile(gctx, f.name, filepath.Join(dir, f.name), f.size, bar)
		})
	}
	return g.Wait()
}

// bundle packs the already-downloaded files (stored, not recompressed — they are
// already ZIPs) into dest, matching the single YYYY-MM.zip the ETL expects.
func bundle(files []davEntry, srcDir, dest string) error {
	slog.Info("bundling CNPJ files", "into", dest)
	out, err := os.Create(dest + ".part")
	if err != nil {
		return fmt.Errorf("could not create bundle: %w", err)
	}
	zw := zip.NewWriter(out)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Store})
		if err != nil {
			_ = out.Close()
			return err
		}
		in, err := os.Open(filepath.Join(srcDir, f.name))
		if err != nil {
			_ = out.Close()
			return err
		}
		if _, err := io.Copy(w, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			return err
		}
		if err := in.Close(); err != nil {
			_ = out.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(dest+".part", dest)
}

func onlyZips(entries []davEntry) []davEntry {
	var out []davEntry
	for _, e := range entries {
		if !e.isDir && strings.HasSuffix(strings.ToLower(e.name), ".zip") {
			out = append(out, e)
		}
	}
	return out
}

func totalSize(files []davEntry) int64 {
	var t int64
	for _, f := range files {
		t += f.size
	}
	return t
}

func humanTotal(files []davEntry) string {
	return fmt.Sprintf("%.1f MB", float64(totalSize(files))/(1<<20))
}

func printList(prefix string, files []davEntry) {
	for _, f := range files {
		fmt.Printf("  %s%s\t%.1f MB\n", prefix, f.name, float64(f.size)/(1<<20))
	}
}
