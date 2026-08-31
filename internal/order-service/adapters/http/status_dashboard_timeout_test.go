package http

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const paymentServiceName = "Payment Service"

type timeoutRemoteChecker struct {
	delay time.Duration
}

func (c timeoutRemoteChecker) CheckPayment(ctx context.Context) StatusItem {
	select {
	case <-ctx.Done():
		return StatusItem{
			Name:        paymentServiceName,
			Status:      statusDown,
			StatusClass: statusDown,
			Detail:      ctx.Err().Error(),
			Endpoint:    "payment-service:50051",
		}
	case <-time.After(c.delay):
		return StatusItem{
			Name:        paymentServiceName,
			Status:      statusOK,
			StatusClass: statusOK,
			Detail:      "grpc health SERVING",
			Endpoint:    "payment-service:50051",
		}
	}
}

func (timeoutRemoteChecker) CheckStore(context.Context) StatusItem {
	return StatusItem{
		Name:        "Store Service",
		Status:      statusOK,
		StatusClass: statusOK,
		Detail:      statusReady,
		Endpoint:    "http://store-service:9092/ready",
	}
}

func TestStatusDashboard_appliesConfiguredTimeout(t *testing.T) {
	t.Parallel()

	dashboard, err := NewStatusDashboard(nil, timeoutRemoteChecker{delay: time.Second})
	require.NoError(t, err)
	dashboard.timeout = 40 * time.Millisecond

	start := time.Now()
	data := dashboard.Build(context.Background())
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 200*time.Millisecond)

	var payment StatusItem
	for _, item := range data.Items {
		if item.Name == paymentServiceName {
			payment = item
			break
		}
	}
	assert.Equal(t, statusDown, payment.Status)
}
