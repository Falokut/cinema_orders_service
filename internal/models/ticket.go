package models

type Ticket struct {
	ID     string          `json:"id"`
	Place  Place           `json:"place"`
	Price  uint32          `json:"price"`
	Status OrderItemStatus `json:"-"`
}
