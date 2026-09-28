//go:build integration

package integration

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/storage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestS3Client builds a storage.S3Client pointed at the shared MinIO
// container.
func newTestS3Client(t *testing.T) *storage.S3Client {
	t.Helper()
	client, err := storage.NewS3Client(context.Background(), testMinIOEndpoint, testMinIOAccessKey, testMinIOSecretKey, testMinIOBucket, false)
	require.NoError(t, err)
	return client
}

func TestS3Client_Upload_Then_Download_RoundTrip(t *testing.T) {
	client := newTestS3Client(t)
	ctx := context.Background()

	key := "raw/" + uuid.NewString() + ".mp4"
	content := []byte("the quick brown fox jumps over the lazy dog, in mp4 disguise")
	require.NoError(t, client.Upload(ctx, key, bytes.NewReader(content), "video/mp4"))

	rc, err := client.Download(ctx, key)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestS3Client_Upload_LargeBody_Succeeds(t *testing.T) {
	client := newTestS3Client(t)
	ctx := context.Background()

	key := "processed/" + uuid.NewString() + "/frames.zip"
	content := bytes.Repeat([]byte("frame-bytes-"), 100_000) // ~1.2MB, exercises the multipart-safe uploader
	require.NoError(t, client.Upload(ctx, key, bytes.NewReader(content), "application/zip"))

	rc, err := client.Download(ctx, key)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestS3Client_Download_MissingKey_ReturnsError(t *testing.T) {
	client := newTestS3Client(t)
	ctx := context.Background()

	_, err := client.Download(ctx, "raw/does-not-exist-"+uuid.NewString()+".mp4")
	assert.Error(t, err)
}
