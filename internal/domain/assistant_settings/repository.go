package assistantsettings

import "github.com/cymonevo/go_template/pkg/store"

// Repository wraps the generic store for assistant settings.
type Repository interface {
	store.Store[Settings]
}

type repository struct {
	store.Store[Settings]
}

// NewRepository constructs a Repository backed by the given store.
func NewRepository(s store.Store[Settings]) Repository {
	return &repository{Store: s}
}
