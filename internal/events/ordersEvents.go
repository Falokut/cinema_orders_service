package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Falokut/cinema_orders_service/internal/models"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type ordersEvents struct {
	eventsWriter *kafka.Writer
	logger       *logrus.Logger
}

func NewOrdersEvents(cfg KafkaConfig, logger *logrus.Logger) *ordersEvents {
	w := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Logger:                 logger,
		AllowAutoTopicCreation: true,
	}
	return &ordersEvents{eventsWriter: w, logger: logger}
}

const (
	orderCreatedTopic       = "order_created"
	orderStatusChangedTopic = "order_status_changed"
)

func (e *ordersEvents) Shutdown() error {
	return e.eventsWriter.Close()
}

type orderCreated struct {
	Email string       `json:"email"`
	Order models.Order `json:"order"`
}

func (e *ordersEvents) OrderCreated(ctx context.Context, email string, order models.Order) error {
	body, err := json.Marshal(orderCreated{Email: email, Order: order})
	if err != nil {
		e.logger.Fatal(err)
	}
	return e.eventsWriter.WriteMessages(ctx, kafka.Message{
		Topic: orderCreatedTopic,
		Key:   []byte(fmt.Sprint("order_", order.ID)),
		Value: body,
	})
}
