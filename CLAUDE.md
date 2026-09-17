# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

"Minha Receita" — a Go CLI that loads Brazilian CNPJ (company registry) data published monthly by the Receita Federal into PostgreSQL and keeps it updated incrementally. The fork has no HTTP API: the data is consumed straight from the database. The module is `codeberg.org/cuducos/minha-receita`; the binary is `minha-receita`. The working directory (`nossa-receita`) is a fork — the module path, binary name, and docs all use `minha-receita`.

Canonical host is **Codeberg**, not GitHub. CI runs on **Woodpecker** (`.woodpecker/*.yaml`). User-facing docs live in `docs/` (Portuguese) and are built with `zensical`.

## Critical: GOEXPERIMENT=jsonv2

The code imports `encoding/json/v2` (e.g. `transform/ibge.go`). **Every `go` invocation — build, test, vet, run — must have `GOEXPERIMENT=jsonv2` set**, or compilation fails. Requires Go 1.26. The Dockerfile and all CI steps set this; mirror it locally.

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

Subcommands: `create`/`drop`, `download`, `transform` (full load), `update` (incremental), `clean-up`, `provision USER@IP`, `report <log>`, `sample`.

## Architecture

Two primary packages, each implementing or consuming the shared `database` interface:

- **`transform/`** — the ETL, entry point `transform.Transform` (full load). Source ZIPs (`YYYY-MM.zip` + `entidades-*.zip`) go in the data directory (`-d`, default `data`); the `download` command (`transform/download.go`) fetches them from the Receita's two public Nextcloud/WebDAV shares (tokens overridable via `RECEITA_CNPJ_SHARE_URL`/`RECEITA_REGIME_SHARE_URL`), bundling the monthly CNPJ files into `YYYY-MM.zip`; the server sometimes stalls mid-transfer, so a download with no data for `downloadStallTimeout` is aborted and retried. The IBGE municipalities CSV is fetched automatically at ETL runtime. The shared `loadSnapshot` runs in three stages:
  1. Unzip `YYYY-MM.zip` into a temp dir.
  2. Load all lookup/relational CSVs (`Empresas*`, `Socios*`, `Simples`, `Cnaes`, `Municipios`, tax-regime files, etc.) into an **on-disk Badger key-value store** (`transform/badger.go`), in parallel via `errgroup`.
  3. Write the relational tables: `loadRelational` scans Badger into `empresa`, `simples`, `socio`, `regime_tributario` and the reference tables, while `loadEstabelecimentos` (`transform/writer.go`) streams `Estabelecimentos*` rows through `parseEstabelecimento` (`transform/estabelecimento.go`) into `estabelecimento` and `estabelecimento_cnae` (see `transform/relational.go`). Stage 3 does not read Badger per establishment: the small reference sources (`mot`, `pai`, `mun`, `tab`) keep an in-memory copy while stage 2 loads them (`source.values`), used to log codes missing from the reference files. Partners with unknown country codes are collected during `loadRelational` and logged per establishment at the end (`logUnknownPartnerCountries`, via `CNPJsByBase`). Those warnings go to `minha-receita-etl-*.log`, which the `report` command reads — keep the message texts (`unknown MotivoSituacaoCadastral`, `unknown CodigoPais`, `unknown Pais`) stable.

- **`db/`** — the PostgreSQL backend (the sole backend). Embedded DDL lives in `db/postgres/*.sql` (`//go:embed postgres`, rendered as Go templates in `db/postgres.go`); the incremental apply in `db/delta.go` is inline SQL. The schema holds the relational tables, the reference tables (including fixed descriptions seeded by `create.sql`: situação cadastral, matriz/filial, porte, faixa etária) and the `*_staging` tables. There is no JSON document table any more (older databases may still have a `cnpj` table; `drop` removes it). When changing the embedded DDL, edit the `.sql` files, not inline strings.

Supporting packages: `report/` (FOI report from ETL logs), `sample/` (shrunken source files for quick local testing).

## Relational model & incremental updates

The relational tables are the data; there is no derived document. **Incremental update** (`transform/update.go` + `db/delta.go`): `update` loads the new snapshot into UNLOGGED `*_staging` tables (reference tables included) and `ApplyDelta` makes the live tables equal to staging touching only the differences — hash anti-join upsert/prune for 1:1 entities, per-parent set replacement for child collections (`socio`, `estabelecimento_cnae`, `regime_tributario`), value upsert/prune for reference tables. **Invariant** (the acceptance test): applying month N+1 incrementally yields a database identical to a full load of N+1.

## Personal data

The tables store the data exactly as the Receita publishes it, including personal data (emails, partner names, CPFs in trade names of sole proprietors). There is no privacy filter any more: it only applied to the removed JSON documents.
