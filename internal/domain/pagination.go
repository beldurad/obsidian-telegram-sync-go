package domain

const DefaultPageSize = 5

type Page[T any] struct {
	TotalPages int
	CurPage    int
	Values     []T
}

func (p Page[T]) HasNext() bool {
	if p.TotalPages < 0 {
		return false
	}
	if p.CurPage < 0 {
		return false
	}
	return p.CurPage < p.TotalPages-1
}

func (p Page[T]) HasPrev() bool {
	if p.TotalPages < 0 {
		return false
	}
	return p.CurPage > 0
}
