package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
)

var ErrBucketNotFound = errors.New("bucket not found")

type Storage struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

type Config struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
	PublicUseSSL   bool
}

func NewStorage(cfg Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	presignClient := client
	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint != "" && publicEndpoint != cfg.Endpoint {
		publicUseSSL := cfg.PublicUseSSL
		if !cfg.PublicUseSSL && cfg.UseSSL {
			publicUseSSL = cfg.UseSSL
		}
		presignClient, err = minio.New(publicEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: publicUseSSL,
		})
		if err != nil {
			return nil, fmt.Errorf("create minio presign client: %w", err)
		}
	}

	return &Storage{client: client, presignClient: presignClient, bucket: cfg.Bucket}, nil
}

func (s *Storage) objectKey(orderID string) string {
	return fmt.Sprintf("receipts/%s.json", orderID)
}

func (s *Storage) Ping(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrBucketNotFound, s.bucket)
	}
	return nil
}

func (s *Storage) Exists(ctx context.Context, orderID string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, s.objectKey(orderID), minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	errResp := minio.ToErrorResponse(err)
	if errResp.Code == "NoSuchKey" || errResp.Code == "NotFound" {
		return false, nil
	}
	return false, fmt.Errorf("stat object: %w", err)
}

func (s *Storage) Save(ctx context.Context, receipt app.Receipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}
	_, err = s.client.PutObject(
		ctx,
		s.bucket,
		s.objectKey(receipt.OrderID),
		bytes.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{ContentType: "application/json"},
	)
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *Storage) PresignGet(ctx context.Context, orderID string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = app.DefaultReceiptURLTTL
	}
	presigned, err := s.presignClient.PresignedGetObject(ctx, s.bucket, s.objectKey(orderID), expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign get object: %w", err)
	}
	return presigned.String(), nil
}

var _ app.ReceiptStorage = (*Storage)(nil)
