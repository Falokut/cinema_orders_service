package paymentservice

import (
	"context"
	"time"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/sirupsen/logrus"
)

type OrdersRepository interface {
	ChangeOrderStatus(ctx context.Context, orderID string, newStatus models.OrderItemStatus) error
	ChangeOrderItemsStatus(ctx context.Context, orderID string,
		itemsIds []string, newStatus models.OrderItemStatus) error
	GetOrderTotalPrice(ctx context.Context, orderID string) (uint32, error)
	GetOrderItemsTotalPrice(ctx context.Context, orderID string, itemsIds []string) (uint32, error)
}

type PaymentServiceStub struct {
	paymenturl       string
	paymentSleepTime time.Duration
	refundSleepTime  time.Duration
	repo             OrdersRepository
	logger           *logrus.Logger
}

func NewPaymentServiceStub(paymentStubURL string,
	paymentSleepTime time.Duration,
	refundSleepTime time.Duration,
	repo OrdersRepository,
	logger *logrus.Logger,
) *PaymentServiceStub {
	return &PaymentServiceStub{
		paymenturl:       paymentStubURL,
		paymentSleepTime: paymentSleepTime,
		refundSleepTime:  refundSleepTime,
		repo:             repo,
		logger:           logger,
	}
}

const errorSleepDuration = time.Millisecond * 200

func (s *PaymentServiceStub) ChangeOrderStatus(ctx context.Context, orderID string, newStatus models.OrderItemStatus) (err error) {
	retry := 0
	for retry <= 2 {
		err = s.repo.ChangeOrderStatus(ctx, orderID, newStatus)
		if err == nil {
			return nil
		}
		s.logger.Errorf("error while changing order status %v", err)
		retry++
		time.Sleep(errorSleepDuration)
	}

	return
}

func (s *PaymentServiceStub) ChangeOrderItemsStatus(ctx context.Context, orderID string,
	itemsIds []string, newStatus models.OrderItemStatus) (err error) {
	retry := 0
	for retry <= 2 {
		err = s.repo.ChangeOrderItemsStatus(ctx, orderID, itemsIds, newStatus)

		if err == nil {
			return nil
		}
		s.logger.Errorf("error while changing order items status %v", err)
		retry++
		time.Sleep(errorSleepDuration)
	}

	return
}

func (s *PaymentServiceStub) PreparePaymentURL(_ context.Context,
	_ string,
	total uint32,
	orderID string) (paymenturl string, err error) {
	go func() {
		time.Sleep(s.paymentSleepTime)
		err := s.ChangeOrderStatus(context.Background(), orderID, models.OrderItemStatusPaid)
		if err != nil {
			err := s.orderRefundRequest(context.Background(), float64(total), orderID)
			if err != nil {
				s.logger.Error(err)
			}
		}
	}()

	paymenturl = s.paymenturl
	return
}

func (s *PaymentServiceStub) RequestOrderRefund(ctx context.Context, percent uint32, orderID string) (err error) {
	total, err := s.repo.GetOrderTotalPrice(ctx, orderID)
	if err != nil {
		return
	}

	go func() {
		err := s.orderRefundRequest(context.Background(), float64(total)*float64(percent)/percentBase, orderID)
		if err != nil {
			s.logger.Error(err)
		}
	}()
	return
}

const percentBase = 100.0

func (s *PaymentServiceStub) RequestOrderItemsRefund(ctx context.Context,
	percent uint32, orderID string, itemsIds []string) (err error) {
	total, err := s.repo.GetOrderItemsTotalPrice(ctx, orderID, itemsIds)
	if err != nil {
		return
	}

	go func() {
		err := s.orderItemsRefundRequest(context.Background(), float64(total)*float64(percent)/percentBase, orderID, itemsIds)
		if err != nil {
			s.logger.Error(err)
		}
	}()
	return
}

func (s *PaymentServiceStub) orderItemsRefundRequest(ctx context.Context,
	_ float64, orderID string, itemsIds []string) (err error) {
	err = s.ChangeOrderItemsStatus(ctx, orderID, itemsIds, models.OrderItemStatusRefundAwaiting)
	if err != nil {
		return
	}

	time.Sleep(s.refundSleepTime)
	err = s.ChangeOrderItemsStatus(ctx, orderID, itemsIds, models.OrderItemStatusRefunded)

	return
}

func (s *PaymentServiceStub) orderRefundRequest(ctx context.Context, _ float64, orderID string) (err error) {
	err = s.ChangeOrderStatus(ctx, orderID, models.OrderItemStatusRefundAwaiting)
	if err != nil {
		return
	}

	time.Sleep(s.refundSleepTime)
	err = s.ChangeOrderStatus(ctx, orderID, models.OrderItemStatusRefunded)
	return
}
