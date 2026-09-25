# tests/fixtures

Este diretorio deve conter um `sample.mp4` — um video curto, real e valido
(poucos segundos bastam) — usado pelo teste de integracao de ffmpeg em
`tests/integration/` (build tag `integration`) para exercitar
`ffmpeg.ExtractFrames` contra o binario `ffmpeg` de verdade, de ponta a ponta
(extracao de frames + zip), em vez das fakes usadas nos testes unitarios de
`internal/application/usecases`.

O arquivo nao foi adicionado neste commit inicial (gerar ou obter um MP4
minimo, valido e leve o suficiente para versionar ficou fora do escopo desta
etapa). Antes de rodar `make test-integration`, adicione um `sample.mp4`
aqui — por exemplo, um clipe de 2-3 segundos gerado localmente com:

```bash
ffmpeg -f lavfi -i testsrc=duration=3:size=320x240:rate=5 tests/fixtures/sample.mp4
```
