package paymentservice

import (
	"context"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/sirupsen/logrus"
)

type OrdersRepository interface {
	ChangeOrderStatus(ctx context.Context, orderId string, newStatus models.OrderItemStatus) error
	ChangeOrderItemsStatus(ctx context.Context, orderId string,
		itemsIds []string, newStatus models.OrderItemStatus) error
	GetOrderTotalPrice(ctx context.Context, orderId string) (uint32, error)
	GetOrderItemsTotalPrice(ctx context.Context, orderId string, itemsIds []string) (uint32, error)
}

type PaymentServiceStub struct {
	paymenturl       string
	paymentSleepTime time.Duration
	refundSleepTime  time.Duration
	repo             OrdersRepository
	logger           *logrus.Logger
}

func NewPaymentServiceStub(paymentStubUrl string,
	paymentSleepTime time.Duration,
	refundSleepTime time.Duration,
	repo OrdersRepository,
	logger *logrus.Logger,
) *PaymentServiceStub {
	return &PaymentServiceStub{
		paymenturl:       paymentStubUrl,
		paymentSleepTime: paymentSleepTime,
		refundSleepTime:  refundSleepTime,
		repo:             repo,
		logger:           logger,
	}
}

func (s *PaymentServiceStub) ChangeOrderStatus(ctx context.Context, orderId string, newStatus models.OrderItemStatus) (err error) {
	retry := 0
	for retry <= 2 {
		err = s.repo.ChangeOrderStatus(ctx, orderId, newStatus)
		if err == nil {
			return nil
		}
		s.logger.Errorf("error while changing order status %v", err)
		retry++
		time.Sleep(time.Millisecond * 200)
	}

	return
}

func (s *PaymentServiceStub) ChangeOrderItemsStatus(ctx context.Context, orderId string,
	itemsIds []string, newStatus models.OrderItemStatus) (err error) {
	retry := 0
	for retry <= 2 {
		err = s.repo.ChangeOrderItemsStatus(ctx, orderId, itemsIds, newStatus)

		if err == nil {
			return nil
		}
		s.logger.Errorf("error while changing order items status %v", err)
		retry++
		time.Sleep(time.Millisecond * 200)
	}

	return
}

func (s *PaymentServiceStub) PreparePaymentUrl(ctx context.Context, email string, total uint32, orderId string) (paymenturl string, err error) {
	go func() {
		time.Sleep(s.paymentSleepTime)
		err := s.ChangeOrderStatus(context.Background(), orderId, models.ORDER_ITEM_STATUS_PAID)
		if err != nil {
			s.orderRefundRequest(context.Background(), float64(total), orderId)
		}
	}()

	paymenturl = s.paymenturl
	return
}

func (s *PaymentServiceStub) RequestOrderRefund(ctx context.Context, percent uint32, orderId string) (err error) {
	total, err := s.repo.GetOrderTotalPrice(ctx, orderId)
	if err != nil {
		return
	}

	go func() {
		s.orderRefundRequest(ctx, float64(total)*float64(percent)/100.0, orderId)
	}()
	return
}

func (s *PaymentServiceStub) RequestOrderItemsRefund(ctx context.Context,
	percent uint32, orderId string, itemsIds []string) (err error) {
	total, err := s.repo.GetOrderItemsTotalPrice(ctx, orderId, itemsIds)
	if err != nil {
		return
	}

	go func() {
		s.orderItemsRefundRequest(ctx, float64(total)*float64(percent)/100.0, orderId, itemsIds)
	}()
	return
}

func (s *PaymentServiceStub) orderItemsRefundRequest(ctx context.Context,
	total float64, orderId string, itemsIds []string) (err error) {
	err = s.ChangeOrderItemsStatus(context.Background(), orderId, itemsIds, models.ORDER_ITEM_STATUS_REFUND_AWAITING)
	if err != nil {
		return
	}

	time.Sleep(s.refundSleepTime)
	err = s.ChangeOrderItemsStatus(context.Background(), orderId, itemsIds, models.ORDER_ITEM_STATUS_REFUNDED)

	return
}

func (s *PaymentServiceStub) orderRefundRequest(ctx context.Context, total float64, orderId string) (err error) {
	err = s.ChangeOrderStatus(context.Background(), orderId, models.ORDER_ITEM_STATUS_REFUND_AWAITING)
	if err != nil {
		return
	}

	time.Sleep(s.refundSleepTime)
	err = s.ChangeOrderStatus(context.Background(), orderId, models.ORDER_ITEM_STATUS_REFUNDED)
	return
}
