-- UNLOGGED staging tables for incremental updates (migration phase 4). They are
-- loaded with the new monthly snapshot, then diffed against the live tables to
-- compute which documents must be rebuilt. They mirror the entity tables but
-- drop the surrogate ids of the child collections (socio, regime_tributario),
-- which are not part of the data being compared.

CREATE UNLOGGED TABLE IF NOT EXISTS empresa_staging (
    cnpj_base                   char(8) NOT NULL,
    razao_social                text,
    codigo_natureza_juridica    int,
    qualificacao_do_responsavel int,
    capital_social              double precision,
    codigo_porte                int,
    ente_federativo_responsavel text,
    hash                        bytea NOT NULL
);

CREATE UNLOGGED TABLE IF NOT EXISTS estabelecimento_staging (
    cnpj                         char(14) NOT NULL,
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

CREATE UNLOGGED TABLE IF NOT EXISTS simples_staging (
    cnpj_base                char(8) NOT NULL,
    opcao_pelo_simples       boolean,
    data_opcao_pelo_simples  date,
    data_exclusao_do_simples date,
    opcao_pelo_mei           boolean,
    data_opcao_pelo_mei      date,
    data_exclusao_do_mei     date,
    hash                     bytea NOT NULL
);

CREATE UNLOGGED TABLE IF NOT EXISTS socio_staging (
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

CREATE UNLOGGED TABLE IF NOT EXISTS estabelecimento_cnae_staging (
    cnpj  char(14) NOT NULL,
    cnae  int NOT NULL,
    ordem int NOT NULL
);

CREATE UNLOGGED TABLE IF NOT EXISTS regime_tributario_staging (
    cnpj                        char(14) NOT NULL,
    tipo                        text,
    ano                         int,
    cnpj_da_scp                 text,
    forma_de_tributacao         text,
    quantidade_de_escrituracoes int
);

TRUNCATE empresa_staging, estabelecimento_staging, simples_staging, socio_staging, estabelecimento_cnae_staging, regime_tributario_staging;
