# Dicionário de dados

Os dados ficam em tabelas relacionais no PostgreSQL. As tabelas guardam os códigos e os valores como publicados pela Receita Federal; as descrições dos códigos ficam em tabelas de referência, ligadas pelo código. Algumas conversões são feitas na carga:

* Campos _booleanos_: textos como `"S"` e `"N"` viram `true`, `false` ou `null` (em branco)
* Datas viram o tipo `date` do PostgreSQL
* Telefones e fax são a concatenação do DDD com o número
* Os CNAEs secundários, que a Receita publica numa lista separada por vírgulas, viram linhas em `estabelecimento_cnae` (um código repetido na mesma lista é gravado uma vez)

Salvo as chaves (`cnpj`, `cnpj_base`), todas as colunas podem estar em branco.

!!! warning "Aviso"
    A Receita Federal, em outubro de 2025, passou a divulgar diversos códigos de países que não constam no `Paises.zip`. Seguindo outros documentos da própria Receita Federal ([exemplo](https://balanca.economia.gov.br/balanca/bd/tabelas/PAIS.csv)) o Minha Receita adiciona esses códigos na tabela `pais`: 15 (Aland, Ilhas), 150 (Ilhas do Guernsey), 151 (Canárias, Ilhas), 200 (Curaçao), 321 (Guernsey), 359 (Ilha de Man), 367 (Inglaterra), 393 (Jersey), 449 (Macedônia), 452 (Madeira, Ilha da), 498 (Montenegro), 578 (Palestina), 678 (Saint Kitts e Nevis), 699 (Sint Maarten), 737 (Sérvia) e 994 (A Designar).

## `empresa`

Uma linha por CNPJ básico (os 8 primeiros dígitos), de `Empresas*.zip`.

| Coluna | Tipo | Nome no _layout_ da Receita Federal | Descrição em |
|---|---|---|---|
| `cnpj_base` | `char(8)` | CNPJ básico | |
| `razao_social` | `text` | Razão social / Nome empresarial | |
| `codigo_natureza_juridica` | `int` | Natureza jurídica | `natureza_juridica` |
| `qualificacao_do_responsavel` | `int` | Qualificação do responsável | `qualificacao` |
| `capital_social` | `double precision` | Capital social da empresa | |
| `codigo_porte` | `int` | Porte da empresa | `porte` |
| `ente_federativo_responsavel` | `text` | Ente federativo responsável | |

## `estabelecimento`

Uma linha por CNPJ completo, de `Estabelecimentos*.zip`.

| Coluna | Tipo | Nome no _layout_ da Receita Federal | Descrição em |
|---|---|---|---|
| `cnpj` | `char(14)` | CNPJ básico, ordem e DV concatenados | |
| `cnpj_base` | `char(8)` | CNPJ básico | liga com `empresa`, `simples` e `socio` |
| `identificador_matriz_filial` | `int` | Identificador matriz/filial | `identificador_matriz_filial` |
| `nome_fantasia` | `text` | Nome fantasia | |
| `situacao_cadastral` | `int` | Situação cadastral | `situacao_cadastral` |
| `data_situacao_cadastral` | `date` | Data situação cadastral | |
| `motivo_situacao_cadastral` | `int` | Motivo da situação cadastral | `motivo` |
| `nome_cidade_no_exterior` | `text` | Nome da cidade no exterior | |
| `codigo_pais` | `int` | País | `pais` |
| `data_inicio_atividade` | `date` | Data de início atividade | |
| `cnae_fiscal` | `int` | CNAE fiscal principal | `cnae` |
| `descricao_tipo_de_logradouro` | `text` | Tipo de logradouro | |
| `logradouro` | `text` | Logradouro | |
| `numero` | `text` | Número | |
| `complemento` | `text` | Complemento | |
| `bairro` | `text` | Bairro | |
| `cep` | `text` | CEP | |
| `uf` | `char(2)` | UF | |
| `codigo_municipio` | `int` | Município (código da Receita) | `municipio`; código do IBGE em `municipio_ibge` |
| `ddd_telefone_1` | `text` | DDD 1 e Telefone 1 | |
| `ddd_telefone_2` | `text` | DDD 2 e Telefone 2 | |
| `ddd_fax` | `text` | DDD do fax e Fax | |
| `email` | `text` | Correio eletrônico | |
| `situacao_especial` | `text` | Situação especial | |
| `data_situacao_especial` | `date` | Data da situação especial | |

## `estabelecimento_cnae`

CNAEs secundários, de `Estabelecimentos*.zip`.

| Coluna | Tipo | Descrição |
|---|---|---|
| `cnpj` | `char(14)` | liga com `estabelecimento` |
| `cnae` | `int` | CNAE fiscal secundária (descrição em `cnae`) |
| `ordem` | `int` | posição na lista original, a partir de 0 |

## `simples`

Uma linha por CNPJ básico, de `Simples.zip`.

| Coluna | Tipo | Nome no _layout_ da Receita Federal |
|---|---|---|
| `cnpj_base` | `char(8)` | CNPJ básico |
| `opcao_pelo_simples` | `boolean` | Opção pelo Simples |
| `data_opcao_pelo_simples` | `date` | Data de opção pelo Simples |
| `data_exclusao_do_simples` | `date` | Data de exclusão do Simples |
| `opcao_pelo_mei` | `boolean` | Opção pelo MEI |
| `data_opcao_pelo_mei` | `date` | Data de opção pelo MEI |
| `data_exclusao_do_mei` | `date` | Data de exclusão do MEI |

## `socio`

Quadro societário, de `Socios*.zip`.

| Coluna | Tipo | Nome no _layout_ da Receita Federal | Descrição em |
|---|---|---|---|
| `cnpj_base` | `char(8)` | CNPJ básico | |
| `identificador_de_socio` | `int` | Identificador de sócio | |
| `nome_socio` | `text` | Nome do sócio (no caso PF) ou razão social (no caso PJ) | |
| `cnpj_cpf_do_socio` | `text` | CNPJ/CPF do sócio | |
| `codigo_qualificacao_socio` | `int` | Qualificação do sócio | `qualificacao` |
| `data_entrada_sociedade` | `date` | Data de entrada sociedade | |
| `codigo_pais` | `int` | País | `pais` |
| `cpf_representante_legal` | `text` | Representante legal | |
| `nome_representante_legal` | `text` | Nome do representante | |
| `codigo_qualificacao_representante_legal` | `int` | Qualificação do representante legal | `qualificacao` |
| `codigo_faixa_etaria` | `int` | Faixa etária | `faixa_etaria` |

## `regime_tributario`

Regime tributário, de `entidades-imunes-e-isentas.zip`, `entidades-lucro-arbitrado.zip`, `entidades-lucro-presumido.zip` e `entidades-lucro-real.zip`.

| Coluna | Tipo | Descrição |
|---|---|---|
| `cnpj` | `char(14)` | liga com `estabelecimento` |
| `tipo` | `text` | arquivo de origem: `imune-isenta`, `arbitrado`, `presumido` ou `real` |
| `ano` | `int` | ano |
| `cnpj_da_scp` | `text` | CNPJ da SCP |
| `forma_de_tributacao` | `text` | forma de tributação |
| `quantidade_de_escrituracoes` | `int` | quantidade de escriturações |

## Tabelas de referência

Todas têm as colunas `codigo` (`int`) e `descricao` (`text`), exceto `municipio_ibge`.

| Tabela | Origem |
|---|---|
| `cnae` | `Cnaes.zip` |
| `motivo` | `Motivos.zip` |
| `municipio` | `Municipios.zip` |
| `natureza_juridica` | `Naturezas.zip` |
| `pais` | `Paises.zip` (mais os códigos do aviso acima) |
| `qualificacao` | `Qualificacoes.zip` |
| `situacao_cadastral` | _layout_ da Receita Federal |
| `identificador_matriz_filial` | _layout_ da Receita Federal |
| `porte` | _layout_ da Receita Federal |
| `faixa_etaria` | _layout_ da Receita Federal |
| `municipio_ibge` | [`TABMUN.CSV` do Tesouro Nacional](https://www.tesourotransparente.gov.br/ckan/dataset/abb968cb-3710-4f85-89cf-875c91b9c7f6/resource/eebb3bc6-9eea-4496-8bcf-304f33155282/): `codigo_municipio` (código da Receita) e `codigo_ibge` |

## `meta`

| Coluna | Descrição |
|---|---|
| `key` | chave; `updated-at` guarda o mês do snapshot carregado (`YYYY-MM`) |
| `value` | valor |
