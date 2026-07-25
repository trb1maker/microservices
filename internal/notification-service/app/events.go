package app

type OrderFinalized struct {
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	TotalAmount int64  `json:"total_amount"`
	Status      string `json:"status"`
	FinalizedAt string `json:"finalized_at"`
}

type OrderCancelled struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

type PaymentSucceeded struct {
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	Amount        int64  `json:"amount"`
	TransactionID string `json:"transaction_id"`
	Timestamp     string `json:"timestamp"`
}

type RefundSucceeded struct {
	OrderID               string `json:"order_id"`
	UserID                string `json:"user_id"`
	Amount                int64  `json:"amount"`
	TransactionID         string `json:"transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	Timestamp             string `json:"timestamp"`
}
