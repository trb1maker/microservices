package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrOrderFieldsRequired = errors.New("order_id and user_id are required")
	ErrInvalidTotalAmount  = errors.New("total_amount must be positive")
	ErrReceiptNotFound     = errors.New("receipt not found")
	ErrReceiptForbidden    = errors.New("receipt access denied")
	ErrSearchQueryRequired = errors.New("search query is required")
)

const (
	DefaultReceiptURLTTL = 15 * time.Minute
	MaxSearchLimit       = 100
)

type AnalyticsService struct {
	receipts  ReceiptStorage
	summary   SummaryRepository
	documents DocumentRepository
	urlTTL    time.Duration
}

func NewAnalyticsService(
	receipts ReceiptStorage,
	summary SummaryRepository,
	documents DocumentRepository,
	urlTTL time.Duration,
) *AnalyticsService {
	if urlTTL <= 0 {
		urlTTL = DefaultReceiptURLTTL
	}
	return &AnalyticsService{
		receipts:  receipts,
		summary:   summary,
		documents: documents,
		urlTTL:    urlTTL,
	}
}

func (s *AnalyticsService) ProcessOrderFinalized(ctx context.Context, event OrderFinalized) error {
	if event.OrderID == "" || event.UserID == "" {
		return ErrOrderFieldsRequired
	}
	if event.TotalAmount <= 0 {
		return ErrInvalidTotalAmount
	}

	finalizedAt, err := time.Parse(time.RFC3339, event.FinalizedAt)
	if err != nil {
		return fmt.Errorf("parse finalized_at: %w", err)
	}

	status := event.Status
	if status == "" {
		status = "CONFIRMED"
	}

	alreadyProcessed, err := s.summary.IsOrderProcessed(ctx, event.OrderID)
	if err != nil {
		return fmt.Errorf("check processed order: %w", err)
	}
	if alreadyProcessed {
		return nil
	}

	exists, err := s.receipts.Exists(ctx, event.OrderID)
	if err != nil {
		return fmt.Errorf("check receipt exists: %w", err)
	}
	if !exists {
		receipt := Receipt(event)
		receipt.Status = status
		if err := s.receipts.Save(ctx, receipt); err != nil {
			return fmt.Errorf("save receipt: %w", err)
		}
	}

	if err := s.documents.Upsert(ctx, ReceiptDocument{
		OrderID:         event.OrderID,
		UserID:          event.UserID,
		TotalAmount:     event.TotalAmount,
		Status:          status,
		FinalizedAt:     finalizedAt,
		DeliveryAddress: event.DeliveryAddress,
		SearchText:      buildSearchText(event),
	}); err != nil {
		return fmt.Errorf("index receipt document: %w", err)
	}

	if _, err := s.summary.RecordOrder(ctx, event.OrderID, event.TotalAmount, finalizedAt); err != nil {
		return fmt.Errorf("record summary: %w", err)
	}
	return nil
}

func (s *AnalyticsService) GetReceiptURL(ctx context.Context, userID, orderID string) (string, time.Duration, error) {
	doc, err := s.documents.GetByOrderID(ctx, orderID)
	if err != nil {
		return "", 0, fmt.Errorf("get receipt document: %w", err)
	}
	if doc == nil {
		return "", 0, ErrReceiptNotFound
	}
	if doc.UserID != userID {
		return "", 0, ErrReceiptNotFound
	}

	exists, err := s.receipts.Exists(ctx, orderID)
	if err != nil {
		return "", 0, fmt.Errorf("check receipt object: %w", err)
	}
	if !exists {
		return "", 0, ErrReceiptNotFound
	}

	url, err := s.receipts.PresignGet(ctx, orderID, s.urlTTL)
	if err != nil {
		return "", 0, fmt.Errorf("presign receipt: %w", err)
	}
	return url, s.urlTTL, nil
}

func (s *AnalyticsService) SearchReceipts(ctx context.Context, userID, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrSearchQueryRequired
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	results, err := s.documents.Search(ctx, userID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search receipts: %w", err)
	}
	return results, nil
}

func buildSearchText(event OrderFinalized) string {
	parts := []string{
		event.OrderID,
		event.UserID,
		event.DeliveryAddress,
		event.Status,
	}
	return strings.Join(parts, " ")
}
