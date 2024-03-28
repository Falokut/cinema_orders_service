package models

type OrderItem struct {
	ID     string          `bson:"_id"`
	Status OrderItemStatus `bson:"status"`
}
