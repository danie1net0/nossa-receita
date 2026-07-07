{{ if not .Logged }}
SET maintenance_work_mem TO '512MB';

-- Rebuild the secondary indexes dropped by pre_load, now that the tables are
-- fully loaded: one sort per index instead of per-row maintenance during the
-- bulk COPY. Built while the tables are still UNLOGGED (no WAL), then made
-- durable together with the heap by the SET LOGGED below.
-- Keep this list in sync with create.sql/pre_load.sql.
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnpj_base   ON estabelecimento (cnpj_base);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnae_fiscal ON estabelecimento (cnae_fiscal);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_uf          ON estabelecimento (uf);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_municipio   ON estabelecimento (codigo_municipio);
CREATE INDEX IF NOT EXISTS idx_socio_cnpj_base             ON socio (cnpj_base);
CREATE INDEX IF NOT EXISTS idx_socio_cnpj_cpf_do_socio     ON socio (cnpj_cpf_do_socio);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnae_cnae   ON estabelecimento_cnae (cnae);
CREATE INDEX IF NOT EXISTS idx_regime_tributario_cnpj      ON regime_tributario (cnpj);
CREATE INDEX IF NOT EXISTS idx_empresa_natureza            ON empresa (codigo_natureza_juridica);

ALTER TABLE {{ .CompanyTableFullName }} SET LOGGED;
ALTER TABLE empresa SET LOGGED;
ALTER TABLE estabelecimento SET LOGGED;
ALTER TABLE simples SET LOGGED;
ALTER TABLE socio SET LOGGED;
ALTER TABLE estabelecimento_cnae SET LOGGED;
ALTER TABLE regime_tributario SET LOGGED;
{{ end }}
