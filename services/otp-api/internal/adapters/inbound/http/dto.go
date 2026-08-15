package http

// Request/response bodies for the OTP HTTP API. `binding` tags drive gin's validation
// at the boundary - a bad body is rejected with 400 before any use case runs.

type sendRequest struct {
	Recipient string `json:"recipient" binding:"required,email"`
}

type sendResponse struct {
	RequestID string `json:"request_id"`
}

type verifyRequest struct {
	Recipient string `json:"recipient" binding:"required,email"`
	Code      string `json:"code" binding:"required"`
}

type apiKeyDTO struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Status   string `json:"status"`
}

type requestDTO struct {
	ID        string `json:"id"`
	Recipient string `json:"recipient"` // already masked at rest
	Channel   string `json:"channel"`
	State     string `json:"state"`
}

type deliveryLogDTO struct {
	RequestID     string `json:"request_id"`
	Provider      string `json:"provider"`
	Status        string `json:"status"`
	LatencyMillis int64  `json:"latency_ms"`
	Error         string `json:"error,omitempty"`
}
