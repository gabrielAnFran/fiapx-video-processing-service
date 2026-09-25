# ADR 0004: Imagem Final Nao-Distroless (Requisito do ffmpeg)

## Status
Aceito

## Contexto
Os quatro microsservicos deste sistema seguem o mesmo `Dockerfile` de duas
etapas: build em `golang:1.23-alpine`, e estagio final em
`gcr.io/distroless/static-debian12:nonroot` — uma imagem minima, sem shell,
sem gerenciador de pacotes e sem bibliotecas alem do necessario para rodar um
binario Go estaticamente linkado. Isso reduz a superficie de ataque e o
tamanho da imagem final dos servicos irmaos (`os-service`, `payment-service`,
`upload-service`).

Este servico (`video-processing-service`), porem, precisa executar o binario
`ffmpeg` real como subprocesso (`internal/infrastructure/ffmpeg/extractor.go`,
via `exec.CommandContext`) para extrair os frames do video. O `ffmpeg` e um
binario de terceiros com suas proprias dependencias dinamicas (libavcodec,
libavformat etc.), e a imagem `distroless/static-debian12` nao tem gerenciador
de pacotes (`apk`, `apt`) nem espaco para instalar esse binario e suas
dependencias manualmente de forma pratica.

## Decisao
O estagio final do `Dockerfile` deste servico usa `alpine:3.20` em vez de
`gcr.io/distroless/static-debian12:nonroot`, e instala o `ffmpeg` via
`apk add --no-cache ffmpeg ca-certificates` nesse estagio. Esse e um desvio
deliberado e documentado do padrao dos tres servicos irmaos, nao um
esquecimento — os outros tres servicos continuam usando distroless
normalmente, ja que nenhum deles depende de um binario externo.

Para compensar a superficie maior da imagem base (Alpine tem shell e
`apk`, ao contrario de distroless), o container continua rodando como usuario
nao-root (`adduser -D -u 10001 appuser` + `USER appuser`), mantendo a mesma
garantia de "nao roda como root" que o `nonroot:nonroot` do distroless
oferece nos servicos irmaos.

## Racional
- **ffmpeg e um binario nativo, nao uma dependencia Go**: nao ha como
  vendorizar ou linkar estaticamente o `ffmpeg` inteiro dentro do binario Go
  deste servico sem reescrever a decodificacao de video em Go puro, o que
  esta fora de escopo.
- **Alpine e o menor superset pratico que resolve isso**: `apk add ffmpeg`
  traz o binario e suas dependencias dinamicas resolvidas automaticamente,
  mantendo a imagem final ainda relativamente pequena comparada a uma base
  Debian/Ubuntu completa.
- **Trade-off aceito**: a imagem final deste servico tem uma superficie de
  ataque maior que a dos servicos irmaos (shell presente, gerenciador de
  pacotes presente). Dado que esse e o unico dos quatro servicos com essa
  necessidade, o desvio fica isolado a um repositorio e nao contamina o
  padrao dos demais.

## Consequencias
- O `Dockerfile` deste servico e o unico dos quatro que nao usa
  `gcr.io/distroless/static-debian12:nonroot` como estagio final.
- Atualizacoes de seguranca do `ffmpeg`/Alpine precisam ser acompanhadas
  separadamente das atualizacoes de dependencias Go deste servico.
- Se no futuro o `ffmpeg` deixar de ser necessario (por exemplo, substituido
  por uma biblioteca Go pura de decodificacao de video), este servico pode
  voltar a usar distroless e esta ADR pode ser marcada como superada.
