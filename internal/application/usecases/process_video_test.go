package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProcessCommand(t *testing.T, videoID, userID uuid.UUID, frameInterval int) messaging.Event {
	t.Helper()
	ev, err := messaging.NewEvent("video.process.requested", "corr-x", "saga-x", processVideoCommandPayload{
		VideoID:              videoID.String(),
		UserID:               userID.String(),
		SagaID:               "saga-x",
		SourceBucket:         "fiapx-videos",
		SourceObjectKey:      "uploads/" + videoID.String() + "/original.mp4",
		FrameIntervalSeconds: frameInterval,
	})
	require.NoError(t, err)
	return ev
}

func TestProcessVideo_Success(t *testing.T) {
	jobs := newFakeProcessingJobRepository()
	processed := newFakeProcessedEventRepository()
	storage := &fakeStorage{downloadData: []byte("fake video bytes")}
	extractor := &fakeExtractor{frameCount: 42}
	uc := NewProcessVideoUseCase(jobs, processed, storage, extractor, "fiapx-videos", 30*time.Second)

	videoID, userID := uuid.New(), uuid.New()
	ev := newProcessCommand(t, videoID, userID, 2)

	require.NoError(t, uc.Handle(context.Background(), ev))

	job, err := jobs.FindByVideoID(context.Background(), videoID)
	require.NoError(t, err)
	assert.Equal(t, entities.StatusCompleted, job.Status)
	require.NotNil(t, job.FrameCount)
	assert.Equal(t, 42, *job.FrameCount)
	require.NotNil(t, job.ZipBucket)
	assert.Equal(t, "fiapx-videos", *job.ZipBucket)
	require.NotNil(t, job.ZipObjectKey)
	assert.Equal(t, "processed/"+userID.String()+"/"+videoID.String()+"/frames.zip", *job.ZipObjectKey)
	assert.NotNil(t, job.CompletedAt)

	require.Len(t, jobs.outbox, 1)
	var env messaging.Event
	require.NoError(t, json.Unmarshal(jobs.outbox[0].Payload, &env))
	assert.Equal(t, eventVideoProcessingCompleted, env.EventName)

	var payload processingCompletedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, 42, payload.FrameCount)
	assert.Equal(t, videoID.String(), payload.VideoID)

	assert.Equal(t, *job.ZipObjectKey, storage.uploadedKey)
	assert.Equal(t, 1, extractor.extractCalls)

	eventID, _ := uuid.Parse(ev.EventID)
	isProcessed, err := processed.IsProcessed(context.Background(), eventID)
	require.NoError(t, err)
	assert.True(t, isProcessed)
}

func TestProcessVideo_FFmpegFailure(t *testing.T) {
	jobs := newFakeProcessingJobRepository()
	processed := newFakeProcessedEventRepository()
	storage := &fakeStorage{downloadData: []byte("fake video bytes")}
	extractor := &fakeExtractor{extractErr: errors.New("ffmpeg: invalid data found when processing input")}
	uc := NewProcessVideoUseCase(jobs, processed, storage, extractor, "fiapx-videos", 30*time.Second)

	videoID, userID := uuid.New(), uuid.New()
	ev := newProcessCommand(t, videoID, userID, 1)

	// A terminal ffmpeg failure is a successfully HANDLED outcome: Handle
	// must return nil so the AMQP consumer does not redeliver it.
	require.NoError(t, uc.Handle(context.Background(), ev))

	job, err := jobs.FindByVideoID(context.Background(), videoID)
	require.NoError(t, err)
	assert.Equal(t, entities.StatusFailed, job.Status)
	require.NotNil(t, job.ErrorMessage)
	assert.Contains(t, *job.ErrorMessage, "invalid data found")
	assert.NotNil(t, job.CompletedAt)

	require.Len(t, jobs.outbox, 1)
	var env messaging.Event
	require.NoError(t, json.Unmarshal(jobs.outbox[0].Payload, &env))
	assert.Equal(t, eventVideoProcessingFailed, env.EventName)

	var payload processingFailedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, errCodeFFmpegFailed, payload.ErrorCode)

	eventID, _ := uuid.Parse(ev.EventID)
	isProcessed, err := processed.IsProcessed(context.Background(), eventID)
	require.NoError(t, err)
	assert.True(t, isProcessed)
}

func TestProcessVideo_Idempotent(t *testing.T) {
	jobs := newFakeProcessingJobRepository()
	processed := newFakeProcessedEventRepository()
	storage := &fakeStorage{downloadData: []byte("fake video bytes")}
	extractor := &fakeExtractor{frameCount: 10}
	uc := NewProcessVideoUseCase(jobs, processed, storage, extractor, "fiapx-videos", 30*time.Second)

	videoID, userID := uuid.New(), uuid.New()
	ev := newProcessCommand(t, videoID, userID, 1)

	require.NoError(t, uc.Handle(context.Background(), ev))
	require.NoError(t, uc.Handle(context.Background(), ev))

	// Second call must be a no-op: only one extraction, one outbox event.
	assert.Equal(t, 1, extractor.extractCalls)
	assert.Len(t, jobs.outbox, 1)
}

func TestProcessVideo_InvalidPayload(t *testing.T) {
	jobs := newFakeProcessingJobRepository()
	processed := newFakeProcessedEventRepository()
	storage := &fakeStorage{}
	extractor := &fakeExtractor{}
	uc := NewProcessVideoUseCase(jobs, processed, storage, extractor, "fiapx-videos", 30*time.Second)

	ev, err := messaging.NewEvent("video.process.requested", "corr-x", "", json.RawMessage(`{"video_id": 123}`))
	require.NoError(t, err)
	// Overwrite payload with something that fails to unmarshal into the
	// command struct's string field.
	ev.Payload = json.RawMessage(`{"video_id": 123}`)

	err = uc.Handle(context.Background(), ev)
	require.Error(t, err)
}

func TestProcessVideo_DownloadError_ReturnsErrorForRetry(t *testing.T) {
	jobs := newFakeProcessingJobRepository()
	processed := newFakeProcessedEventRepository()
	storage := &fakeStorage{downloadErr: errors.New("minio unreachable")}
	extractor := &fakeExtractor{}
	uc := NewProcessVideoUseCase(jobs, processed, storage, extractor, "fiapx-videos", 30*time.Second)

	videoID, userID := uuid.New(), uuid.New()
	ev := newProcessCommand(t, videoID, userID, 1)

	err := uc.Handle(context.Background(), ev)
	require.Error(t, err)

	// The RUNNING job row was still written before the infra error.
	job, err2 := jobs.FindByVideoID(context.Background(), videoID)
	require.NoError(t, err2)
	assert.Equal(t, entities.StatusRunning, job.Status)

	// Not marked processed, so a redelivery will retry it.
	eventID, _ := uuid.Parse(ev.EventID)
	isProcessed, err3 := processed.IsProcessed(context.Background(), eventID)
	require.NoError(t, err3)
	assert.False(t, isProcessed)
}
