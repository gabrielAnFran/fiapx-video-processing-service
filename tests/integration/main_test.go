//go:build integration

// Package integration holds real testcontainers-go integration tests
// (Postgres, RabbitMQ, and MinIO) exercising this service's GORM
// repository, messaging.Conn helper, S3Client, and ffmpeg.Adapter against
// real dependencies. Kept behind the `integration` build tag so
// `go test ./...` (no tags) stays fast and dependency-free.
package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	infradb "github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/db"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// Shared test fixtures, populated once in TestMain and reused across every
// test in this package (Postgres, RabbitMQ, and MinIO containers are each
// started exactly once for the whole suite).
var (
	testDSN            string
	testAMQPURL        string
	testMinIOEndpoint  string
	testMinIOAccessKey = "minioadmin"
	testMinIOSecretKey = "minioadmin"
	testMinIOBucket    = "video-processing-test"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, dsn, err := startPostgres(ctx)
	if err != nil {
		log.Fatalf("start postgres: %v", err)
	}
	testDSN = dsn

	amqpContainer, amqpURL, err := startRabbitMQ(ctx)
	if err != nil {
		log.Fatalf("start rabbitmq: %v", err)
	}
	testAMQPURL = amqpURL

	minioContainer, minioEndpoint, err := startMinIO(ctx)
	if err != nil {
		log.Fatalf("start minio: %v", err)
	}
	testMinIOEndpoint = minioEndpoint

	if err := runMigrations(testDSN); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := createBucket(ctx, testMinIOEndpoint, testMinIOAccessKey, testMinIOSecretKey, testMinIOBucket); err != nil {
		log.Fatalf("create minio bucket: %v", err)
	}

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	_ = amqpContainer.Terminate(ctx)
	_ = minioContainer.Terminate(ctx)
	os.Exit(code)
}

func startPostgres(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "processing_service",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return nil, "", err
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		return nil, "", err
	}

	dsn := fmt.Sprintf("host=%s user=postgres password=postgres dbname=processing_service port=%s sslmode=disable", host, port.Port())
	return c, dsn, nil
}

func startRabbitMQ(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:3-management-alpine",
		ExposedPorts: []string{"5672/tcp"},
		WaitingFor:   wait.ForListeningPort("5672/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return nil, "", err
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := c.MappedPort(ctx, "5672")
	if err != nil {
		return nil, "", err
	}

	return c, fmt.Sprintf("amqp://guest:guest@%s:%s/", host, port.Port()), nil
}

// startMinIO uses bitnamilegacy/minio rather than the upstream minio/minio
// image: MinIO went source-only on Docker Hub and quay.io on 2025-10-15, so
// the official image can no longer be pulled. bitnamilegacy/minio ships the
// same upstream server binary, honors MINIO_ROOT_USER/MINIO_ROOT_PASSWORD,
// and exposes a working health check at /minio/health/live on port 9000 via
// its own entrypoint scripts (no command override needed).
func startMinIO(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "bitnamilegacy/minio",
		ExposedPorts: []string{"9000/tcp", "9001/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return nil, "", err
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := c.MappedPort(ctx, "9000")
	if err != nil {
		return nil, "", err
	}

	return c, fmt.Sprintf("%s:%s", host, port.Port()), nil
}

func runMigrations(dsn string) error {
	dbConn, err := infradb.Connect(dsn)
	if err != nil {
		return err
	}
	sqlDB, err := dbConn.DB()
	if err != nil {
		return err
	}
	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// createBucket makes the test bucket using a plain aws-sdk-go-v2 S3 client
// (rather than storage.S3Client, which has no bucket-management method).
func createBucket(ctx context.Context, endpoint, accessKey, secretKey, bucket string) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String("http://" + endpoint)
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	return err
}

// newTestDB opens a fresh GORM connection to the shared test Postgres container.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := infradb.Connect(testDSN)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	return db
}

// truncateAll clears every table so each test starts from a clean slate
// despite sharing one Postgres container across the whole package.
func truncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, tbl := range []string{"processed_events", "outbox", "processing_jobs"} {
		if err := db.Exec("TRUNCATE TABLE " + tbl + " CASCADE").Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}
