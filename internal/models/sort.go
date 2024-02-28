package models

type SortOrdering string

const (
	ASC  SortOrdering = "ASC"
	DESC SortOrdering = "DESC"
)

type SortDTO struct {
	FieldName    string
	SortOrdering SortOrdering
}
