package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrOrderFieldsRequired = errors.New("order_id and user_id are required")
	ErrInvalidTotalAmount  = errors.New("total_amount must be positive")
)

type AnalyticsService struct {
	receipts ReceiptStorage
	summary  SummaryRepository
}

func NewAnalyticsService(receipts ReceiptStorage, summary SummaryRepository) *AnalyticsService {
	return &AnalyticsService{receipts: receipts, summary: summary}
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

	exists, err := s.receipts.Exists(ctx, event.OrderID)
	if err != nil {
		return fmt.Errorf("check receipt exists: %w", err)
	}
	if !exists {
		receipt := Receipt(event)
		if receipt.Status == "" {
			receipt.Status = "CONFIRMED"
		}
		if err := s.receipts.Save(ctx, receipt); err != nil {
			return fmt.Errorf("save receipt: %w", err)
		}
	}

	alreadyProcessed, err := s.summary.RecordOrder(ctx, event.OrderID, event.TotalAmount, finalizedAt)
	if err != nil {
		return fmt.Errorf("record summary: %w", err)
	}
	if alreadyProcessed {
		return nil
	}
	return nil
}
