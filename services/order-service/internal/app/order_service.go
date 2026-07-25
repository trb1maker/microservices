package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

type OrderService struct {
	carts    CartRepository
	orders   OrderRepository
	events   EventPublisher
	payments PaymentClient
	notifier StatusNotifier
	metrics  OrderMetrics
}

func NewOrderService(
	carts CartRepository,
	orders OrderRepository,
	events EventPublisher,
	extras ...any,
) *OrderService {
	var (
		payments PaymentClient  = NewNoopPaymentClient()
		notifier StatusNotifier = NoopStatusNotifier{}
		metrics  OrderMetrics   = NewNoopOrderMetrics()
	)
	for _, extra := range extras {
		switch value := extra.(type) {
		case PaymentClient:
			if value != nil {
				payments = value
			}
		case StatusNotifier:
			if value != nil {
				notifier = value
			}
		case OrderMetrics:
			if value != nil {
				metrics = value
			}
		}
	}
	return &OrderService{
		carts:    carts,
		orders:   orders,
		events:   events,
		payments: payments,
		notifier: notifier,
		metrics:  metrics,
	}
}

func (s *OrderService) Checkout(
	ctx context.Context,
	userID domain.UserID,
	deliveryAddress string,
	now time.Time,
) (*domain.Order, error) {
	if strings.TrimSpace(deliveryAddress) == "" {
		return nil, ErrDeliveryAddressRequired
	}

	cart, err := s.carts.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}
	if cart == nil || !cart.AllItemsReserved() {
		return nil, domain.ErrCartNotFullyReserved
	}

	order, err := cart.Checkout(deliveryAddress, now)
	if err != nil {
		return nil, fmt.Errorf("checkout cart: %w", err)
	}

	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	cart.Clear()
	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	if err := s.publishOrderCreated(ctx, order); err != nil {
		if rollbackErr := s.rollbackCheckout(ctx, userID, order); rollbackErr != nil {
			return nil, fmt.Errorf("publish order created: %w (rollback failed: %w)", err, rollbackErr)
		}
		return nil, fmt.Errorf("publish order created: %w", err)
	}

	s.metrics.RecordOrderCreated()
	s.notifyAndRefresh(ctx, order)
	return order, nil
}

func (s *OrderService) PayOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID, now time.Time) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	if err := requireOrderForUser(order, userID); err != nil {
		return nil, err
	}
	if order.Status() != domain.OrderStatusReserved {
		return nil, ErrOrderNotPayable
	}

	txID, ok, message, err := s.payments.Charge(ctx,
		uuid.UUID(order.OrderID()).String(),
		uuid.UUID(order.UserID()).String(),
		order.TotalPrice(),
	)
	if err != nil {
		return nil, fmt.Errorf("charge payment: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPaymentFailed, message)
	}

	paymentID, err := uuid.Parse(txID)
	if err != nil {
		return nil, fmt.Errorf("parse transaction id: %w", err)
	}

	if err := order.MarkPaid(domain.PaymentID(paymentID), now); err != nil {
		return nil, fmt.Errorf("mark paid: %w", err)
	}
	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	for _, item := range order.Items() {
		if err := s.events.PublishConfirmOrder(ctx, ConfirmOrder{
			OrderID:   uuid.UUID(order.OrderID()).String(),
			UserID:    uuid.UUID(order.UserID()).String(),
			ProductID: uuid.UUID(item.ProductID()).String(),
			Quantity:  int(item.Quantity()),
		}); err != nil {
			return nil, fmt.Errorf("publish confirm order: %w", err)
		}
	}

	s.notifyAndRefresh(ctx, order)
	return order, nil
}

func (s *OrderService) HandleOrderConfirmed(ctx context.Context, event OrderConfirmed, now time.Time) error {
	orderID, err := uuid.Parse(event.OrderID)
	if err != nil {
		return fmt.Errorf("parse order id: %w", err)
	}

	order, err := s.orders.Get(ctx, domain.OrderID(orderID))
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if order == nil {
		return nil
	}
	if order.Status() == domain.OrderStatusConfirmed {
		return nil
	}
	if order.Status() != domain.OrderStatusPaid {
		return nil
	}

	if err := order.MarkConfirmed(now); err != nil {
		return fmt.Errorf("mark confirmed: %w", err)
	}
	if err := s.orders.Save(ctx, order); err != nil {
		return fmt.Errorf("save order: %w", err)
	}

	if err := s.events.PublishOrderFinalized(ctx, OrderFinalized{
		OrderID:     uuid.UUID(order.OrderID()).String(),
		UserID:      uuid.UUID(order.UserID()).String(),
		TotalAmount: order.TotalPrice(),
		Status:      string(order.Status()),
		FinalizedAt: now.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("publish order finalized: %w", err)
	}

	s.notifyAndRefresh(ctx, order)
	return nil
}

func (s *OrderService) HandleReservationFailed(ctx context.Context, event ReservationFailed, now time.Time) error {
	orderID, err := uuid.Parse(event.OrderID)
	if err != nil {
		return fmt.Errorf("parse order id: %w", err)
	}
	order, err := s.orders.Get(ctx, domain.OrderID(orderID))
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if order == nil || order.Status() == domain.OrderStatusCancelled {
		return nil
	}
	if order.Status() == domain.OrderStatusPaid {
		_, succeeded, message, refundErr := s.payments.Refund(ctx,
			uuid.UUID(order.OrderID()).String(),
			uuid.UUID(order.UserID()).String(),
			order.TotalPrice(),
			uuid.UUID(order.PaymentID()).String(),
		)
		if refundErr != nil {
			return fmt.Errorf("refund payment: %w", refundErr)
		}
		if !succeeded {
			return fmt.Errorf("%w: %s", ErrPaymentFailed, message)
		}
	}
	if err := order.Cancel(now); err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}
	if err := s.orders.Save(ctx, order); err != nil {
		return fmt.Errorf("save order: %w", err)
	}
	if err := s.publishOrderCancelled(ctx, order); err != nil {
		return fmt.Errorf("publish order cancelled: %w", err)
	}
	s.notifyAndRefresh(ctx, order)
	return nil
}

func (s *OrderService) ListOrders(ctx context.Context, userID domain.UserID, limit, offset int) ([]*domain.Order, error) {
	if limit <= 0 {
		limit = 20
	}
	orders, err := s.orders.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return orders, nil
}

func (s *OrderService) GetOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	if err := requireOrderForUser(order, userID); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderService) GetOrderForService(ctx context.Context, orderID domain.OrderID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	return order, nil
}

func (s *OrderService) CancelOrder(
	ctx context.Context,
	userID domain.UserID,
	orderID domain.OrderID,
	now time.Time,
) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	if err := requireOrderForUser(order, userID); err != nil {
		return nil, err
	}

	releaseReservation := order.Status() == domain.OrderStatusPending || order.Status() == domain.OrderStatusReserved
	needsRefund := order.Status() == domain.OrderStatusPaid

	if needsRefund {
		_, ok, message, err := s.payments.Refund(ctx,
			uuid.UUID(order.OrderID()).String(),
			uuid.UUID(order.UserID()).String(),
			order.TotalPrice(),
			uuid.UUID(order.PaymentID()).String(),
		)
		if err != nil {
			return nil, fmt.Errorf("refund payment: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrPaymentFailed, message)
		}
		releaseReservation = true
	}

	if err := order.Cancel(now); err != nil {
		return nil, fmt.Errorf("cancel order: %w", err)
	}
	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	if err := s.publishOrderCancelled(ctx, order); err != nil {
		return nil, fmt.Errorf("publish order cancelled: %w", err)
	}

	if releaseReservation {
		for _, item := range order.Items() {
			if err := s.events.PublishReleaseReservation(ctx, ReleaseReservation{
				UserID:    uuid.UUID(order.UserID()).String(),
				OrderID:   uuid.UUID(order.OrderID()).String(),
				ProductID: uuid.UUID(item.ProductID()).String(),
				Quantity:  int(item.Quantity()),
			}); err != nil {
				return nil, fmt.Errorf("publish release reservation: %w", err)
			}
		}
	}

	s.notifyAndRefresh(ctx, order)
	return order, nil
}

func (s *OrderService) rollbackCheckout(ctx context.Context, userID domain.UserID, order *domain.Order) error {
	if err := s.orders.Delete(ctx, order.OrderID()); err != nil {
		return fmt.Errorf("delete order: %w", err)
	}
	restoredCart, err := domain.ReconstituteCart(userID, order.OrderID(), time.Now(), order.Items()...)
	if err != nil {
		return fmt.Errorf("reconstitute cart: %w", err)
	}
	if err := s.carts.Save(ctx, restoredCart); err != nil {
		return fmt.Errorf("restore cart: %w", err)
	}
	s.refreshActiveOrders(ctx)
	return nil
}

func (s *OrderService) publishOrderCreated(ctx context.Context, order *domain.Order) error {
	if err := s.events.PublishOrderCreated(ctx, OrderCreated{
		OrderID:    uuid.UUID(order.OrderID()).String(),
		UserID:     uuid.UUID(order.UserID()).String(),
		TotalPrice: order.TotalPrice(),
	}); err != nil {
		return fmt.Errorf("publish order created event: %w", err)
	}
	return nil
}

func (s *OrderService) publishOrderCancelled(ctx context.Context, order *domain.Order) error {
	if err := s.events.PublishOrderCancelled(ctx, OrderCancelled{
		OrderID: uuid.UUID(order.OrderID()).String(),
		UserID:  uuid.UUID(order.UserID()).String(),
	}); err != nil {
		return fmt.Errorf("publish order cancelled event: %w", err)
	}
	return nil
}

func (s *OrderService) notifyAndRefresh(ctx context.Context, order *domain.Order) {
	s.notifier.NotifyOrderStatus(order)
	s.refreshActiveOrders(ctx)
}

func (s *OrderService) refreshActiveOrders(ctx context.Context) {
	count, err := s.orders.CountActiveOrders(ctx)
	if err != nil {
		slog.WarnContext(ctx, "active_orders_refresh_failed", slog.Any("error", err))
		return
	}
	s.metrics.SetActiveOrders(count)
}

func requireOrderForUser(order *domain.Order, userID domain.UserID) error {
	if order == nil || order.UserID() != userID {
		return ErrOrderNotFound
	}
	return nil
}
