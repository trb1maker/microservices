package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
)

type Storage struct {
	client *minio.Client
	bucket string
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

func NewStorage(cfg Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *Storage) objectKey(orderID string) string {
	return fmt.Sprintf("receipts/%s.json", orderID)
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

var _ app.ReceiptStorage = (*Storage)(nil)
