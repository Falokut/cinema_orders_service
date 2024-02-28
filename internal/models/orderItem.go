package models

type OrderItem struct {
	Id     string          `bson:"_id"`
	Status OrderItemStatus `bson:"status"`
}
