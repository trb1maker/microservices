package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

// Worker listens to JetStream commands and delegates to StoreService.
type Worker struct {
	svc   *app.StoreService
	inbox natsx.Inbox
	subs  []*natsx.Subscription
}

// NewWorker creates a new NATS Worker.
func NewWorker(svc *app.StoreService) *Worker {
	return &Worker{svc: svc}
}

func (w *Worker) SetInbox(inbox natsx.Inbox) {
	w.inbox = inbox
}

func (w *Worker) Inbox() natsx.Inbox {
	return w.inbox
}

// ReserveItemsMessage is the expected payload for RESERVE_ITEMS.
type ReserveItemsMessage struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleReserveItems processes RESERVE_ITEMS commands.
func (w *Worker) HandleReserveItems(ctx context.Context, msg *nats.Msg) error {
	var req ReserveItemsMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal reserve items request", slog.Any("error", err))
		return nil
	}

	err := w.svc.ReserveItems(ctx, app.ReserveItemsRequest{
		OrderID:   msg.Header.Get(natsx.HeaderOrderID),
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		return fmt.Errorf("reserve items: %w", err)
	}
	return nil
}

// ConfirmOrderMessage is the expected payload for CONFIRM_ORDER.
type ConfirmOrderMessage struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleConfirmOrder processes CONFIRM_ORDER commands.
func (w *Worker) HandleConfirmOrder(ctx context.Context, msg *nats.Msg) error {
	var req ConfirmOrderMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal confirm order request", slog.Any("error", err))
		return nil
	}

	err := w.svc.ConfirmOrder(ctx, app.ConfirmOrderRequest{
		OrderID:   req.OrderID,
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		return fmt.Errorf("confirm order: %w", err)
	}
	return nil
}

// ReleaseReservationMessage is the expected payload for RELEASE_RESERVATION.
type ReleaseReservationMessage struct {
	UserID    string `json:"user_id"`
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// HandleReleaseReservation processes RELEASE_RESERVATION commands.
func (w *Worker) HandleReleaseReservation(ctx context.Context, msg *nats.Msg) error {
	var req ReleaseReservationMessage
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		slog.Error("failed to unmarshal release reservation request", slog.Any("error", err))
		return nil
	}

	err := w.svc.ReleaseReservation(ctx, app.ReleaseReservationRequest{
		OrderID:   req.OrderID,
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}
	return nil
}

// SubscribeAll subscribes to all store command subjects via JetStream durable consumers.
func (w *Worker) SubscribeAll(ctx context.Context, client *natsx.Client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj string) error {
	subs := []struct {
		subject string
		durable string
		handler natsx.Handler
	}{
		{reserveItemsSubj, "store-reserve-items", w.HandleReserveItems},
		{confirmOrderSubj, "store-confirm-order", w.HandleConfirmOrder},
		{releaseReservationSubj, "store-release-reservation", w.HandleReleaseReservation},
	}

	for _, s := range subs {
		stream := natsx.StreamForSubject(s.subject)
		sub, err := client.ConsumeDurable(ctx, stream, s.durable, s.subject, s.handler, natsx.DurableConsumerConfig{
			Inbox: w.inbox,
		})
		if err != nil {
			w.Close()
			return fmt.Errorf("subscribe to %s: %w", s.subject, err)
		}
		w.subs = append(w.subs, sub)
	}

	return nil
}

func (w *Worker) ActiveSubscriptions() int {
	return len(w.subs)
}

func (w *Worker) Close() {
	for _, sub := range w.subs {
		if sub != nil {
			sub.Stop()
		}
	}
	w.subs = nil
}
