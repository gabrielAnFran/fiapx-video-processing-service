.PHONY: build test test-integration lint cover docker helm-lint run

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags integration ./tests/integration/...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, falling back to go vet + gofmt"; \
		go vet ./...; \
		test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1); \
	fi

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

docker:
	docker build -t fiapx-video-processing-service:server --build-arg TARGET=server .
	docker build -t fiapx-video-processing-service:outbox-dispatcher --build-arg TARGET=outbox-dispatcher .
	docker build -t fiapx-video-processing-service:worker --build-arg TARGET=worker .

helm-lint:
	@if command -v helm >/dev/null 2>&1; then \
		helm lint charts/processing-service; \
	else \
		echo "helm not installed, skipping lint"; \
	fi

run:
	PROCESSING_PORT=$${PROCESSING_PORT:-8082} \
	PROCESSING_DB_DSN=$${PROCESSING_DB_DSN:-"host=localhost user=postgres password=postgres dbname=processing_service port=5432 sslmode=disable"} \
	PROCESSING_AMQP_URL=$${PROCESSING_AMQP_URL:-"amqp://guest:guest@localhost:5672/"} \
	MINIO_ENDPOINT=$${MINIO_ENDPOINT:-"localhost:9000"} \
	MINIO_ACCESS_KEY=$${MINIO_ACCESS_KEY:-"minioadmin"} \
	MINIO_SECRET_KEY=$${MINIO_SECRET_KEY:-"minioadmin"} \
	MINIO_BUCKET=$${MINIO_BUCKET:-"fiapx-videos"} \
	go run ./cmd/server
