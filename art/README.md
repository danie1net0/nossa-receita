# Artes

Pixel art 16-bit: as caixas de arquivo são os arquivos que a Receita publica, o
gopher com a lupa é a carga que lê e confere cada registro, e o cilindro é o
banco onde tudo termina.

| Arquivo | Tamanho | Onde é usado |
|---|---|---|
| [`avatar-512.png`](avatar-512.png) | 512×512 | Original, de onde saem os outros tamanhos |
| [`avatar-256.png`](avatar-256.png) | 256×256 | Avatar do projeto no GitLab |
| [`banner-1280x640.png`](banner-1280x640.png) | 1280×640 | Topo do README e *social preview* do GitHub |

Paleta: fundo grafite `#242b33`, objetos em marfim com sombra cinza-ardósia,
destaques em coral `#dd5e53` e o ciano do Go no gopher.

O avatar de 256 px é o que o GitLab aceita, cujo limite é 200 KB. Para gerar
outro tamanho a partir do original:

```console
$ sips -Z 256 avatar-512.png --out avatar-256.png
```

Nenhuma das duas leva o nome do projeto: o nome já vem ao lado da imagem nos
dois sites.
