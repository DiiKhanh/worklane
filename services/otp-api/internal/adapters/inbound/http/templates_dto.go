package http

import "time"

// Request/response bodies for the template CRUD + preview API.

type createTemplateRequest struct {
	Name    string `json:"name" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Locale  string `json:"locale" binding:"required"`
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
	Note    string `json:"note"`
}

type addVersionRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
	Note    string `json:"note"`
}

type previewRequest struct {
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body" binding:"required"`
}

type templateDTO struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Channel         string    `json:"channel"`
	Locale          string    `json:"locale"`
	Status          string    `json:"status"`
	ActiveVersionID string    `json:"active_version_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type versionDTO struct {
	ID        string    `json:"id"`
	VersionNo int       `json:"version_no"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type templateDetailDTO struct {
	Template templateDTO  `json:"template"`
	Versions []versionDTO `json:"versions"`
}

type previewResponse struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
