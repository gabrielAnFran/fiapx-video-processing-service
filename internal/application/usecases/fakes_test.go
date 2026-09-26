package usecases

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/entities"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/domain/repositories"
	"github.com/google/uuid"
)

// fakeProcessingJobRepository is a simple in-memory implementation of
// repositories.ProcessingJobRepository for unit tests.
type fakeProcessingJobRepository struct {
	mu      sync.Mutex
	jobs    map[uuid.UUID]entities.ProcessingJob
	outbox  []repositories.OutboxEvent
	saveErr error
}

func newFakeProcessingJobRepository() *fakeProcessingJobRepository {
	return &fakeProcessingJobRepository{jobs: map[uuid.UUID]entities.ProcessingJob{}}
}

func (f *fakeProcessingJobRepository) Save(_ context.Context, job *entities.ProcessingJob, outboxEvent *repositories.OutboxEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.jobs[job.ID] = *job
	if outboxEvent != nil {
		f.outbox = append(f.outbox, *outboxEvent)
	}
	return nil
}

func (f *fakeProcessingJobRepository) FindByVideoID(_ context.Context, videoID uuid.UUID) (*entities.ProcessingJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, j := range f.jobs {
		if j.VideoID == videoID {
			jj := j
			return &jj, nil
		}
	}
	return nil, repositories.ErrNotFound
}

// fakeProcessedEventRepository is an in-memory ProcessedEventRepository.
type fakeProcessedEventRepository struct {
	mu        sync.Mutex
	processed map[uuid.UUID]bool
}

func newFakeProcessedEventRepository() *fakeProcessedEventRepository {
	return &fakeProcessedEventRepository{processed: map[uuid.UUID]bool{}}
}

func (f *fakeProcessedEventRepository) IsProcessed(_ context.Context, eventID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.processed[eventID], nil
}

func (f *fakeProcessedEventRepository) MarkProcessed(_ context.Context, eventID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.processed[eventID] = true
	return nil
}

// fakeStorage is an in-memory Storage fake.
type fakeStorage struct {
	downloadData []byte
	downloadErr  error
	uploadErr    error

	mu           sync.Mutex
	uploadedKey  string
	uploadedBody []byte
}

func (f *fakeStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return io.NopCloser(bytes.NewReader(f.downloadData)), nil
}

func (f *fakeStorage) Upload(_ context.Context, key string, body io.Reader, _ string) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadedKey = key
	f.uploadedBody = b
	return nil
}

// fakeExtractor is an in-memory Extractor fake. ZipDirectory writes a real
// (fake-content) file at destZipPath so the use case's os.Open succeeds.
type fakeExtractor struct {
	extractErr error
	frameCount int
	countErr   error
	zipErr     error

	mu           sync.Mutex
	extractCalls int
}

func (f *fakeExtractor) ExtractFrames(_ context.Context, _, _ string, _ int) error {
	f.mu.Lock()
	f.extractCalls++
	f.mu.Unlock()
	return f.extractErr
}

func (f *fakeExtractor) CountFrames(_ string) (int, error) {
	return f.frameCount, f.countErr
}

func (f *fakeExtractor) ZipDirectory(_, destZipPath string) error {
	if f.zipErr != nil {
		return f.zipErr
	}
	return os.WriteFile(destZipPath, []byte("fake zip content"), 0o600)
}
