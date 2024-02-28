package models

import "time"

type OrderPreview struct {
	OrderId     string    `bson:"_id"`
	OrderDate   time.Time `bson:"order_date"`
	TotalPrice  uint32    `bson:"total_price"`
	ScreeningId int64     `bson:"screening_id"`
}
