package models

type Ticket struct {
	Id     string          `json:"id"`
	Place  Place           `json:"place"`
	Price  uint32          `json:"price"`
	Status OrderItemStatus `json:"-"`
}
