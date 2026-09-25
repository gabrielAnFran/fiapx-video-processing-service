package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/repositories"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/messaging"
	"github.com/google/uuid"
)

const (
	eventVideoProcessingCompleted = "video.processing.completed"
	eventVideoProcessingFailed    = "video.processing.failed"

	errCodeFFmpegFailed = "FFMPEG_FAILED"

	// maxErrorMessageLen bounds how much of ffmpeg's stderr output we persist
	// and publish; ffmpeg diagnostics can be verbose and the field is meant
	// for a human glancing at a failure, not a full log dump.
	maxErrorMessageLen = 2000
)

// Storage is the subset of storage.S3Client behavior this use case needs, so
// tests can fake it without touching MinIO.
type Storage interface {
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Upload(ctx context.Context, key string, body io.Reader, contentType string) error
}

// Extractor is the subset of the ffmpeg package's behavior this use case
// needs, so tests can fake it without invoking a real ffmpeg binary.
type Extractor interface {
	ExtractFrames(ctx context.Context, inputPath, outputDir string, intervalSeconds int) error
	CountFrames(outputDir string) (int, error)
	ZipDirectory(dir, destZipPath string) error
}

type ProcessVideoUseCase struct {
	jobs            repositories.ProcessingJobRepository
	processedEvents repositories.ProcessedEventRepository
	storage         Storage
	extractor       Extractor
	zipBucket       string
	ffmpegTimeout   time.Duration
}

func NewProcessVideoUseCase(
	jobs repositories.ProcessingJobRepository,
	processedEvents repositories.ProcessedEventRepository,
	storage Storage,
	extractor Extractor,
	zipBucket string,
	ffmpegTimeout time.Duration,
) *ProcessVideoUseCase {
	return &ProcessVideoUseCase{
		jobs:            jobs,
		processedEvents: processedEvents,
		storage:         storage,
		extractor:       extractor,
		zipBucket:       zipBucket,
		ffmpegTimeout:   ffmpegTimeout,
	}
}

type processVideoCommandPayload struct {
	VideoID              string `json:"video_id"`
	UserID               string `json:"user_id"`
	SagaID               string `json:"saga_id"`
	SourceBucket         string `json:"source_bucket"`
	SourceObjectKey      string `json:"source_object_key"`
	FrameIntervalSeconds int    `json:"frame_interval_seconds"`
}

type processingFailedPayload struct {
	VideoID      string `json:"video_id"`
	UserID       string `json:"user_id"`
	JobID        string `json:"job_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	FailedAt     string `json:"failed_at"`
}

type processingCompletedPayload struct {
	VideoID      string `json:"video_id"`
	UserID       string `json:"user_id"`
	JobID        string `json:"job_id"`
	ZipBucket    string `json:"zip_bucket"`
	ZipObjectKey string `json:"zip_object_key"`
	FrameCount   int    `json:"frame_count"`
	CompletedAt  string `json:"completed_at"`
}

// Handle processes a video.process.requested command, idempotently: it
// checks the processed_events table before doing any work, and marks the
// event as processed once a terminal outcome (success or ffmpeg failure) has
// been durably recorded.
//
// Only infrastructure errors (DB down, MinIO unreachable, malformed payload)
// are returned as errors, so the AMQP consumer retries them via the
// retry/DLQ mechanism in messaging.Conn. A failure in ffmpeg itself, on the
// video's own content, is a deterministic and terminal business outcome:
// it is reported via a video.processing.failed event and the handler
// returns nil, since redelivering the command would not change the result.
func (uc *ProcessVideoUseCase) Handle(ctx context.Context, ev messaging.Event) error {
	eventID, err := uuid.Parse(ev.EventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	processed, err := uc.processedEvents.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("check processed: %w", err)
	}
	if processed {
		return nil
	}

	var cmd processVideoCommandPayload
	if err := json.Unmarshal(ev.Payload, &cmd); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	videoID, err := uuid.Parse(cmd.VideoID)
	if err != nil {
		return fmt.Errorf("invalid video_id: %w", err)
	}
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now().UTC()
	job := &entities.ProcessingJob{
		ID:                   uuid.New(),
		VideoID:              videoID,
		UserID:               userID,
		SourceBucket:         cmd.SourceBucket,
		SourceObjectKey:      cmd.SourceObjectKey,
		Status:               entities.StatusRunning,
		FrameIntervalSeconds: cmd.FrameIntervalSeconds,
		StartedAt:            &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	// Standalone write, no outbox event: this is this service's own audit
	// state, not a cross-service fact that other services need to react to.
	if err := uc.jobs.Save(ctx, job, nil); err != nil {
		return fmt.Errorf("save running job: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "video-"+videoID.String())
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "original"+filepath.Ext(cmd.SourceObjectKey))
	if err := uc.downloadTo(ctx, cmd.SourceObjectKey, inputPath); err != nil {
		return fmt.Errorf("download source video: %w", err)
	}

	extractCtx, cancel := context.WithTimeout(ctx, uc.ffmpegTimeout)
	defer cancel()

	if err := uc.extractor.ExtractFrames(extractCtx, inputPath, tmpDir, cmd.FrameIntervalSeconds); err != nil {
		return uc.handleFFmpegFailure(ctx, eventID, ev, job, err)
	}

	return uc.handleSuccess(ctx, eventID, ev, job, tmpDir)
}

func (uc *ProcessVideoUseCase) downloadTo(ctx context.Context, key, destPath string) error {
	rc, err := uc.storage.Download(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, rc)
	return err
}

func (uc *ProcessVideoUseCase) handleFFmpegFailure(ctx context.Context, eventID uuid.UUID, ev messaging.Event, job *entities.ProcessingJob, ffmpegErr error) error {
	msg := ffmpegErr.Error()
	if len(msg) > maxErrorMessageLen {
		msg = msg[:maxErrorMessageLen]
	}

	now := time.Now().UTC()
	job.Status = entities.StatusFailed
	job.ErrorMessage = &msg
	job.CompletedAt = &now
	job.UpdatedAt = now

	payload := processingFailedPayload{
		VideoID:      job.VideoID.String(),
		UserID:       job.UserID.String(),
		JobID:        job.ID.String(),
		ErrorCode:    errCodeFFmpegFailed,
		ErrorMessage: msg,
		FailedAt:     now.Format(time.RFC3339Nano),
	}
	outEv, err := messaging.NewEvent(eventVideoProcessingFailed, ev.CorrelationID, ev.SagaID, payload)
	if err != nil {
		return fmt.Errorf("build failed event: %w", err)
	}
	envelope, err := marshalEnvelope(outEv)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.MustParse(outEv.EventID),
		AggregateID: job.VideoID,
		EventName:   outEv.EventName,
		Payload:     envelope,
		Headers:     []byte(`{"content-type":"application/json"}`),
	}

	if err := uc.jobs.Save(ctx, job, outboxEvent); err != nil {
		return fmt.Errorf("save failed job: %w", err)
	}
	if err := uc.processedEvents.MarkProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

func (uc *ProcessVideoUseCase) handleSuccess(ctx context.Context, eventID uuid.UUID, ev messaging.Event, job *entities.ProcessingJob, tmpDir string) error {
	frameCount, err := uc.extractor.CountFrames(tmpDir)
	if err != nil {
		return fmt.Errorf("count frames: %w", err)
	}

	zipPath := filepath.Join(tmpDir, "frames.zip")
	if err := uc.extractor.ZipDirectory(tmpDir, zipPath); err != nil {
		return fmt.Errorf("zip frames: %w", err)
	}

	zipFile, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zipFile.Close()

	zipObjectKey := fmt.Sprintf("processed/%s/%s/frames.zip", job.UserID.String(), job.VideoID.String())
	if err := uc.storage.Upload(ctx, zipObjectKey, zipFile, "application/zip"); err != nil {
		return fmt.Errorf("upload zip: %w", err)
	}

	now := time.Now().UTC()
	zipBucket := uc.zipBucket
	job.Status = entities.StatusCompleted
	job.FrameCount = &frameCount
	job.ZipBucket = &zipBucket
	job.ZipObjectKey = &zipObjectKey
	job.CompletedAt = &now
	job.UpdatedAt = now

	payload := processingCompletedPayload{
		VideoID:      job.VideoID.String(),
		UserID:       job.UserID.String(),
		JobID:        job.ID.String(),
		ZipBucket:    zipBucket,
		ZipObjectKey: zipObjectKey,
		FrameCount:   frameCount,
		CompletedAt:  now.Format(time.RFC3339Nano),
	}
	outEv, err := messaging.NewEvent(eventVideoProcessingCompleted, ev.CorrelationID, ev.SagaID, payload)
	if err != nil {
		return fmt.Errorf("build completed event: %w", err)
	}
	envelope, err := marshalEnvelope(outEv)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	outboxEvent := &repositories.OutboxEvent{
		EventID:     uuid.MustParse(outEv.EventID),
		AggregateID: job.VideoID,
		EventName:   outEv.EventName,
		Payload:     envelope,
		Headers:     []byte(`{"content-type":"application/json"}`),
	}

	if err := uc.jobs.Save(ctx, job, outboxEvent); err != nil {
		return fmt.Errorf("save completed job: %w", err)
	}
	if err := uc.processedEvents.MarkProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}
