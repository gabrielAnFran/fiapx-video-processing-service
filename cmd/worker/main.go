package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"syscall"

	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/application/usecases"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/config"
	infradb "github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/db"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/ffmpeg"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/messaging"
	"github.com/gabrielAnFran/fiapx-video-processing-service/internal/infrastructure/storage"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"os/signal"
)

func runMigrations(sqlDB *sql.DB) error {
	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	gormDB, err := infradb.Connect(cfg.DBDSN)
	if err != nil {
		slog.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		slog.Error("failed to get sql.DB", "error", err)
		os.Exit(1)
	}
	if err := runMigrations(sqlDB); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	amqpConn, err := messaging.Dial(cfg.AMQPURL)
	if err != nil {
		slog.Error("failed to connect to amqp", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	if _, err := amqpConn.DeclareServiceQueue("processing-service", []string{"video.process.requested"}); err != nil {
		slog.Error("failed to declare service queue", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s3Client, err := storage.NewS3Client(ctx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		slog.Error("failed to create s3 client", "error", err)
		os.Exit(1)
	}

	jobRepo := infradb.NewProcessingJobRepository(gormDB)
	extractor := ffmpeg.NewAdapter()
	uc := usecases.NewProcessVideoUseCase(jobRepo, jobRepo, s3Client, extractor, cfg.MinioBucket, cfg.FFmpegTimeout)

	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("worker started")

	handler := func(ctx context.Context, ev messaging.Event) error {
		switch ev.EventName {
		case "video.process.requested":
			return uc.Handle(ctx, ev)
		default:
			slog.Warn("ignoring unhandled event", "event", ev.EventName)
			return nil
		}
	}

	if err := amqpConn.Consume(stopCtx, "processing-service", handler); err != nil && stopCtx.Err() == nil {
		slog.Error("consumer stopped with error", "error", err)
		os.Exit(1)
	}
	slog.Info("worker stopped")
}
