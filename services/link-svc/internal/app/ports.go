package app

import (
	"context"
	"errors"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

var ErrInvalidAPIKey = errors.New("invalid api key")

type Introspector interface {
	Introspect(context.Context, string) (string, error)
}

type Repo interface {
	ByDestination(context.Context, string, string) (domain.Link, error)
	Insert(context.Context, domain.Link) (domain.Link, error)
	Get(context.Context, string, string) (domain.Link, error)
	List(context.Context, string, *domain.Position, int) ([]domain.Link, error)
	Disable(context.Context, string, string) (domain.Link, error)
}

type IDGenerator interface {
	New() (int64, string, error)
}

type Quota interface {
	Take(context.Context, string) (time.Duration, error)
}

type Cache interface {
	Evict(context.Context, string) error
}
