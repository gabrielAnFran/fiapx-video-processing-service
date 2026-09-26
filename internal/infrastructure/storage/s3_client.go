// Package storage provides a thin MinIO/S3-compatible object storage client
// built on aws-sdk-go-v2, used to download source videos and upload the
// resulting frame zip archives.
package storage

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps an s3.Client configured for a MinIO (or any S3-compatible)
// endpoint, scoped to a single bucket.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client builds an S3Client pointed at a MinIO-compatible endpoint using
// static credentials and path-style addressing, which MinIO requires (it does
// not support the virtual-hosted-style bucket addressing that is the AWS S3
// default).
func NewS3Client(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*S3Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	scheme := "http"
	if useSSL {
		scheme = "https"
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(scheme + "://" + endpoint)
	})

	return &S3Client{client: client, bucket: bucket}, nil
}

// Download fetches the object at key and returns its body for the caller to
// stream into a local file. The caller is responsible for closing it.
func (c *S3Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// Upload streams body to key using a multipart-safe uploader, suitable for
// large zip archives.
func (c *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	uploader := manager.NewUploader(c.client)          //nolint:staticcheck // feature/s3/transfermanager migration is a separate, larger change
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{ //nolint:staticcheck // same as above
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	return err
}
