CREATE TABLE IF NOT EXISTS {{ .CompanyTableFullName }} (
    {{ .CursorFieldName }} SERIAL PRIMARY KEY,
    {{ .IDFieldName }} char(14) NOT NULL,
    {{ .JSONFieldName }} jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS {{ .MetaTableFullName }} (
    {{ .KeyFieldName }} char(16) NOT NULL PRIMARY KEY,
    {{ .ValueFieldName }} text NOT NULL
);
CREATE UNIQUE INDEX {{ .CompanyTableName }}_id ON {{ .CompanyTableFullName }} ({{ .IDFieldName }});

-- Relational source-of-truth tables (migration phase 2). These are populated
-- by the ETL alongside the cnpj JSONB document (dual-write); the document
-- remains the source for reads until materialization is in place.
--
-- Notes:
--   * Table names are unqualified, i.e. created in the default (public) schema.
--     If a non-public --postgres-schema is used, these need qualification (which
--     requires exposing the schema to the SQL templates).
--   * Foreign keys are intentionally omitted: they would slow the parallel,
--     COPY-based bulk load and impose creation/insert ordering. Relationships
--     are enforced by the ETL; the indexes below support the joins and the
--     base -> establishments fan-out used by incremental updates.
--   * Only codes and literal values are stored. Descriptions are resolved from
--     the reference tables (or derived in code) at materialization time.
--   * `hash` is xxhash64 of the normalized source record; it drives the cheap
--     diff in the incremental-update phase. Present only on the tables with a
--     1:1 natural key; child collections (socio, regime_tributario,
--     estabelecimento_cnae) are diffed as a set per parent.

CREATE TABLE IF NOT EXISTS empresa (
    cnpj_base                   char(8) NOT NULL PRIMARY KEY,
    razao_social                text,
    codigo_natureza_juridica    int,
    qualificacao_do_responsavel int,
    capital_social              double precision,
    codigo_porte                int,
    ente_federativo_responsavel text,
    hash                        bytea NOT NULL
);

CREATE TABLE IF NOT EXISTS estabelecimento (
    cnpj                         char(14) NOT NULL PRIMARY KEY,
    cnpj_base                    char(8) NOT NULL,
    identificador_matriz_filial  int,
    nome_fantasia                text,
    situacao_cadastral           int,
    data_situacao_cadastral      date,
    motivo_situacao_cadastral    int,
    nome_cidade_no_exterior      text,
    codigo_pais                  int,
    data_inicio_atividade        date,
    cnae_fiscal                  int,
    descricao_tipo_de_logradouro text,
    logradouro                   text,
    numero                       text,
    complemento                  text,
    bairro                       text,
    cep                          text,
    uf                           char(2),
    codigo_municipio             int,
    ddd_telefone_1               text,
    ddd_telefone_2               text,
    ddd_fax                      text,
    email                        text,
    situacao_especial            text,
    data_situacao_especial       date,
    hash                         bytea NOT NULL
);

CREATE TABLE IF NOT EXISTS simples (
    cnpj_base                char(8) NOT NULL PRIMARY KEY,
    opcao_pelo_simples       boolean,
    data_opcao_pelo_simples  date,
    data_exclusao_do_simples date,
    opcao_pelo_mei           boolean,
    data_opcao_pelo_mei      date,
    data_exclusao_do_mei     date,
    hash                     bytea NOT NULL
);

CREATE TABLE IF NOT EXISTS socio (
    id                                      bigserial PRIMARY KEY,
    cnpj_base                               char(8) NOT NULL,
    identificador_de_socio                  int,
    nome_socio                              text,
    cnpj_cpf_do_socio                       text,
    codigo_qualificacao_socio               int,
    data_entrada_sociedade                  date,
    codigo_pais                             int,
    cpf_representante_legal                 text,
    nome_representante_legal                text,
    codigo_qualificacao_representante_legal int,
    codigo_faixa_etaria                     int
);

CREATE TABLE IF NOT EXISTS estabelecimento_cnae (
    cnpj  char(14) NOT NULL,
    cnae  int NOT NULL,
    ordem int NOT NULL, -- position in the source list, so materialization preserves order
    PRIMARY KEY (cnpj, cnae)
);

CREATE TABLE IF NOT EXISTS regime_tributario (
    id                          bigserial PRIMARY KEY,
    cnpj                        char(14) NOT NULL,
    tipo                        text,
    ano                         int,
    cnpj_da_scp                 text,
    forma_de_tributacao         text,
    quantidade_de_escrituracoes int
);

-- Reference (lookup) tables: code -> description, resolved at materialization.
CREATE TABLE IF NOT EXISTS motivo            (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS pais              (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS cnae              (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS municipio         (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS natureza_juridica (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS qualificacao      (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS municipio_ibge    (codigo_municipio int PRIMARY KEY, codigo_ibge int);

-- Indexes for filtering (socios, CNAEs, UF) and the base -> establishment fan-out.
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnpj_base    ON estabelecimento (cnpj_base);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnae_fiscal  ON estabelecimento (cnae_fiscal);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_uf           ON estabelecimento (uf);
CREATE INDEX IF NOT EXISTS idx_socio_cnpj_base              ON socio (cnpj_base);
CREATE INDEX IF NOT EXISTS idx_socio_cnpj_cpf_do_socio      ON socio (cnpj_cpf_do_socio);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_cnae_cnae    ON estabelecimento_cnae (cnae);
CREATE INDEX IF NOT EXISTS idx_regime_tributario_cnpj       ON regime_tributario (cnpj);
CREATE INDEX IF NOT EXISTS idx_estabelecimento_municipio    ON estabelecimento (codigo_municipio);
CREATE INDEX IF NOT EXISTS idx_empresa_natureza             ON empresa (codigo_natureza_juridica);
CREATE INDEX IF NOT EXISTS idx_municipio_ibge_codigo_ibge   ON municipio_ibge (codigo_ibge);
