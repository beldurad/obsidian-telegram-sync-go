package app

import (
	"context"
	"strings"
)

// Template represents text pattern which applies to [Note]'s text
type Template struct {
	Alias string
	Text  string
}

func (t *Template) Validate() bool {
	if t.Alias == "" || t.Text == "" {
		return false
	}
	if !strings.Contains(t.Text, "%s") {
		return false
	}
	return true
}

type TemplateService interface {
	Save(context.Context, *Template) error
}
