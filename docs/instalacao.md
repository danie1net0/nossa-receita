# Instalação local

Existem três formas de rodar essa aplicação localmente:

* ou com a imagem Docker
* ou gerando o binário a partir do código fonte
* ou com Docker Compose — apenas para desenvolvimento (não recomendado para o banco de dados completo)

As duas últimas alternativas necessitam do código fonte. Você pode usar o Git para baixar o código do projeto:

```console
$ git clone https://codeberg.org/cuducos/minha-receita.git
```

## Requisitos e instalação

O banco de dados completo ocupa cerca de 55Gb, com os índices. A máquina que faz o download e a transformação dos dados precisa de mais cerca de 30Gb temporários, e o `update` precisa de espaço para uma segunda cópia das tabelas (as de _staging_) enquanto roda.

No fim da carga completa (`transform`), as tabelas são reescritas uma a uma para voltarem a ser duráveis, e as chaves primárias e os índices são recriados. Enquanto cada tabela é reescrita, a cópia antiga só é apagada ao final, então é preciso ter livre, além da base, espaço para uma segunda cópia da maior tabela (`estabelecimento`, cerca de 21Gb) e para as ordenações da criação dos índices: reserve ao menos 30Gb. No macOS, os _snapshots_ locais do Time Machine seguram o espaço de arquivos apagados ou reescritos; se o disco encher, libere-os com `sudo tmutil thinlocalsnapshots / 999999999999 4`.

### Banco de dados

* O banco de dados gerado utiliza cerca de 55Gb
* Rodar o banco de dados localmente com Docker só é recomendado para desenvolvimento (não recomendado para o banco de dados completo)

#### Download e transformação dos dados

* Os arquivos da Receita federal tem menos de 10Gb
* O processo de importação utiliza uma estrutura temporária de cerca de 15Gb

### Imagem Docker

* [Docker](https://www.docker.com/)

Baixar a imagem com:

```console
$ docker pull atcr.io/cuducos.me/minha-receita:main
```

### A partir do código fonte

* [Go](https://golang.org/) versão 1.26
* Variável de ambiente `GOEXPERIMENT=jsonv2`

Depois de clonar o repositório, baixe as dependências e compile a aplicação para um diretório incluído no `PATH`, por exemplo:

```console
$ go get
$ go build -o /usr/local/bin/minha-receita main.go
```

### Docker Compose

* [Docker](https://www.docker.com/)
* [Docker Compose](https://docs.docker.com/compose/install/)
* Arquivo `.env` (copie o `.env.sample` e ajuste caso necessário)

Gere as imagens dos containers com:

```console
$ docker compose build
```

## Execução e configurações

Várias configurações podem ser passadas para a CLI, e elas estão documentadas no `--help` de cada comando da aplicação.

### Exemplos

#### Imagem Docker

```console
$ docker run --rm atcr.io/cuducos.me/minha-receita:main --help
```

#### A partir do código fonte

```console
$ minha-receita --help
```

#### Docker Compose

```console
$ docker compose run --rm minha-receita --help
```

### Variáveis de ambiente

Para facilitar a manutenção, algumas variáveis de ambiente podem ser utilizadas, mas todas são opcionais:

| Variável | Descrição |
|---|---|
| `DATABASE_URL` | URI de acesso ao banco de dados |
| `TEST_POSTGRES_URL` | URI de acesso ao banco de dados PostgreSQL para ser utilizado nos testes |
