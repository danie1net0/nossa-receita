# Introdução

Os arquivos estão organizados em pacotes Go, sendo esses os dois principais:

1. `transform/` é um <abbr title="Extract, Transform, Load">ETL</abbr> que processa os arquivos da Receita Federal e salva os dados no banco de dados — entenda melhor sobre essa etapa lendo sobre os [detalhes do <abbr title="Extract, Transform, Load">ETL</abbr>](etl.md)
1. `db/` implementação do _backend_ de banco de dados, ou seja, implementa as interfaces necessárias pelo ETL e pela atualização incremental no PostgreSQL

Escreva testes e rode os testes, use autoformatação e _linter_:

```console
$ gofmt ./
$ golanglint-ci run ./...
$ go test --race ./...
```

Os testes requerem uma instância do PostgreSQL configurada em `TEST_POSTGRES_URL`, como no exemplo em `.env`, que pode ser [facilmente criada com o Docker Compose](docker.md).

## Vibe coding

Sobre contribuições e [_vibe coding_](https://pt.wikipedia.org/wiki/Vibe_coding):

* Você pode usar LLMs para ajudar a gerar código, desde que você leia e entenda o código que foi gerado.
* O código enviado deve ser algo que você testou, compreende e é capaz de discutir sobre a implementação, abstrações e testes sugeridos.
* Você deve seguir os critérios de [transparência como na política da comunidade do Fedora](https://docs.fedoraproject.org/en-US/council/policy/ai-contribution-policy/); resumindo: se uma parte signifiativa da contribuição foi gerada por LLM e você não mudou quase nada, é **obrigatório** informar sobre o emprego dessas ferramentas.

Contribuições que não se encaixem nesses critérios serão simplesmente descartadas. Se você não se importou em escrever teu código, não espere que alguém vá se importar em lê-lo.
