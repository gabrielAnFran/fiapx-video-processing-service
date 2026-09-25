package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/repositories"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type processingJobModel struct {
	ID                   uuid.UUID  `gorm:"column:id;primaryKey"`
	VideoID              uuid.UUID  `gorm:"column:video_id"`
	UserID               uuid.UUID  `gorm:"column:user_id"`
	SourceBucket         string     `gorm:"column:source_bucket"`
	SourceObjectKey      string     `gorm:"column:source_object_key"`
	Status               string     `gorm:"column:status"`
	FrameIntervalSeconds int        `gorm:"column:frame_interval_seconds"`
	FrameCount           *int       `gorm:"column:frame_count"`
	ZipBucket            *string    `gorm:"column:zip_bucket"`
	ZipObjectKey         *string    `gorm:"column:zip_object_key"`
	ErrorMessage         *string    `gorm:"column:error_message"`
	StartedAt            *time.Time `gorm:"column:started_at"`
	CompletedAt          *time.Time `gorm:"column:completed_at"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

func (processingJobModel) TableName() string { return "processing_jobs" }

type outboxModel struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	EventID     uuid.UUID  `gorm:"column:event_id"`
	AggregateID uuid.UUID  `gorm:"column:aggregate_id"`
	EventName   string     `gorm:"column:event_name"`
	Payload     []byte     `gorm:"column:payload;type:jsonb"`
	Headers     []byte     `gorm:"column:headers;type:jsonb"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	PublishedAt *time.Time `gorm:"column:published_at"`
}

func (outboxModel) TableName() string { return "outbox" }

type processedEventModel struct {
	EventID     uuid.UUID `gorm:"column:event_id;primaryKey"`
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

func (processedEventModel) TableName() string { return "processed_events" }

// ProcessingJobRepository is a GORM-backed implementation of the domain
// repository interfaces defined in internal/domain/repositories.
type ProcessingJobRepository struct {
	db *gorm.DB
}

func NewProcessingJobRepository(db *gorm.DB) *ProcessingJobRepository {
	return &ProcessingJobRepository{db: db}
}

func toProcessingJobModel(j *entities.ProcessingJob) processingJobModel {
	return processingJobModel{
		ID:                   j.ID,
		VideoID:              j.VideoID,
		UserID:               j.UserID,
		SourceBucket:         j.SourceBucket,
		SourceObjectKey:      j.SourceObjectKey,
		Status:               string(j.Status),
		FrameIntervalSeconds: j.FrameIntervalSeconds,
		FrameCount:           j.FrameCount,
		ZipBucket:            j.ZipBucket,
		ZipObjectKey:         j.ZipObjectKey,
		ErrorMessage:         j.ErrorMessage,
		StartedAt:            j.StartedAt,
		CompletedAt:          j.CompletedAt,
		CreatedAt:            j.CreatedAt,
		UpdatedAt:            j.UpdatedAt,
	}
}

func fromProcessingJobModel(m processingJobModel) entities.ProcessingJob {
	return entities.ProcessingJob{
		ID:                   m.ID,
		VideoID:              m.VideoID,
		UserID:               m.UserID,
		SourceBucket:         m.SourceBucket,
		SourceObjectKey:      m.SourceObjectKey,
		Status:               entities.ProcessingJobStatus(m.Status),
		FrameIntervalSeconds: m.FrameIntervalSeconds,
		FrameCount:           m.FrameCount,
		ZipBucket:            m.ZipBucket,
		ZipObjectKey:         m.ZipObjectKey,
		ErrorMessage:         m.ErrorMessage,
		StartedAt:            m.StartedAt,
		CompletedAt:          m.CompletedAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

func (r *ProcessingJobRepository) Save(ctx context.Context, job *entities.ProcessingJob, outboxEvent *repositories.OutboxEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := toProcessingJobModel(job)
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"video_id", "user_id", "source_bucket", "source_object_key", "status",
				"frame_interval_seconds", "frame_count", "zip_bucket", "zip_object_key",
				"error_message", "started_at", "completed_at", "updated_at",
			}),
		}).Create(&m).Error; err != nil {
			return fmt.Errorf("save processing job: %w", err)
		}

		if outboxEvent != nil {
			om := outboxModel{
				EventID:     outboxEvent.EventID,
				AggregateID: outboxEvent.AggregateID,
				EventName:   outboxEvent.EventName,
				Payload:     outboxEvent.Payload,
				Headers:     outboxEvent.Headers,
				CreatedAt:   time.Now().UTC(),
			}
			if err := tx.Create(&om).Error; err != nil {
				return fmt.Errorf("save outbox event: %w", err)
			}
		}
		return nil
	})
}

func (r *ProcessingJobRepository) FindByVideoID(ctx context.Context, videoID uuid.UUID) (*entities.ProcessingJob, error) {
	var m processingJobModel
	if err := r.db.WithContext(ctx).First(&m, "video_id = ?", videoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repositories.ErrNotFound
		}
		return nil, err
	}
	j := fromProcessingJobModel(m)
	return &j, nil
}

// OutboxRepository implementation.

func (r *ProcessingJobRepository) FetchUnpublished(ctx context.Context, batch int) ([]repositories.OutboxRow, error) {
	var rows []outboxModel
	if err := r.db.WithContext(ctx).Where("published_at IS NULL").Order("created_at ASC").Limit(batch).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]repositories.OutboxRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, repositories.OutboxRow{
			ID:        m.ID,
			EventID:   m.EventID,
			EventName: m.EventName,
			Payload:   m.Payload,
			Headers:   m.Headers,
		})
	}
	return out, nil
}

func (r *ProcessingJobRepository) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&outboxModel{}).Where("id IN ?", ids).Update("published_at", now).Error
}

// ProcessedEventRepository implementation.

func (r *ProcessingJobRepository) IsProcessed(ctx context.Context, eventID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&processedEventModel{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ProcessingJobRepository) MarkProcessed(ctx context.Context, eventID uuid.UUID) error {
	m := processedEventModel{EventID: eventID, ProcessedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error
}
