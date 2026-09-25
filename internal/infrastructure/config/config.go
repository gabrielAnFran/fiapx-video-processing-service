package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	DBDSN              string
	AMQPURL            string
	DispatchIntervalMS int
	MinioEndpoint      string
	MinioAccessKey     string
	MinioSecretKey     string
	MinioBucket        string
	MinioUseSSL        bool
	FrameIntervalSec   int
	FFmpegTimeout      time.Duration
}

func Load() Config {
	return Config{
		Port:               getEnv("PROCESSING_PORT", "8082"),
		DBDSN:              getEnv("PROCESSING_DB_DSN", "host=localhost user=postgres password=postgres dbname=processing_service port=5432 sslmode=disable"),
		AMQPURL:            getEnv("PROCESSING_AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		DispatchIntervalMS: getEnvInt("PROCESSING_DISPATCH_INTERVAL_MS", 500),
		MinioEndpoint:      getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey:     getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:        getEnv("MINIO_BUCKET", "fiapx-videos"),
		MinioUseSSL:        getEnvBool("MINIO_USE_SSL", false),
		FrameIntervalSec:   getEnvInt("PROCESSING_FRAME_INTERVAL_SECONDS", 1),
		FFmpegTimeout:      time.Duration(getEnvInt("PROCESSING_FFMPEG_TIMEOUT_SECONDS", 600)) * time.Second,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
