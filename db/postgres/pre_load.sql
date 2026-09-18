{{ if not .Logged }}
SET maintenance_work_mem TO '512MB';

-- Each statement runs on its own (see execStatements), so every table rewrite
-- frees its old copy before the next one starts.
--
-- Drop the secondary indexes so the bulk COPY doesn't maintain a B-tree per row
-- (the estabelecimento table alone carries four). post_load rebuilds them once,
-- with a single sort per index, after the data is in. Dropping them before SET
-- UNLOGGED also spares that rewrite from copying them.
-- Keep this list in sync with the CREATE INDEX statements in create.sql/post_load.sql.
DROP INDEX IF EXISTS idx_estabelecimento_cnpj_base;
DROP INDEX IF EXISTS idx_estabelecimento_cnae_fiscal;
DROP INDEX IF EXISTS idx_estabelecimento_uf;
DROP INDEX IF EXISTS idx_estabelecimento_municipio;
DROP INDEX IF EXISTS idx_socio_cnpj_base;
DROP INDEX IF EXISTS idx_socio_cnpj_cpf_do_socio;
DROP INDEX IF EXISTS idx_estabelecimento_cnae_cnae;
DROP INDEX IF EXISTS idx_regime_tributario_cnpj;
DROP INDEX IF EXISTS idx_empresa_natureza;

-- The primary keys of the big tables go too: checking each COPY'd row against a
-- growing B-tree was what capped the full load at a few MB/s. post_load adds
-- them back, which also rejects any duplicate key the source may carry.
-- Keep this list in sync with create.sql/post_load.sql.
ALTER TABLE empresa DROP CONSTRAINT IF EXISTS empresa_pkey;
ALTER TABLE estabelecimento DROP CONSTRAINT IF EXISTS estabelecimento_pkey;
ALTER TABLE simples DROP CONSTRAINT IF EXISTS simples_pkey;
ALTER TABLE socio DROP CONSTRAINT IF EXISTS socio_pkey;
ALTER TABLE estabelecimento_cnae DROP CONSTRAINT IF EXISTS estabelecimento_cnae_pkey;
ALTER TABLE regime_tributario DROP CONSTRAINT IF EXISTS regime_tributario_pkey;

-- Unlogged during the bulk load: no WAL, no fsync per COPY. post_load restores
-- durability with SET LOGGED.
ALTER TABLE empresa SET UNLOGGED;
ALTER TABLE estabelecimento SET UNLOGGED;
ALTER TABLE simples SET UNLOGGED;
ALTER TABLE socio SET UNLOGGED;
ALTER TABLE estabelecimento_cnae SET UNLOGGED;
ALTER TABLE regime_tributario SET UNLOGGED;

RESET maintenance_work_mem;
{{ end }}
