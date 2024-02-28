package models

import (
	"errors"
	"strings"
	"time"
)

type OrderItemStatus string

const (
	ORDER_ITEM_STATUS_PAID             OrderItemStatus = "PAID"
	ORDER_ITEM_STATUS_PAYMENT_REQUIRED OrderItemStatus = "PAYMENT_REQUIRED"
	ORDER_ITEM_STATUS_REFUND_AWAITING  OrderItemStatus = "REFUND_AWAITING"
	ORDER_ITEM_STATUS_CANCELLED        OrderItemStatus = "CANCELLED"
	ORDER_ITEM_STATUS_USED             OrderItemStatus = "USED"
	ORDER_ITEM_STATUS_REFUNDED         OrderItemStatus = "REFUNDED"
)

type Order struct {
	Id          string    `json:"id"`
	Tickets     []Ticket  `json:"tickets"`
	ScreeningId int64     `json:"screening_id"`
	Date        time.Time `json:"order_date"`
}

func OrderItemStatusFromString(str string) (status OrderItemStatus, err error) {
	switch {
	case strings.EqualFold(str, "PAID"):
		status = ORDER_ITEM_STATUS_PAID
	case strings.EqualFold(str, "PAYMENT_REQUIRED") || strings.EqualFold(str, "PAYMENTREQUIRED"):
		status = ORDER_ITEM_STATUS_PAYMENT_REQUIRED
	case strings.EqualFold(str, "REFUND_AWAITING") || strings.EqualFold(str, "REFUNDAWAITING"):
		status = ORDER_ITEM_STATUS_REFUND_AWAITING
	case strings.EqualFold(str, "CANCELLED"):
		status = ORDER_ITEM_STATUS_CANCELLED
	case strings.EqualFold(str, "USED"):
		status = ORDER_ITEM_STATUS_USED
	case strings.EqualFold(str, "REFUNDED"):
		status = ORDER_ITEM_STATUS_REFUNDED
	default:
		err = errors.New("unknown status")
	}
	return
}

func GetAllowedPreviousOrderStatuses(status OrderItemStatus) (allowed []OrderItemStatus) {
	switch status {
	case ORDER_ITEM_STATUS_CANCELLED:
		return []OrderItemStatus{ORDER_ITEM_STATUS_PAYMENT_REQUIRED}
	case ORDER_ITEM_STATUS_PAID:
		return []OrderItemStatus{ORDER_ITEM_STATUS_PAYMENT_REQUIRED}
	case ORDER_ITEM_STATUS_USED:
		return []OrderItemStatus{ORDER_ITEM_STATUS_PAID}
	case ORDER_ITEM_STATUS_REFUND_AWAITING:
		return []OrderItemStatus{ORDER_ITEM_STATUS_PAID, ORDER_ITEM_STATUS_USED}
	case ORDER_ITEM_STATUS_REFUNDED:
		return []OrderItemStatus{ORDER_ITEM_STATUS_REFUND_AWAITING}
	}
	return []OrderItemStatus{}
}
