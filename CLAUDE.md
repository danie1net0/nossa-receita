# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

"Minha Receita" — a Go CLI + web API that serves Brazilian CNPJ (company registry) data published monthly by the Receita Federal. The module is `codeberg.org/cuducos/minha-receita`; the binary is `minha-receita`. The working directory (`nossa-receita`) is a fork — the module path, binary name, and docs all use `minha-receita`.

Canonical host is **Codeberg**, not GitHub. CI runs on **Woodpecker** (`.woodpecker/*.yaml`). User-facing docs live in `docs/` (Portuguese) and are built with `zensical`.

## Critical: GOEXPERIMENT=jsonv2

The code imports `encoding/json/v2` (e.g. `api/graph.go`, `transform/company.go`, `transform/cast.go`). **Every `go` invocation — build, test, vet, run — must have `GOEXPERIMENT=jsonv2` set**, or compilation fails. Requires Go 1.26. The Dockerfile and all CI steps set this; mirror it locally.

## Commands

```console
# Build
GOEXPERIMENT=jsonv2 go build -o minha-receita main.go

# Format check (CI fails if this prints any file)
gofmt -s -l .

# Lint (CI pins golangci-lint v2.10.1)
golangci-lint run ./...

# Full test suite (race detector, as in CI)
GOEXPERIMENT=jsonv2 go test --race ./...

# A single package / test
GOEXPERIMENT=jsonv2 go test ./transform
GOEXPERIMENT=jsonv2 go test -run TestName ./db
```

### Test databases

`db/` tests **error out (not skip)** unless `TEST_POSTGRES_URL` points at a live instance. Spin up the non-persistent test database via Docker Compose, then export the URL from `.env.sample`:

```console
docker compose up -d postgres_test
```

## CLI structure

The CLI is Cobra-based; all commands are wired in `cmd/cmd.go` → `CLI()`. Each subcommand's file under `cmd/` builds flags and calls into a feature package. The key abstraction is the `database` interface in `cmd/db.go`: `loadDatabase` requires a PostgreSQL URI (`postgres://`/`postgresql://`); `DATABASE_URL` is the default source. PostgreSQL is the only backend.

Subcommands: `api`, `graph`, `create`/`drop`, `extra-indexes`, `transform`, `clean-up`, `provision USER@IP`, `report <log>`, `sample`.

## Architecture

Three primary packages, each implementing or consuming the shared `database` interface:

- **`transform/`** — the ETL, entry point `transform.Transform`. Source ZIPs (`YYYY-MM.zip` + `entidades-*.zip`) must be placed **manually** in the data directory (`-d`, default `data`); the tool does not download them. The IBGE municipalities CSV is the one file fetched automatically at runtime. Runs in three stages:
  1. Unzip `YYYY-MM.zip` into a temp dir.
  2. Load all lookup/relational CSVs (`Empresas*`, `Socios*`, `Simples`, `Cnaes`, `Municipios`, tax-regime files, etc.) into an **on-disk Badger key-value store** (`transform/badger.go`), in parallel via `errgroup`.
  3. Stream `Estabelecimentos*` rows, enrich each from Badger, and write **one JSON record per full CNPJ** to the database in batches.

- **`db/`** — the PostgreSQL backend implementing the `database` interface (the sole backend). Its SQL lives in `db/postgres/*.sql`, embedded via `//go:embed postgres` and rendered as Go templates (`db/postgres.go`). When changing schema/queries, edit the `.sql` files, not inline strings.

- **`api/`** — two independent HTTP servers:
  - `api.Serve` (`api/api.go`): single-company lookup (`/`) + paginated search. Wrapped by a Ristretto **cache** and a **bloom filter** (`bloom/`) that short-circuits 404s before hitting the DB. Exposes `/metrics` (Prometheus), `/healthz`, `/updated`.
  - `api.ServeGraph` (`api/graph.go`): partnership graph — `/relacoes/<id>` and `/conexao/<id>/<id>` (shortest path via **bidirectional BFS** over `GetRelated`).

Supporting packages: `bloom/`, `report/` (FOI report from ETL logs), `sample/` (shrunken source files for quick local testing), `testutils/`.

## Privacy

PII handling is a deliberate, load-bearing behavior — don't regress it:
- `transform` strips PII (emails, CPF, etc.) by default; `--no-privacy` includes it in the stored JSON.
- The graph API zeroes `PartnerCPF` on every edge unless `PartnerType == 2` (`api/graph.go`).
