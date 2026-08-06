package domain

const DefaultPageSize = 5

type Page[T any] struct {
	TotalPages int
	Values     []T
}
