package domain

import (
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("link: not found")
	ErrDisabled    = errors.New("LINK_DISABLED")
	ErrInvalid     = errors.New("link: invalid request")
	ErrUnavailable = errors.New("link: unavailable")
	ErrDuplicate   = errors.New("link: duplicate mapping")
)

type Link struct {
	ID         int64
	Code       string
	TenantID   string
	LongURL    string
	CreatedAt  time.Time
	DisabledAt *time.Time
}

type Position struct {
	CreatedAt time.Time
	ID        int64
}
