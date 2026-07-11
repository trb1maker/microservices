package app

type NoopOrderMetrics struct{}

func NewNoopOrderMetrics() *NoopOrderMetrics {
	return &NoopOrderMetrics{}
}

func (NoopOrderMetrics) RecordOrderCreated() {}

func (NoopOrderMetrics) SetActiveOrders(int) {}
