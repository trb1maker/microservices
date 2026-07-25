//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestE2E_UserCancellation(t *testing.T) {
	env := newEnv(t)

	orderID := env.checkoutOrder(t)
	reservedBefore := env.stockReserved(t)
	require.Greater(t, reservedBefore, 0)

	cancelResp := env.doJSON(t, http.MethodDelete, "/orders/"+orderID, env.token, "")
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)
	var cancelled struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(cancelResp.Body).Decode(&cancelled))
	_ = cancelResp.Body.Close()
	assert.Equal(t, "CANCELLED", cancelled.Status)

	require.Eventually(t, func() bool {
		return env.notification.Sink().HasEvent("ORDER_CANCELLED")
	}, 10*time.Second, pollInterval)

	require.Eventually(t, func() bool {
		var doc struct {
			Reserved int `bson:"reserved"`
		}
		err := env.mongoDB.Collection("stock").FindOne(context.Background(), bson.M{"product_id": testProductID}).Decode(&doc)
		return err == nil && doc.Reserved == 0
	}, 10*time.Second, pollInterval)

	assert.Equal(t, 0, env.countPaymentTransactions(t, orderID))
	assert.Equal(t, 0, env.countProcessedOrders(t, orderID))
	assert.False(t, env.notification.Sink().HasEvent("ORDER_FINALIZED"))
}
