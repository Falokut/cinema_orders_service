package models

import (
	"errors"
	"strings"
	"time"
)

type OrderItemStatus string

const (
	OrderItemStatusPaid            OrderItemStatus = "PAID"
	OrderItemStatusPaymentRequired OrderItemStatus = "PAYMENT_REQUIRED"
	OrderItemStatusRefundAwaiting  OrderItemStatus = "REFUND_AWAITING"
	OrderItemStatusCanceled        OrderItemStatus = "CANCELED"
	OrderItemStatusUsed            OrderItemStatus = "USED"
	OrderItemStatusRefunded        OrderItemStatus = "REFUNDED"
)

type Order struct {
	ID          string    `json:"id"`
	Tickets     []Ticket  `json:"tickets"`
	ScreeningID int64     `json:"screening_id"`
	Date        time.Time `json:"order_date"`
}

func OrderItemStatusFromString(str string) (status OrderItemStatus, err error) {
	switch {
	case strings.EqualFold(str, "PAID"):
		status = OrderItemStatusPaid
	case strings.EqualFold(str, "PAYMENT_REQUIRED") || strings.EqualFold(str, "PAYMENTREQUIRED"):
		status = OrderItemStatusPaymentRequired
	case strings.EqualFold(str, "REFUND_AWAITING") || strings.EqualFold(str, "REFUNDAWAITING"):
		status = OrderItemStatusRefundAwaiting
	case strings.EqualFold(str, "CANCELED"):
		status = OrderItemStatusCanceled
	case strings.EqualFold(str, "USED"):
		status = OrderItemStatusUsed
	case strings.EqualFold(str, "REFUNDED"):
		status = OrderItemStatusRefunded
	default:
		err = errors.New("unknown status")
	}
	return
}

func GetAllowedPreviousOrderStatuses(status OrderItemStatus) (allowed []OrderItemStatus) {
	switch status {
	case OrderItemStatusCanceled:
		return []OrderItemStatus{OrderItemStatusPaymentRequired}
	case OrderItemStatusPaid:
		return []OrderItemStatus{OrderItemStatusPaymentRequired}
	case OrderItemStatusUsed:
		return []OrderItemStatus{OrderItemStatusPaid}
	case OrderItemStatusRefundAwaiting:
		return []OrderItemStatus{OrderItemStatusPaid, OrderItemStatusUsed}
	case OrderItemStatusRefunded:
		return []OrderItemStatus{OrderItemStatusRefundAwaiting}
	}
	return []OrderItemStatus{}
}
