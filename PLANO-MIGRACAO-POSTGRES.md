# Plano de migração: documento único → relacional + documento materializado

Migração do armazenamento de `1 documento jsonb por CNPJ` para `tabelas relacionais
como fonte de verdade + documento jsonb materializado`, habilitando **atualização
incremental** a partir dos snapshots mensais da Receita e **filtro nativo por sócios
e CNAEs**. PostgreSQL passa a ser o único backend.

## Estado atual (ponto de partida)

- Schema: `cnpj(cursor serial pk, id char(14), json jsonb)` + `meta(key, value)`.
  Índice único em `id`; índices extra BTREE/GIN sobre campos do JSON
  (`db/postgres/create.sql`, `extra_indexes.sql`).
- ETL (`transform/transform.go`): unzip do `YYYY-MM.zip` → carrega lookups e dados
  relacionais no Badger → lê `Estabelecimentos*`, enriquece pelo Badger e grava
  1 documento `jsonb` por CNPJ.
- API (`api/`): `Serve` (`GetCompany` por id, `Search` paginada, cache Ristretto +
  bloom filter) e `ServeGraph` (tabela `graph`, `GetRelated`).
- Atualização = recarga mensal completa (~140GB).
- Backends: PostgreSQL (produção) e MongoDB, atrás da interface `database`
  (`cmd/db.go`).

## Estado alvo

- Tabelas relacionais normalizadas = **fonte de verdade**.
- `cnpj.json` = **projeção materializada**, regerada por CNPJ afetado.
- Atualização mensal = **diff do snapshot novo contra o atual** → aplicação localizada
  → re-materialização apenas dos CNPJs afetados.
- Recarga completa vira **bootstrap/fallback**, não o caminho normal.

## Invariantes (valem em todas as fases)

1. **Oráculo de correção:** aplicar o mês N+1 incrementalmente sobre o mês N tem que
   produzir um banco **idêntico** a uma recarga completa do mês N+1. Esse é o teste
   que valida toda a máquina de delta — sem ele, nada de cortar a recarga completa.
2. `GetCompany(id)` continua sendo uma sondagem de índice única (sem JOINs em request).
3. Re-materialização é **UPDATE in-place** na linha `cnpj` (preserva o `cursor` →
   paginação keyset estável).
4. **"Baixada" é UPDATE de campo** (`situacao_cadastral`), não DELETE. DELETE só quando
   a chave desaparece do snapshot — caso raro.
5. Cada fase é entregável e reversível isoladamente.

---

## Fase 1 — Remover o MongoDB

**Objetivo:** liberar a interface `database` para evoluir para um modelo relacional,
sem o contrato de "devolver documento" imposto por dois backends.

**Mudanças:**
- Remover `db/mongodb.go`, `db/mongodb_test.go`.
- Tirar o ramo `mongodb://` de `loadDatabase` (`cmd/db.go`) e a flag de erro associada.
- Remover serviços `mongo`/`mongo_test` do `docker-compose.yml`, `TEST_MONGODB_URL` do
  `.env.sample` e do `.woodpecker/tests.yaml`.
- Limpar dependência `go.mongodb.org/mongo-driver` do `go.mod` (`go mod tidy`).
- Atualizar docs (`docs/contributing/index.md`, `docker.md`).

**Verificação:** `GOEXPERIMENT=jsonv2 go test --race ./...` passa sem instância Mongo.

**Risco:** baixo, puramente subtrativo. Reversível por `git revert`.

---

## Fase 2 — Schema relacional em paralelo (dual-write)

**Objetivo:** introduzir as tabelas normalizadas **ao lado** da `cnpj` existente, sem
mudar a fonte de verdade. Já entrega o filtro por sócios/CNAEs.

**Schema novo (`db/postgres/create.sql`):**
```
empresa(cnpj_base char(8) PK, razao_social, natureza_juridica, porte,
        capital_social, ..., hash bytea)
estabelecimento(cnpj char(14) PK, cnpj_base char(8) FK,
                cnae_fiscal, codigo_municipio, uf, situacao_cadastral,
                data_situacao, ..., hash bytea)
socio(cnpj_base char(8) FK, identificador int, nome, cpf_cnpj, qualificacao,
      ..., hash bytea, PRIMARY KEY (cnpj_base, identificador, cpf_cnpj))
simples(cnpj_base char(8) PK FK, opcao_simples, data_simples, opcao_mei, ..., hash bytea)
regime_tributario(cnpj char(14) FK, ano, forma, ..., hash bytea)
estabelecimento_cnae(cnpj char(14) FK, cnae int)         -- CNAEs secundários
```
Mais tabelas de referência (`cnae`, `municipio`, `pais`, `natureza`, `qualificacao`,
`motivo`) populadas a partir dos arquivos de lookup. Índices: `estabelecimento(cnpj_base)`,
`socio(cpf_cnpj)`, `estabelecimento_cnae(cnae)`, `estabelecimento(uf)`, `(cnae_fiscal)`.

**Coluna `hash`:** xxhash64 do conteúdo normalizado de cada registro
(`github.com/cespare/xxhash/v2`, já no `go.mod`). É o que torna o diff barato nas
fases seguintes.

**ETL:** `CreateCompanies` (ou novo caminho) passa a **gravar as linhas relacionais
além do documento jsonb** (dual-write). O `cnpj.json` continua sendo a fonte de verdade
para leitura.

**Verificação:** após uma carga completa, contagens batem com a `cnpj`; amostra de CNPJs
reconstruídos a partir do relacional é semanticamente igual ao `jsonb` armazenado.

**Risco:** aditivo; API intocada. Reversível dropando as tabelas novas.

> Já dá para expor, opcionalmente, endpoints/queries de filtro por sócio e CNAE contra
> as tabelas relacionais — valor entregue antes mesmo da máquina de update existir.

---

## Fase 3 — Materialização derivada (inverter a fonte de verdade)

**Objetivo:** o `cnpj.json` deixa de ser escrito diretamente e passa a ser uma
**projeção** das tabelas relacionais.

**Mudanças:**
- Nova função `Materialize(ctx, cnpjs ...string)` em `db`: monta o documento de cada
  CNPJ via JOIN das tabelas relacionais + resolução das referências (nomes de CNAE,
  município, país, etc.), e faz `UPDATE`/`UPSERT` na `cnpj`.
- A carga completa passa a: (1) gravar relacional, (2) materializar todos os documentos
  a partir do relacional. A etapa "enriquece pelo Badger e grava doc" some.
- Papel do Badger encolhe: lookups viram tabelas de referência no Postgres; o Badger
  pode ser mantido só como acelerador do bulk inicial, ou removido.

**Verificação:** documento materializado == documento do dual-write da Fase 2 (diff
semântico sobre uma amostra grande). A partir daqui, relacional é a verdade.

**Risco:** médio — é onde a lógica de montagem do JSON migra. Mitigado pelo diff contra
os documentos da fase anterior.

---

## Fase 4 — Diff / CDC (cálculo do delta)

**Objetivo:** dado o snapshot novo, descobrir o que mudou sem reprocessar o que não mudou.
Lembrar: **a Receita publica snapshot mensal completo, não delta** — o delta é calculado
por nós.

**Mecanismo (set-based no Postgres):**
1. Parsear o snapshot novo e carregar `(chave, hash, colunas...)` em tabelas de
   **staging UNLOGGED** (uma por entidade).
2. Diff por chave + hash:
   - **inserts** = `staging LEFT JOIN atual` onde `atual.chave IS NULL`
   - **deletes** = `atual LEFT JOIN staging` onde `staging.chave IS NULL`
   - **updates** = `staging JOIN atual` onde `staging.hash <> atual.hash`
3. Resultado: conjuntos de chaves novas/alteradas/removidas por entidade.

**Verificação:** rodar contra dois meses reais consecutivos; conferir as ordens de
grandeza (novas empresas, baixadas, removidas) contra o esperado.

**Risco:** baixo (só leitura/cálculo, não aplica nada ainda).

---

## Fase 5 — Aplicação incremental + re-materialização escopada

**Objetivo:** aplicar o delta e regerar só o necessário.

**Mudanças:**
- Aplicar os changesets nas tabelas relacionais (UPSERT/DELETE).
- Calcular o conjunto de **CNPJs completos afetados** com o fan-out por base:
  - alteração em `empresa(base X)`, `socio(base X)` ou `simples(base X)` →
    **todos os `estabelecimento` da base X** (via índice `estabelecimento(cnpj_base)`).
  - alteração em `estabelecimento(Y)` ou `regime_tributario(Y)` → só `Y`.
- `Materialize(...)` (Fase 3) regenera apenas esses CNPJs.
- Novo comando CLI: `update` (ou `transform --incremental`) em `cmd/`.

**Verificação (o oráculo):** carregar mês N completo, aplicar mês N+1 incrementalmente,
e comparar com uma carga completa do mês N+1 — têm que ser idênticos (relacional e
documentos). Esse teste é o critério de aceite da fase.

**Risco:** alto — é o coração da feature. Mitigado pelo oráculo e por manter a recarga
completa funcional como fallback.

---

## Fase 6 — Cache, bloom filter e busca relacional

**Objetivo:** integrar o update incremental com a camada de serviço.

**Mudanças:**
- **Cache Ristretto:** invalidar (evict) os CNPJs re-materializados, senão a API serve
  documento velho. Definir o gatilho (ex.: o `update` publica a lista, ou a API
  recarrega).
- **Bloom filter:** inserir os CNPJs novos. Remoções não são possíveis em bloom — mas
  não quebram correção (vira falso-positivo que cai no DB e responde "não encontrado").
  Decidir só a cadência de rebuild.
- **Busca:** mover/estender `Search` (`api/`) para consultar as tabelas relacionais,
  habilitando filtros ricos por sócio, CNAE principal/secundário, UF, município, com
  JOIN e estatísticas reais do planner. `AllCompanies` (usada no build do bloom) passa
  a varrer `estabelecimento`.

**Verificação:** ponta a ponta — após um update, a API reflete empresa nova encontrável,
baixada com situação atualizada, e cache não fica obsoleto.

**Risco:** médio (mexe na API). Filtros novos são aditivos.

---

## Fase 7 — Corte e limpeza

**Objetivo:** tornar o incremental o caminho padrão e remover o andaime.

**Mudanças:**
- Update incremental vira o fluxo mensal padrão; recarga completa = bootstrap/fallback.
- Remover dual-write residual e código morto do caminho antigo de materialização.
- Atualizar docs (`docs/contributing/etl.md`, `instalacao.md`) com o novo fluxo e o
  diagrama de 3 etapas revisado.
- Revisar grants do provisionamento (`cmd/provision/setup.sql`) para as tabelas novas
  (usuários `etl` write / `web` read-only).

**Verificação:** suíte completa verde; um ciclo mensal real rodado via `update`.

**Risco:** baixo.

---

## Riscos transversais e decisões em aberto

- **Reprodutibilidade do JSON:** a materialização precisa produzir documentos
  *semanticamente* iguais aos atuais. Como é `jsonb` (normaliza ordenação de chaves),
  defina a igualdade como semântica, não byte a byte. Confirmar se algum cliente
  depende da forma exata do output.
- **DELETE vs soft-delete:** decidir se chave que some do snapshot é removida de fato ou
  marcada. (Distinto de "baixada", que é update de campo.)
- **Mudança em tabela de referência** (ex.: nome de município muda): obrigaria
  re-materializar todos que a referenciam. Raro — tratar como re-materialização
  ampla pontual ou resolver o nome em tempo de leitura.
- **Tipo do hash:** xxhash64 → `bytea(8)` ou `bigint`. Normalizar o input do hash
  (trim, encoding, ordem de campos) para evitar falsos "alterados".
- **Staging UNLOGGED:** acelera o diff, mas não sobrevive a crash — aceitável porque é
  recomputável a partir do snapshot.
- **Armazenamento:** tabelas + índices novos custam disco, mas tirar `qsa`/
  `cnaes_secundarios` de dentro do documento e os tipos nativos compensam parte.

## Ordem de entrega sugerida

Fases 1–2 já entregam Postgres-only + filtro por sócios/CNAEs (valor imediato, risco
baixo). Fases 3–5 entregam o update incremental (o objetivo principal). Fases 6–7
integram e cortam o caminho antigo.
