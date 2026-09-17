CREATE TABLE IF NOT EXISTS {{ .MetaTableFullName }} (
    {{ .KeyFieldName }} char(16) NOT NULL PRIMARY KEY,
    {{ .ValueFieldName }} text NOT NULL
);

-- Relational tables: the data from the Receita's files, loaded by the ETL and
-- kept up to date by the incremental update.
--
-- Notes:
--   * Table names are unqualified, i.e. created in the default (public) schema.
--     If a non-public --postgres-schema is used, these need qualification (which
--     requires exposing the schema to the SQL templates).
--   * Foreign keys are intentionally omitted: they would slow the parallel,
--     COPY-based bulk load and impose creation/insert ordering. Relationships
--     are enforced by the ETL; the indexes below support the joins and the
--     base -> establishments fan-out used by incremental updates.
--   * Only codes and literal values are stored. Descriptions live in the
--     reference tables below, joined by code.
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
    ordem int NOT NULL, -- position in the source list
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

-- Reference (lookup) tables from the Receita's auxiliary files: code -> description.
CREATE TABLE IF NOT EXISTS motivo            (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS pais              (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS cnae              (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS municipio         (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS natureza_juridica (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS qualificacao      (codigo int PRIMARY KEY, descricao text);
CREATE TABLE IF NOT EXISTS municipio_ibge    (codigo_municipio int PRIMARY KEY, codigo_ibge int);

-- Fixed descriptions defined by the Receita's file layout rather than by an
-- auxiliary file: situação cadastral, matriz/filial, porte and faixa etária.
CREATE TABLE IF NOT EXISTS situacao_cadastral          (codigo int PRIMARY KEY, descricao text NOT NULL);
CREATE TABLE IF NOT EXISTS identificador_matriz_filial (codigo int PRIMARY KEY, descricao text NOT NULL);
CREATE TABLE IF NOT EXISTS porte                       (codigo int PRIMARY KEY, descricao text NOT NULL);
CREATE TABLE IF NOT EXISTS faixa_etaria                (codigo int PRIMARY KEY, descricao text NOT NULL);
INSERT INTO situacao_cadastral (codigo, descricao) VALUES
    (1, 'NULA'), (2, 'ATIVA'), (3, 'SUSPENSA'), (4, 'INAPTA'), (8, 'BAIXADA')
    ON CONFLICT (codigo) DO NOTHING;
INSERT INTO identificador_matriz_filial (codigo, descricao) VALUES
    (1, 'MATRIZ'), (2, 'FILIAL')
    ON CONFLICT (codigo) DO NOTHING;
INSERT INTO porte (codigo, descricao) VALUES
    (0, 'NÃO INFORMADO'), (1, 'MICRO EMPRESA'), (3, 'EMPRESA DE PEQUENO PORTE'), (5, 'DEMAIS')
    ON CONFLICT (codigo) DO NOTHING;
INSERT INTO faixa_etaria (codigo, descricao) VALUES
    (0, 'Não se aplica'), (1, 'Entre 0 a 12 anos'), (2, 'Entre 13 a 20 ano'), (3, 'Entre 21 a 30 anos'),
    (4, 'Entre 31 a 40 anos'), (5, 'Entre 41 a 50 anos'), (6, 'Entre 51 a 60 anos'), (7, 'Entre 61 a 70 anos'),
    (8, 'Entre 71 a 80 anos'), (9, 'Maiores de 80 anos')
    ON CONFLICT (codigo) DO NOTHING;

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
