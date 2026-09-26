package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PROCESSING_PORT", "")
	t.Setenv("PROCESSING_DB_DSN", "")
	t.Setenv("PROCESSING_AMQP_URL", "")
	t.Setenv("PROCESSING_DISPATCH_INTERVAL_MS", "")
	t.Setenv("MINIO_ENDPOINT", "")
	t.Setenv("MINIO_ACCESS_KEY", "")
	t.Setenv("MINIO_SECRET_KEY", "")
	t.Setenv("MINIO_BUCKET", "")
	t.Setenv("MINIO_USE_SSL", "")
	t.Setenv("PROCESSING_FRAME_INTERVAL_SECONDS", "")
	t.Setenv("PROCESSING_FFMPEG_TIMEOUT_SECONDS", "")

	cfg := Load()

	if cfg.Port != "8082" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8082")
	}
	if cfg.DBDSN != "host=localhost user=postgres password=postgres dbname=processing_service port=5432 sslmode=disable" {
		t.Errorf("unexpected default DBDSN: %q", cfg.DBDSN)
	}
	if cfg.AMQPURL != "amqp://guest:guest@localhost:5672/" { //nolint:gosec // default RabbitMQ dev credentials, not a real secret
		t.Errorf("unexpected default AMQPURL: %q", cfg.AMQPURL)
	}
	if cfg.DispatchIntervalMS != 500 {
		t.Errorf("DispatchIntervalMS = %d, want 500", cfg.DispatchIntervalMS)
	}
	if cfg.MinioEndpoint != "localhost:9000" {
		t.Errorf("MinioEndpoint = %q, want %q", cfg.MinioEndpoint, "localhost:9000")
	}
	if cfg.MinioAccessKey != "minioadmin" {
		t.Errorf("MinioAccessKey = %q, want %q", cfg.MinioAccessKey, "minioadmin")
	}
	if cfg.MinioSecretKey != "minioadmin" {
		t.Errorf("MinioSecretKey = %q, want %q", cfg.MinioSecretKey, "minioadmin")
	}
	if cfg.MinioBucket != "fiapx-videos" {
		t.Errorf("MinioBucket = %q, want %q", cfg.MinioBucket, "fiapx-videos")
	}
	if cfg.MinioUseSSL != false {
		t.Errorf("MinioUseSSL = %v, want false", cfg.MinioUseSSL)
	}
	if cfg.FrameIntervalSec != 1 {
		t.Errorf("FrameIntervalSec = %d, want 1", cfg.FrameIntervalSec)
	}
	if cfg.FFmpegTimeout.Seconds() != 600 {
		t.Errorf("FFmpegTimeout = %v, want 600s", cfg.FFmpegTimeout)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("PROCESSING_PORT", "9999")
	t.Setenv("PROCESSING_DB_DSN", "custom-dsn")
	t.Setenv("PROCESSING_AMQP_URL", "amqp://custom/")
	t.Setenv("PROCESSING_DISPATCH_INTERVAL_MS", "1500")
	t.Setenv("MINIO_ENDPOINT", "minio.internal:9000")
	t.Setenv("MINIO_ACCESS_KEY", "custom-access")
	t.Setenv("MINIO_SECRET_KEY", "custom-secret")
	t.Setenv("MINIO_BUCKET", "custom-bucket")
	t.Setenv("MINIO_USE_SSL", "true")
	t.Setenv("PROCESSING_FRAME_INTERVAL_SECONDS", "5")
	t.Setenv("PROCESSING_FFMPEG_TIMEOUT_SECONDS", "120")

	cfg := Load()

	if cfg.Port != "9999" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9999")
	}
	if cfg.DBDSN != "custom-dsn" {
		t.Errorf("DBDSN = %q, want %q", cfg.DBDSN, "custom-dsn")
	}
	if cfg.AMQPURL != "amqp://custom/" {
		t.Errorf("AMQPURL = %q, want %q", cfg.AMQPURL, "amqp://custom/")
	}
	if cfg.DispatchIntervalMS != 1500 {
		t.Errorf("DispatchIntervalMS = %d, want 1500", cfg.DispatchIntervalMS)
	}
	if cfg.MinioEndpoint != "minio.internal:9000" {
		t.Errorf("MinioEndpoint = %q, want %q", cfg.MinioEndpoint, "minio.internal:9000")
	}
	if cfg.MinioUseSSL != true {
		t.Errorf("MinioUseSSL = %v, want true", cfg.MinioUseSSL)
	}
	if cfg.FrameIntervalSec != 5 {
		t.Errorf("FrameIntervalSec = %d, want 5", cfg.FrameIntervalSec)
	}
	if cfg.FFmpegTimeout.Seconds() != 120 {
		t.Errorf("FFmpegTimeout = %v, want 120s", cfg.FFmpegTimeout)
	}
}

func TestLoad_InvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("PROCESSING_DISPATCH_INTERVAL_MS", "not-a-number")
	t.Setenv("PROCESSING_FRAME_INTERVAL_SECONDS", "not-a-number")

	cfg := Load()

	if cfg.DispatchIntervalMS != 500 {
		t.Errorf("DispatchIntervalMS = %d, want default 500 on parse error", cfg.DispatchIntervalMS)
	}
	if cfg.FrameIntervalSec != 1 {
		t.Errorf("FrameIntervalSec = %d, want default 1 on parse error", cfg.FrameIntervalSec)
	}
}

func TestLoad_InvalidBoolFallsBackToDefault(t *testing.T) {
	t.Setenv("MINIO_USE_SSL", "not-a-bool")

	cfg := Load()

	if cfg.MinioUseSSL != false {
		t.Errorf("MinioUseSSL = %v, want default false on parse error", cfg.MinioUseSSL)
	}
}
