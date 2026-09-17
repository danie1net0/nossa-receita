DROP TABLE IF EXISTS {{ .MetaTableFullName }} CASCADE;

-- Relational source-of-truth and reference tables (migration phase 2).
DROP TABLE IF EXISTS empresa CASCADE;
DROP TABLE IF EXISTS estabelecimento CASCADE;
DROP TABLE IF EXISTS simples CASCADE;
DROP TABLE IF EXISTS socio CASCADE;
DROP TABLE IF EXISTS estabelecimento_cnae CASCADE;
DROP TABLE IF EXISTS regime_tributario CASCADE;
DROP TABLE IF EXISTS motivo CASCADE;
DROP TABLE IF EXISTS pais CASCADE;
DROP TABLE IF EXISTS cnae CASCADE;
DROP TABLE IF EXISTS municipio CASCADE;
DROP TABLE IF EXISTS natureza_juridica CASCADE;
DROP TABLE IF EXISTS qualificacao CASCADE;
DROP TABLE IF EXISTS municipio_ibge CASCADE;
DROP TABLE IF EXISTS situacao_cadastral CASCADE;
DROP TABLE IF EXISTS identificador_matriz_filial CASCADE;
DROP TABLE IF EXISTS porte CASCADE;
DROP TABLE IF EXISTS faixa_etaria CASCADE;

-- The cnpj JSONB document table of older versions.
DROP TABLE IF EXISTS cnpj CASCADE;

-- Staging tables (migration phase 4).
DROP TABLE IF EXISTS empresa_staging CASCADE;
DROP TABLE IF EXISTS estabelecimento_staging CASCADE;
DROP TABLE IF EXISTS simples_staging CASCADE;
DROP TABLE IF EXISTS socio_staging CASCADE;
DROP TABLE IF EXISTS estabelecimento_cnae_staging CASCADE;
DROP TABLE IF EXISTS regime_tributario_staging CASCADE;
DROP TABLE IF EXISTS motivo_staging CASCADE;
DROP TABLE IF EXISTS pais_staging CASCADE;
DROP TABLE IF EXISTS cnae_staging CASCADE;
DROP TABLE IF EXISTS municipio_staging CASCADE;
DROP TABLE IF EXISTS natureza_juridica_staging CASCADE;
DROP TABLE IF EXISTS qualificacao_staging CASCADE;
DROP TABLE IF EXISTS municipio_ibge_staging CASCADE;
