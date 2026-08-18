package http

import "time"

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type userDTO struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userDTO   `json:"user"`
}
