package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOClient struct {
	client     *minio.Client
	bucket     string
	publicHost string
}

func NewMinIOClient(endpoint, accessKey, secretKey, bucket string, useSSl bool) (*MinIOClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSl,
	})
	if err != nil {
		return nil, fmt.Errorf("failed connect to MinIO:%w", err)
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("error examination bucket:%w", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{
			Region: "us-east-1",
		})
		if err != nil {
			return nil, fmt.Errorf("error create bucket:%w", err)
		}
	}
	return &MinIOClient{
		client:     client,
		bucket:     bucket,
		publicHost: fmt.Sprintf("http://%s", endpoint),
	}, nil
}

func (m *MinIOClient) GenerateObjectKey(userID, filename string) string {
	now := time.Now()
	randomID := uuid.New().String()

	ext := filepath.Ext(filename)

	return fmt.Sprintf("users/%s/%d/%02d/%s%s",
		userID,
		now.Year(),
		now.Month(),
		randomID,
		ext,
	)
}
func (m *MinIOClient) PresignedPutURL(ctx context.Context, objectKey string, expire time.Duration) (string, error) {
	u, err := m.client.PresignedPutObject(ctx, m.bucket, objectKey, expire)
	if err != nil {
		return "", fmt.Errorf("error generation presigned URL: %w", err)
	}
	return u.String(), nil
}

func (m *MinIOClient) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	u, err := m.client.PresignedGetObject(ctx, m.bucket, objectKey, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("error generation download URL: %w", err)
	}
	return u.String(), nil
}

func (m *MinIOClient) DeleteObject(ctx context.Context, objectKey string) error {
	err := m.client.RemoveObject(ctx, m.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("error deleted file URL: %s: %w", objectKey, err)
	}
	return nil
}
