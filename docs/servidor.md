# Criando seu próprio servidor

## Banco de dados

O projeto requer um banco de dados PostgreSQL e os comandos que requerem banco de dados aceitam `--database-uri` (ou `-u`) como argumento com a URI de acesso ao banco de dados (o padrão é o valor da variável de ambiente `DATABASE_URL`).

Caso deseje usar o Docker Compose do projeto para subir uma instância do banco de dados:

```console
$ docker compose up -d postgres
```

A URI será `postgres://minhareceita:minhareceita@localhost:5432/minhareceita?sslmode=disable`.

### Provisionando o banco de dados

O comando `provision` instala e configura o PostgreSQL em um servidor remoto via SSH e exibe as credenciais de acesso.

* [Pulumi CLI](https://www.pulumi.com/)
* servidor Debian ou Ubuntu
* acesso SSH (acesso com senha não é permitido) e acesso `sudo` ao servidor de destino

```console
$ minha-receita provision root@200.100.0.1
```

O comando cria dois usuários no Postgres: `etl` (com permissões de escrita) e `web` (apenas leitura), ambos tem senhas distintas e aleatórias que são exibidas apenas uma única vez ao final do comando.

As URIs exibidas podem ser usadas na variável `DATABASE_URL`: a do `etl` para carregar e atualizar os dados, a do `web` para consultas somente leitura.

## Dados

Os dados são disponibilizados mensalmente pela [Receita Federal](https://dados.gov.br/dados/conjuntos-dados/cadastro-nacional-da-pessoa-juridica-cnpj). Para o comando `transform` funcionar, é necessário um diretório contendo:

* o arquivo `YYYY-MM.zip` da Receita Federal com os dados do CNPJ
* os arquivos de regime tributário com prefixo `entidades-*.zip`

!!! tip "Download automático"
    O comando `download` baixa esses arquivos direto da Receita Federal (o mês mais recente, ou `--month YYYY-MM`), já no formato esperado pelo `transform`/`update`:
    ```console
    $ minha-receita download
    ```

A fonte oficial desses arquivos é sempre o [Portal de Dados Abertos](https://dados.gov.br) do governo federal:

1. Busque por _CPNJ_ e escolha _Cadastro Nacional da Pessoa Jurídica - CNPJ_
2. Na aba _Recursos_ os arquivos necessários são encontrados em:
    * Inscrições no CNPJ (`YYYY-MM.zip`)
    * Regimes Tributários (`entidades-*.zip`)

## Tratamento dos dados

O comando `transform` carrega os arquivos CSV nas tabelas relacionais do banco de dados (descritas no [dicionário de dados](dicionario.md)). Para tanto, é preciso criar as tabelas com o comando `create` (o comando `drop` pode ser utilizado para excluí-las).

Para especificar onde ficam os arquivos originais da Receita Federal e do Tesouro Nacional, o comando aceita como argumento `--directory` (ou `-d`), sendo o padrão `data/`.


!!! tip "Atualização incremental"
    O comando `update` aplica um novo snapshot mensal de forma incremental: carrega os dados em tabelas de _staging_, compara com os dados atuais por _hash_, e aplica apenas as diferenças (inserções, alterações e remoções) — em vez de recriar o banco inteiro. O `transform` (recarga completa) segue sendo usado para o _bootstrap_ inicial.

### Exemplos de uso

Sem Docker, com a variável de ambiente `DATABASE_URL` configurada:

```console
$ minha-receita drop  # caso necessário
$ minha-receita create
$ minha-receita transform
```

Para aplicar um novo snapshot mensal incrementalmente (sem recriar o banco), coloque os novos arquivos no diretório de dados e rode:

```console
$ minha-receita update
```

Com Docker:

```console
$ docker compose run --rm minha-receita drop  # caso necessário
$ docker compose run --rm minha-receita create
$ docker compose run --rm minha-receita transform -d /mnt/data/
```

### Dados pessoais

Os dados são gravados como a Receita Federal os publica, incluindo dados de pessoas físicas que constam nos arquivos (por exemplo, e-mails, nomes de sócios, CPFs parcialmente mascarados pela própria Receita e CPFs que aparecem em nomes fantasia de empresários individuais). Leve isso em conta ao dar acesso ao banco de dados.
