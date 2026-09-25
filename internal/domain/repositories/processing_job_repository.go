package repositories

import (
	"context"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/google/uuid"
)

// OutboxEvent is the write-side shape handed to ProcessingJobRepository.Save
// so the domain write and the outbox row land in the same transaction.
type OutboxEvent struct {
	EventID     uuid.UUID
	AggregateID uuid.UUID
	EventName   string
	Payload     []byte
	Headers     []byte
}

type ProcessingJobRepository interface {
	// Save persists/updates the job row and, when outboxEvent is non-nil, writes
	// it to the outbox in the same transaction.
	Save(ctx context.Context, job *entities.ProcessingJob, outboxEvent *OutboxEvent) error
	FindByVideoID(ctx context.Context, videoID uuid.UUID) (*entities.ProcessingJob, error)
}

type OutboxRepository interface {
	FetchUnpublished(ctx context.Context, batch int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, ids []int64) error
}

type OutboxRow struct {
	ID        int64
	EventID   uuid.UUID
	EventName string
	Payload   []byte
	Headers   []byte
}

type ProcessedEventRepository interface {
	IsProcessed(ctx context.Context, eventID uuid.UUID) (bool, error)
	MarkProcessed(ctx context.Context, eventID uuid.UUID) error
}
