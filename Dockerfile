FROM golang:1.23-alpine AS builder
ARG TARGET=server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/${TARGET}

# Deliberate deviation from the sibling services' distroless final stage: this
# service shells out to the real ffmpeg binary, which needs a package manager
# to install (distroless has none). See docs/adr/0004 for the full rationale.
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /out/app /app/app
COPY --from=builder /src/migrations /app/migrations
USER appuser
EXPOSE 8082
ENTRYPOINT ["/app/app"]
