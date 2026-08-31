package grpc

import (
	"sync"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"
	orderpb "github.com/trb1maker/microservices/internal/platform/proto/order"
)

type StatusHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan *orderpb.StatusUpdate]struct{}
}

const subscriberBufferSize = 8

func NewStatusHub() *StatusHub {
	return &StatusHub{
		subscribers: make(map[string]map[chan *orderpb.StatusUpdate]struct{}),
	}
}

func (h *StatusHub) Subscribe(orderID string) (<-chan *orderpb.StatusUpdate, func()) {
	ch := make(chan *orderpb.StatusUpdate, subscriberBufferSize)
	h.mu.Lock()
	if h.subscribers[orderID] == nil {
		h.subscribers[orderID] = make(map[chan *orderpb.StatusUpdate]struct{})
	}
	h.subscribers[orderID][ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(h.subscribers[orderID], ch)
		if len(h.subscribers[orderID]) == 0 {
			delete(h.subscribers, orderID)
		}
		close(ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *StatusHub) Notify(order *domain.Order) {
	if order == nil {
		return
	}
	history := order.StatusHistory()
	if len(history) == 0 {
		return
	}
	entry := history[len(history)-1]
	orderID := uuid.UUID(order.OrderID()).String()
	h.publish(orderID, &orderpb.StatusUpdate{
		OrderId:   orderID,
		Status:    string(entry.Status),
		Reason:    entry.Reason,
		Timestamp: entry.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *StatusHub) publish(orderID string, update *orderpb.StatusUpdate) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers[orderID] {
		select {
		case ch <- update:
		default:
		}
	}
}

var _ interface {
	NotifyOrderStatus(*domain.Order)
} = (*StatusHub)(nil)

func (h *StatusHub) NotifyOrderStatus(order *domain.Order) {
	h.Notify(order)
}
