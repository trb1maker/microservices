package events

type OrderCreated struct {
	OrderID    string `json:"order_id"`
	UserID     string `json:"user_id"`
	TotalPrice int64  `json:"total_price"`
}

type ReserveItems struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type ConfirmOrder struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type ReleaseReservation struct {
	UserID    string `json:"user_id"`
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type OrderFinalized struct {
	OrderID         string `json:"order_id"`
	UserID          string `json:"user_id"`
	TotalAmount     int64  `json:"total_amount"`
	Status          string `json:"status"`
	FinalizedAt     string `json:"finalized_at"`
	DeliveryAddress string `json:"delivery_address"`
}

type OrderCancelled struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

type ItemsReserved struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Timestamp string `json:"timestamp"`
}

type ReservationFailed struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

type OrderConfirmed struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	Timestamp string `json:"timestamp"`
}

type ReservationReleased struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Timestamp string `json:"timestamp"`
}

type PaymentSucceeded struct {
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	Amount        int64  `json:"amount"`
	TransactionID string `json:"transaction_id"`
	Timestamp     string `json:"timestamp"`
}

type PaymentFailed struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	Amount    int64  `json:"amount"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

type RefundSucceeded struct {
	OrderID               string `json:"order_id"`
	UserID                string `json:"user_id"`
	Amount                int64  `json:"amount"`
	TransactionID         string `json:"transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	Timestamp             string `json:"timestamp"`
}

type RefundFailed struct {
	OrderID               string `json:"order_id"`
	UserID                string `json:"user_id"`
	Amount                int64  `json:"amount"`
	OriginalTransactionID string `json:"original_transaction_id"`
	Reason                string `json:"reason"`
	Timestamp             string `json:"timestamp"`
}
