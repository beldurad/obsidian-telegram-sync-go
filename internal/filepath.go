package app

import "context"

type FilePath struct {
	Alias string
	Path  string
}

type FilePathService interface {
	Save(context.Context, *FilePath) error
}
