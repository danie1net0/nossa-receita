<h1 align="center">
  <img src="art/banner-1280x640.png" width="640" alt="Nossa Receita">
</h1>

<p align="center">
  Banco de dados PostgreSQL com as informações do CNPJ da Receita Federal,<br>
  carregado do zero e atualizado todo mês de forma incremental.
</p>

<p align="center">
  <img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
  <img alt="PostgreSQL 18" src="https://img.shields.io/badge/PostgreSQL-18-4169E1?logo=postgresql&logoColor=white">
  <img alt="Licença MIT" src="https://img.shields.io/badge/licen%C3%A7a-MIT-dd5e53">
  <a href="https://codeberg.org/cuducos/minha-receita"><img alt="fork de cuducos/minha-receita" src="https://img.shields.io/badge/fork%20de-cuducos%2Fminha--receita-dd5e53"></a>
</p>

---

Fork do [Minha Receita](https://codeberg.org/cuducos/minha-receita) **sem a API**:
as tabelas são o produto final e são consultadas direto no banco. O módulo e o
binário continuam se chamando `minha-receita`.

O que muda em relação ao projeto original:

- **Só PostgreSQL.** Não há outro backend nem a tabela de documentos JSON; os
  dados ficam no modelo relacional, que é o que se consulta.
- **Sem servidor HTTP.** Os comandos de API e o filtro de dados pessoais, que
  só valiam para os documentos JSON, foram removidos.
- **Atualização incremental.** O `update` carrega o mês novo em tabelas
  `*_staging` e aplica só a diferença, em vez de refazer a carga inteira.

## Como usar

```console
$ minha-receita create     # cria as tabelas
$ minha-receita download   # baixa os arquivos do mês na pasta de dados
$ minha-receita transform  # carga completa
$ minha-receita update     # meses seguintes, só a diferença
```

A [documentação do projeto original](https://docs.minhareceita.org) continua
valendo para instalação, para a origem dos dados e para o dicionário de campos;
as partes sobre a API não se aplicam aqui.

[Guia de contribuição](https://docs.minhareceita.org/contributing/)<br>
[Ajude a manter o projeto original](https://apoia.se/minhareceita)

## Artes

O avatar e o banner estão em [`art/`](art), em pixel art 16-bit: as caixas de
arquivo são os arquivos que a Receita publica, o gopher com a lupa é a carga
que lê e confere cada registro, e o cilindro é o banco onde tudo termina.
