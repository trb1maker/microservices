// go:build integration

package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_StoreFailureAfterPayment(t *testing.T) {
	env := newEnv(t, envOptions{gateConfirm: true})

	orderID := env.checkoutOrder(t)

	payResp := env.doJSON(t, http.MethodPost, "/orders/"+orderID+"/pay", env.token, "")
	require.Equal(t, http.StatusOK, payResp.StatusCode)
	_ = payResp.Body.Close()

	env.waitForOrderStatus(t, orderID, "PAID")
	env.breakStoreConfirm(t)
	env.storeWorker.AllowConfirm()
	env.waitForOrderStatus(t, orderID, "CANCELLED")

	require.Eventually(t, func() bool {
		sink := env.notification.Sink()
		return sink.HasEvent("ORDER_CANCELLED") && sink.HasEvent("REFUND_SUCCEEDED")
	}, 10*time.Second, pollInterval)

	assert.Equal(t, 0, env.countProcessedOrders(t, orderID))
	assert.GreaterOrEqual(t, env.countPaymentTransactions(t, orderID), 2)
	assert.Equal(t, 0, env.stockReserved(t))
	assert.False(t, env.notification.Sink().HasEvent("ORDER_FINALIZED"))
}
