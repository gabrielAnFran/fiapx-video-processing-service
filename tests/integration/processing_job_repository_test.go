//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/repositories"
	infradb "github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newJob() *entities.ProcessingJob {
	now := time.Now().UTC()
	return &entities.ProcessingJob{
		ID:                   uuid.New(),
		VideoID:              uuid.New(),
		UserID:               uuid.New(),
		SourceBucket:         "videos",
		SourceObjectKey:      "raw/some-video.mp4",
		Status:               entities.StatusRunning,
		FrameIntervalSeconds: 2,
		StartedAt:            &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestProcessingJobRepository_Save_And_FindByVideoID(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	job := newJob()
	require.NoError(t, repo.Save(ctx, job, nil))

	found, err := repo.FindByVideoID(ctx, job.VideoID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, found.ID)
	assert.Equal(t, job.VideoID, found.VideoID)
	assert.Equal(t, job.UserID, found.UserID)
	assert.Equal(t, job.SourceBucket, found.SourceBucket)
	assert.Equal(t, job.SourceObjectKey, found.SourceObjectKey)
	assert.Equal(t, entities.StatusRunning, found.Status)
	assert.Equal(t, job.FrameIntervalSeconds, found.FrameIntervalSeconds)
	assert.Nil(t, found.FrameCount)
	assert.Nil(t, found.ZipBucket)
	assert.Nil(t, found.ZipObjectKey)
}

func TestProcessingJobRepository_Save_UpsertsOnConflict(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	job := newJob()
	require.NoError(t, repo.Save(ctx, job, nil))

	frameCount := 42
	zipBucket := "zips"
	zipKey := "processed/user/video/frames.zip"
	completedAt := time.Now().UTC()
	job.Status = entities.StatusCompleted
	job.FrameCount = &frameCount
	job.ZipBucket = &zipBucket
	job.ZipObjectKey = &zipKey
	job.CompletedAt = &completedAt
	job.UpdatedAt = completedAt

	require.NoError(t, repo.Save(ctx, job, nil))

	found, err := repo.FindByVideoID(ctx, job.VideoID)
	require.NoError(t, err)
	assert.Equal(t, entities.StatusCompleted, found.Status)
	require.NotNil(t, found.FrameCount)
	assert.Equal(t, frameCount, *found.FrameCount)
	require.NotNil(t, found.ZipBucket)
	assert.Equal(t, zipBucket, *found.ZipBucket)
	require.NotNil(t, found.ZipObjectKey)
	assert.Equal(t, zipKey, *found.ZipObjectKey)
	require.NotNil(t, found.CompletedAt)
}

func TestProcessingJobRepository_Save_WithOutboxEvent_WritesBothInSameTransaction(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	job := newJob()
	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.New(),
		AggregateID: job.VideoID,
		EventName:   "video.processing.completed",
		Payload:     []byte(`{"video_id":"` + job.VideoID.String() + `"}`),
		Headers:     []byte(`{"content-type":"application/json"}`),
	}
	require.NoError(t, repo.Save(ctx, job, outboxEvent))

	rows, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, outboxEvent.EventID, rows[0].EventID)
	assert.Equal(t, outboxEvent.EventName, rows[0].EventName)
}

func TestProcessingJobRepository_FindByVideoID_NotFound(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)

	_, err := repo.FindByVideoID(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, repositories.ErrNotFound)
}

func TestProcessingJobRepository_FetchUnpublished_And_MarkPublished(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	job1 := newJob()
	ev1 := &repositories.OutboxEvent{
		EventID:     uuid.New(),
		AggregateID: job1.VideoID,
		EventName:   "video.processing.completed",
		Payload:     []byte(`{}`),
		Headers:     []byte(`{}`),
	}
	require.NoError(t, repo.Save(ctx, job1, ev1))

	job2 := newJob()
	ev2 := &repositories.OutboxEvent{
		EventID:     uuid.New(),
		AggregateID: job2.VideoID,
		EventName:   "video.processing.failed",
		Payload:     []byte(`{}`),
		Headers:     []byte(`{}`),
	}
	require.NoError(t, repo.Save(ctx, job2, ev2))

	rows, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	require.NoError(t, repo.MarkPublished(ctx, ids))

	remaining, err := repo.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestProcessingJobRepository_MarkPublished_EmptyIDs_NoOp(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)

	require.NoError(t, repo.MarkPublished(context.Background(), []int64{}))
}

func TestProcessingJobRepository_FetchUnpublished_RespectsBatchLimit(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		job := newJob()
		ev := &repositories.OutboxEvent{
			EventID:     uuid.New(),
			AggregateID: job.VideoID,
			EventName:   "video.processing.completed",
			Payload:     []byte(`{}`),
			Headers:     []byte(`{}`),
		}
		require.NoError(t, repo.Save(ctx, job, ev))
	}

	rows, err := repo.FetchUnpublished(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestProcessingJobRepository_IsProcessed_And_MarkProcessed(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	eventID := uuid.New()

	processed, err := repo.IsProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.False(t, processed)

	require.NoError(t, repo.MarkProcessed(ctx, eventID))

	processed, err = repo.IsProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestProcessingJobRepository_MarkProcessed_IdempotentOnConflict(t *testing.T) {
	db := newTestDB(t)
	truncateAll(t, db)
	repo := infradb.NewProcessingJobRepository(db)
	ctx := context.Background()

	eventID := uuid.New()

	require.NoError(t, repo.MarkProcessed(ctx, eventID))
	// Marking the same event twice must not error (ON CONFLICT DO NOTHING).
	require.NoError(t, repo.MarkProcessed(ctx, eventID))

	processed, err := repo.IsProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, processed)
}
