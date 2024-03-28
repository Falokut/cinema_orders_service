package models

type ProcessOrderPlace struct {
	Place Place
	Price uint32
}

type ProcessOrderDTO struct {
	Places      []ProcessOrderPlace
	ID          string
	OwnerID     string
	ScreeningID int64
}
