package domain

import "errors"

// Sentinel errors returned by the link domain and application layers. Adapters map
// these to transport-specific codes (e.g. HTTP status) at the edge.
var (
	ErrNotFound      = errors.New("link: not found")
	ErrInvalidURL    = errors.New("link: long url must be an absolute http(s) url")
	ErrURLTooLong    = errors.New("link: long url too long")
	ErrAlreadyExists = errors.New("link: already exists")
	ErrRateLimited   = errors.New("link: rate limited")
)
