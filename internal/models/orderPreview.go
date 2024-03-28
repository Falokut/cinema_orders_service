package models

import "time"

type OrderPreview struct {
	OrderID     string    `bson:"_id"`
	OrderDate   time.Time `bson:"order_date"`
	TotalPrice  uint32    `bson:"total_price"`
	ScreeningID int64     `bson:"screening_id"`
}
