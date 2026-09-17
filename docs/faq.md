# Perguntas frequentes

## Como consultar os dados?

[Criando o seu banco de dados localmente](servidor.md), as consultas são feitas diretamente no PostgreSQL, juntando as tabelas descritas no [dicionário de dados](dicionario.md). Por exemplo:

* estabelecimentos ativos em uma UF, com a descrição da situação cadastral:

```sql
SELECT e.cnpj, em.razao_social, s.descricao AS situacao_cadastral
FROM estabelecimento e
JOIN empresa em ON em.cnpj_base = e.cnpj_base
JOIN situacao_cadastral s ON s.codigo = e.situacao_cadastral
WHERE e.uf = 'SP' AND e.situacao_cadastral = 2
```

* empresas com sócios em uma faixa etária:

```sql
SELECT DISTINCT em.cnpj_base, em.razao_social
FROM socio so
JOIN empresa em ON em.cnpj_base = so.cnpj_base
WHERE so.codigo_faixa_etaria = 5
```

As colunas mais usadas em filtros (`uf`, `cnae_fiscal`, `codigo_municipio`, `cnpj_base`, CNAEs secundários, CPF/CNPJ de sócios e natureza jurídica) já têm índices.

## Como consigo um CSV único dos dados?

Os dados oficiais da Receita Federal já são em CSV e talvez o mais fácil seja você utilizar os CSVs originais.

Caso queira gerar um CSV a partir dos dados consolidados, com ou sem filtragem de dados, o _Minha Receita_ pode ajudar. [Criando o seu banco de dados localmente](servidor.md), é possível utilizar o PostgreSQL para tal, como por exemplo:

```sql
COPY (
  SELECT
      e.cnpj,
      em.razao_social,
      e.nome_fantasia,
      m.descricao AS municipio
      -- adicione os campos que desejar
  FROM estabelecimento e
  JOIN empresa em ON em.cnpj_base = e.cnpj_base
  LEFT JOIN municipio m ON m.codigo = e.codigo_municipio
  -- WHERE e.uf = '…' caso queira filtrar, por exemplo
)
TO 'nome-do-arquivo.csv' DELIMITER ',' CSV HEADER;
```

## Como saber de quando são os dados carregados?

A data de divulgação pela Receita Federal do snapshot carregado fica na tabela `meta`:

```sql
SELECT value FROM meta WHERE key = 'updated-at';
```
