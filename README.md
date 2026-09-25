# fiapx-video-processing-service

Microsservico de processamento de video do sistema FIAP X (upload de video ->
extracao assincrona de frames -> zip para download), um dos quatro
microsservicos do desafio.

Este servico e um worker: consome um comando "processe este video", baixa o
video original do MinIO, extrai frames com `ffmpeg`, zipa os frames, sobe o
zip de volta pro MinIO e reporta sucesso ou falha via evento.

## Arquitetura

Este servico nao expoe nenhuma API HTTP de negocio — apenas `/healthz` e
`/readyz`. Toda a logica roda de forma assincrona, disparada por eventos no
RabbitMQ (exchange topic `video.events`), usando o padrao transactional
outbox para garantir que a escrita no banco e o evento correspondente nunca
fiquem inconsistentes entre si.

Organizacao dos processos (3 binarios, 1 banco de dados, 1 broker AMQP e 1
bucket MinIO compartilhados com os demais servicos):

- `cmd/server` — apenas health checks (`/healthz`, `/readyz`); tambem roda as
  migrations no boot, antes do worker e do dispatcher comecarem a
  consumir/publicar.
- `cmd/worker` — consome `video.process.requested` de
  `processing-service.events.q` e executa o pipeline: download do MinIO ->
  `ffmpeg` (extracao de frames) -> zip -> upload do zip pro MinIO -> grava o
  resultado e o evento de saida na mesma transacao (outbox).
- `cmd/outbox-dispatcher` — faz polling da tabela `outbox` e publica as
  linhas ainda nao publicadas no RabbitMQ, com backoff exponencial em caso de
  falha de publicacao.

## Pipeline do worker

1. Recebe o comando `video.process.requested` (payload: `video_id`,
   `user_id`, `saga_id`, `source_bucket`, `source_object_key`,
   `frame_interval_seconds`).
2. Verifica idempotencia via `processed_events` (mesmo padrao dos servicos
   irmaos) — reentrega do mesmo `event_id` e um no-op.
3. Grava uma linha `processing_jobs` com status `RUNNING` (escrita isolada,
   sem evento de outbox — e estado de auditoria proprio deste servico, nao um
   fato que outro servico precise reagir).
4. Baixa o video original do MinIO para um diretorio temporario
   (`os.MkdirTemp`, removido ao final via `defer`).
5. Roda `ffmpeg` (via `exec.CommandContext`, com timeout configuravel) para
   extrair um frame a cada N segundos.
6. **Se o `ffmpeg` falhar** (video corrompido, formato invalido etc.): isso e
   tratado como um resultado de negocio terminal e deterministico, nao como
   um erro de infraestrutura — o job vai para `FAILED`, um evento
   `video.processing.failed` e gravado no outbox (com o `stderr` do `ffmpeg`
   truncado como `error_message`), e o handler retorna `nil` (sucesso ao
   *tratar* a falha), para que o RabbitMQ nao reentregue o comando —
   reprocessar o mesmo video invalido teria o mesmo resultado.
7. **Se o `ffmpeg` tiver sucesso**: conta os frames extraidos, zipa o
   diretorio (`archive/zip`, sem depender do binario `zip` externo), sobe o
   zip para `processed/{user_id}/{video_id}/frames.zip` no MinIO, grava o job
   como `COMPLETED` e o evento `video.processing.completed` no outbox.
8. Qualquer erro de infraestrutura (banco fora do ar, MinIO inacessivel,
   payload malformado) faz o handler retornar `error`, para que o mecanismo
   de retry/DLQ do `messaging.Conn` reentregue o comando ate `MaxRetries`
   vezes antes de mandar para a dead-letter queue.

## Desvio de padrao: imagem Docker nao-distroless

Ao contrario dos outros tres microsservicos (que usam
`gcr.io/distroless/static-debian12:nonroot` no estagio final), este servico
usa `alpine:3.20` com `apk add ffmpeg`, porque precisa executar o binario
`ffmpeg` real como subprocesso, e distroless nao tem gerenciador de pacotes
para instala-lo. Veja
`docs/adr/0004-ffmpeg-requires-non-distroless-image.md` para o racional
completo — e um desvio deliberado e isolado a este repositorio, nao um
esquecimento.

## Rodando localmente (standalone)

Requer Postgres, RabbitMQ e um MinIO (ou qualquer storage compativel com S3)
acessiveis pelas variaveis de ambiente abaixo, alem do binario `ffmpeg`
instalado no `PATH` para o `cmd/worker`.

```bash
docker run -d --name processing-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=processing_service -p 5432:5432 postgres:16-alpine
docker run -d --name processing-rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management-alpine
docker run -d --name processing-minio -p 9000:9000 -p 9001:9001 -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin minio/minio server /data --console-address ":9001"

export PROCESSING_DB_DSN="host=localhost user=postgres password=postgres dbname=processing_service port=5432 sslmode=disable"
export PROCESSING_AMQP_URL="amqp://guest:guest@localhost:5672/"
export MINIO_ENDPOINT="localhost:9000"
export MINIO_ACCESS_KEY="minioadmin"
export MINIO_SECRET_KEY="minioadmin"
export MINIO_BUCKET="fiapx-videos"

go run ./cmd/server             # apenas /healthz e /readyz na porta :8082, roda migrations no boot
go run ./cmd/outbox-dispatcher  # processo separado, faz polling da outbox
go run ./cmd/worker             # processo separado, consome video.process.requested (precisa de ffmpeg no PATH)
```

Ou usando as imagens de container geradas a partir do `Dockerfile` deste
repositorio (ja com `ffmpeg` embutido no estagio final):

```bash
docker build --build-arg TARGET=worker -t fiapx-video-processing-service:worker .
docker run --rm \
  -e PROCESSING_DB_DSN="host.docker.internal:..." \
  -e PROCESSING_AMQP_URL="amqp://guest:guest@host.docker.internal:5672/" \
  -e MINIO_ENDPOINT="host.docker.internal:9000" \
  fiapx-video-processing-service:worker
```

## Rodando como parte da stack completa

Este servico e composto como parte da stack completa do FIAP X junto com os
outros tres microsservicos, Postgres, RabbitMQ e MinIO compartilhados (veja o
repositorio que centraliza o `docker-compose`/deploy da stack completa, nao
incluido neste repositorio).

## Variaveis de ambiente

| Variavel                              | Padrao                                                                                                | Descricao                                                    |
|----------------------------------------|---------------------------------------------------------------------------------------------------------|----------------------------------------------------------------|
| `PROCESSING_PORT`                     | `8082`                                                                                                   | Porta HTTP do `cmd/server` (apenas health checks)               |
| `PROCESSING_DB_DSN`                   | `host=localhost user=postgres password=postgres dbname=processing_service port=5432 sslmode=disable`   | DSN do Postgres (formato GORM/`lib/pq`)                        |
| `PROCESSING_AMQP_URL`                 | `amqp://guest:guest@localhost:5672/`                                                                    | URL de conexao com o RabbitMQ                                   |
| `PROCESSING_DISPATCH_INTERVAL_MS`     | `500`                                                                                                     | Intervalo de polling da outbox no `cmd/outbox-dispatcher`      |
| `MINIO_ENDPOINT`                      | `localhost:9000`                                                                                          | Endpoint do MinIO/S3 (host:porta, sem esquema)                  |
| `MINIO_ACCESS_KEY`                    | `minioadmin`                                                                                              | Access key do MinIO/S3                                          |
| `MINIO_SECRET_KEY`                    | `minioadmin`                                                                                              | Secret key do MinIO/S3                                          |
| `MINIO_BUCKET`                        | `fiapx-videos`                                                                                            | Bucket usado tanto para o video original quanto para o zip      |
| `MINIO_USE_SSL`                       | `false`                                                                                                    | Usa `https://` em vez de `http://` para o endpoint do MinIO/S3  |
| `PROCESSING_FRAME_INTERVAL_SECONDS`   | `1`                                                                                                        | Intervalo padrao (segundos) entre frames extraidos              |
| `PROCESSING_FFMPEG_TIMEOUT_SECONDS`   | `600`                                                                                                      | Timeout maximo do subprocesso `ffmpeg` por job                  |

## Participacao no fluxo de eventos

- **Consome**: `video.process.requested` (comando) — de
  `processing-service.events.q`.
- **Emite**: `video.processing.completed` (payload: `video_id`, `user_id`,
  `job_id`, `zip_bucket`, `zip_object_key`, `frame_count`, `completed_at`) ou
  `video.processing.failed` (payload: `video_id`, `user_id`, `job_id`,
  `error_code`, `error_message`, `failed_at`). Ambos publicados via padrao
  outbox.
- **Idempotencia**: tabela `processed_events`, indexada por `event_id`,
  verificada antes do processamento e marcada apos um resultado terminal
  (sucesso ou falha de ffmpeg) ser gravado. Combinado com a constraint de
  chave primaria, isso torna o reenvio do mesmo `video.process.requested` uma
  operacao sem efeito (no-op).

## Testes

```bash
go test ./...                                   # apenas testes unitarios, sem dependencias externas, rapido
go test -tags=integration ./tests/integration/... # exige ffmpeg real + tests/fixtures/sample.mp4 (veja tests/fixtures/README.md)
```

Os testes unitarios de `internal/application/usecases` cobrem o caso de uso
`ProcessVideoUseCase` (sucesso, falha de ffmpeg, replay idempotente, payload
invalido, erro de infraestrutura) contra fakes de
`ProcessingJobRepository`/`ProcessedEventRepository`/`Storage`/`Extractor` —
nenhum deles chama um binario `ffmpeg` real ou um MinIO real. Os testes de
`internal/infrastructure/ffmpeg` cobrem `BuildExtractArgs` (funcao pura) e
`ZipDirectory` (contra o `archive/zip` real, em arquivos temporarios) sem
depender do binario `ffmpeg`. O teste de integracao ponta a ponta com
`ffmpeg` real fica atras da build tag `integration` e depende de
`tests/fixtures/sample.mp4`, que ainda nao foi adicionado a este repositorio
(veja `tests/fixtures/README.md`).

## Notas de desenvolvimento / desvios da especificacao

- O `go.mod` fixa `go 1.25.11`, diferente do `go 1.23.2` dos servicos irmaos.
  Isso nao foi uma escolha deliberada: `go mod tidy` resolveu essa versao
  como piso minimo porque `aws-sdk-go-v2` (dependencia nova, exclusiva deste
  servico, sem equivalente nos irmaos) e a versao atual do
  `golang-migrate/migrate/v4` declaram esse requisito em seus proprios
  `go.mod`. Ao contrario do que o `pos-os-service` fez (fixar todas as
  dependencias em versoes antigas compativeis com `go 1.23.2`), aqui nao foi
  perseguido o mesmo pin exaustivo — o toolchain local (`go1.26.5`) builda e
  testa normalmente com `go 1.25.11`, e travar o SDK da AWS em versoes velhas
  o bastante para suportar `go 1.23` exigiria um esforco de arqueologia de
  versoes desproporcional ao ganho.
- `tests/fixtures/sample.mp4` (video real curto usado pelo teste de
  integracao de ffmpeg) nao foi adicionado neste commit inicial — gerar ou
  obter um MP4 minimo valido para versionar ficou fora do escopo desta etapa.
  O teste em `tests/integration/extract_frames_test.go` pula a si mesmo
  (`t.Skip`) quando o arquivo nao existe, em vez de falhar.
- Os charts Helm (`charts/processing-service/`) seguem a mesma estrutura dos
  servicos irmaos (`Chart.yaml` + `templates/` + `values.yaml`), com recursos
  de CPU/memoria do worker ajustados para cima em relacao aos outros
  servicos, ja que `ffmpeg` e sensivelmente mais pesado que uma transicao de
  maquina de estados em memoria.

## TODO / proximos passos

- Adicionar `tests/fixtures/sample.mp4` e validar o teste de integracao de
  ffmpeg de ponta a ponta em uma maquina com `ffmpeg` instalado.
- Adicionar testes de integracao para `ProcessingJobRepository`/
  `OutboxRepository`/`ProcessedEventRepository` contra Postgres real
  (testcontainers-go), no mesmo padrao usado pelos servicos irmaos.
- Configurar credenciais de um registry real no CI quando existir um para
  este servico.
