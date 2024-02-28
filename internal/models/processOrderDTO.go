package models

type ProcessOrderPlace struct {
	Place Place
	Price uint32
}

type ProcessOrderDTO struct {
	Places      []ProcessOrderPlace
	Id          string
	OwnerId     string
	ScreeningId int64
}
