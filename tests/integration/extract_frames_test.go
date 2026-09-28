//go:build integration

// Package integration holds tests that require real external dependencies
// (a real ffmpeg binary and, eventually, Postgres/RabbitMQ via
// testcontainers-go, mirroring the sibling services' tests/integration
// setup). Kept behind the `integration` build tag so `go test ./...` stays
// fast and dependency-free; see tests/fixtures/README.md for what this test
// still needs before it can run.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/ffmpeg"
)

// TestExtractFrames_RealFFmpeg is a placeholder for the end-to-end ffmpeg
// pipeline test (extract frames from a real short MP4, then zip them). It is
// skipped until tests/fixtures/sample.mp4 is added — see that directory's
// README.md.
func TestExtractFrames_RealFFmpeg(t *testing.T) {
	fixture := filepath.Join("..", "fixtures", "sample.mp4")
	if _, err := os.Stat(fixture); os.IsNotExist(err) {
		t.Skip("tests/fixtures/sample.mp4 not present yet, see tests/fixtures/README.md")
	}

	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ffmpeg.ExtractFrames(ctx, fixture, outDir, 1); err != nil {
		t.Fatalf("ExtractFrames failed: %v", err)
	}

	count, err := ffmpeg.CountFrames(outDir)
	if err != nil {
		t.Fatalf("CountFrames failed: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one extracted frame")
	}
}

// TestAdapter_RealFFmpeg exercises ffmpeg.Adapter (ExtractFrames,
// CountFrames, ZipDirectory), the thin wrapper the use case layer actually
// depends on via the usecases.Extractor interface. The free functions it
// delegates to are already covered by TestExtractFrames_RealFFmpeg and the
// package's own unit tests; this test's job is only to prove the Adapter
// wiring itself works end to end against the real ffmpeg binary.
func TestAdapter_RealFFmpeg(t *testing.T) {
	fixture := filepath.Join("..", "fixtures", "sample.mp4")
	if _, err := os.Stat(fixture); os.IsNotExist(err) {
		t.Skip("tests/fixtures/sample.mp4 not present yet, see tests/fixtures/README.md")
	}

	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adapter := ffmpeg.NewAdapter()

	if err := adapter.ExtractFrames(ctx, fixture, outDir, 1); err != nil {
		t.Fatalf("Adapter.ExtractFrames failed: %v", err)
	}

	count, err := adapter.CountFrames(outDir)
	if err != nil {
		t.Fatalf("Adapter.CountFrames failed: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one extracted frame")
	}

	zipPath := filepath.Join(t.TempDir(), "frames.zip")
	if err := adapter.ZipDirectory(outDir, zipPath); err != nil {
		t.Fatalf("Adapter.ZipDirectory failed: %v", err)
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("expected zip file to exist: %v", err)
	}
}
