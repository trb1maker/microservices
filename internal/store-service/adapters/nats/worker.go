package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

// Worker listens to NATS commands and delegates to StoreService.
type Worker struct {
	svc  *app.StoreService
	subs []*nats.Subscription
}

// NewWorker creates a new NATS Worker.
func NewWorker(svc *app.StoreService) *Worker {
	return &Worker{svc: svc}
}

// ReserveItemsMessage is the expected payload for RESERVE_ITEMS.
type ReserveItemsMessage struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleReserveItems processes RESERVE_ITEMS commands.
func (w *Worker) HandleReserveItems(msg *nats.Msg) {
	var req ReserveItemsMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal reserve items request", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	err := w.svc.ReserveItems(ctx, app.ReserveItemsRequest{
		OrderID:   msg.Header.Get("X-Order-ID"),
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		slog.Error("reserve items failed",
			slog.String("product_id", req.ProductID),
			slog.Any("error", err),
		)
	}
}

// ConfirmOrderMessage is the expected payload for CONFIRM_ORDER.
type ConfirmOrderMessage struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleConfirmOrder processes CONFIRM_ORDER commands.
func (w *Worker) HandleConfirmOrder(msg *nats.Msg) {
	var req ConfirmOrderMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal confirm order request", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	err := w.svc.ConfirmOrder(ctx, app.ConfirmOrderRequest{
		OrderID:   req.OrderID,
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		slog.Error("confirm order failed",
			slog.String("order_id", req.OrderID),
			slog.Any("error", err),
		)
	}
}

// ReleaseReservationMessage is the expected payload for RELEASE_RESERVATION.
type ReleaseReservationMessage struct {
	UserID    string `json:"user_id"`
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleReleaseReservation processes RELEASE_RESERVATION commands.
func (w *Worker) HandleReleaseReservation(msg *nats.Msg) {
	var req ReleaseReservationMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal release reservation request", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	err := w.svc.ReleaseReservation(ctx, app.ReleaseReservationRequest{
		OrderID:   req.OrderID,
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		slog.Error("release reservation failed",
			slog.String("order_id", req.OrderID),
			slog.Any("error", err),
		)
	}
}

// SubscribeAll subscribes to all store command subjects.
func (w *Worker) SubscribeAll(nc *nats.Conn, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj string) error {
	subs := []struct {
		subject string
		handler nats.MsgHandler
	}{
		{reserveItemsSubj, w.HandleReserveItems},
		{confirmOrderSubj, w.HandleConfirmOrder},
		{releaseReservationSubj, w.HandleReleaseReservation},
	}

	for _, s := range subs {
		sub, err := nc.Subscribe(s.subject, s.handler)
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", s.subject, err)
		}
		w.subs = append(w.subs, sub)
		slog.Info("subscribed to NATS subject", slog.String("subject", s.subject))
	}

	return nil
}

func (w *Worker) ActiveSubscriptions() int {
	return len(w.subs)
}

func (w *Worker) Close() {
	for _, sub := range w.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
	w.subs = nil
}
