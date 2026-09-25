package entities

import (
	"time"

	"github.com/google/uuid"
)

// ProcessingJobStatus is the lifecycle status of a video processing job.
type ProcessingJobStatus string

const (
	StatusRunning   ProcessingJobStatus = "RUNNING"
	StatusCompleted ProcessingJobStatus = "COMPLETED"
	StatusFailed    ProcessingJobStatus = "FAILED"
)

// ProcessingJob is the aggregate that tracks the state of a single
// "extract frames from this video" job, from download through zip upload.
type ProcessingJob struct {
	ID                   uuid.UUID
	VideoID              uuid.UUID
	UserID               uuid.UUID
	SourceBucket         string
	SourceObjectKey      string
	Status               ProcessingJobStatus
	FrameIntervalSeconds int
	FrameCount           *int
	ZipBucket            *string
	ZipObjectKey         *string
	ErrorMessage         *string
	StartedAt            *time.Time
	CompletedAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
