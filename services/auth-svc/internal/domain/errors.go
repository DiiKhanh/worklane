package domain

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactive           = errors.New("user inactive")
	ErrRateLimited        = errors.New("too many attempts")
	ErrAPIKeyNotFound     = errors.New("api key not found")
)
