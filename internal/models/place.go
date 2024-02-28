package models

type Place struct {
	Row  int32 `bson:"row" json:"row"`
	Seat int32 `bson:"seat" json:"seat"`
}
