package events

import (
	"context"

	"github.com/Falokut/cinema_orders_service/internal/models"
)

type KafkaConfig struct {
	Brokers []string
}

type OrdersEventsMQ interface {
	OrderCreated(ctx context.Context, email string, order models.Order) error
}
