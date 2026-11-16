package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/idgen"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

const maxCreateBodyBytes = 32 * 1024 // permits JSON-escaped 4096-byte destinations

type Handlers struct {
	svc        Management
	publicBase string
	shortHost  string
}

func (h *Handlers) Create(c *gin.Context) {
	var body CreateRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateBodyBytes)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil || !utf8.Valid(data) {
		respondError(c, domain.ErrInvalid)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !validDestination(body.LongURL, h.shortHost) {
		respondError(c, domain.ErrInvalid)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respondError(c, domain.ErrInvalid)
		return
	}
	mapping, created, err := h.svc.Create(c.Request.Context(), c.GetString(tenantCtxKey), body.LongURL)
	if err != nil {
		respondError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, CreateResponse{Code: mapping.Code, ShortURL: h.publicBase + "/" + mapping.Code})
}

func (h *Handlers) Get(c *gin.Context) {
	if !validCode(c.Param("code")) {
		respondError(c, domain.ErrNotFound)
		return
	}
	mapping, err := h.svc.Get(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("code"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.metadata(mapping))
}

func (h *Handlers) Disable(c *gin.Context) {
	if !validCode(c.Param("code")) {
		respondError(c, domain.ErrNotFound)
		return
	}
	mapping, err := h.svc.Disable(c.Request.Context(), c.GetString(tenantCtxKey), c.Param("code"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.metadata(mapping))
}

type cursor struct {
	Tenant    string    `json:"t"`
	CreatedAt time.Time `json:"c"`
	ID        *int64    `json:"i"`
}

func (h *Handlers) List(c *gin.Context) {
	if len(c.Request.URL.RawQuery) > 1024 {
		respondError(c, domain.ErrInvalid)
		return
	}
	query, err := urlQuery(c)
	if err != nil {
		respondError(c, domain.ErrInvalid)
		return
	}
	limit := DefaultPageSize
	if values, ok := query["limit"]; ok {
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > MaxPageSize {
			respondError(c, domain.ErrInvalid)
			return
		}
	}
	tenant := c.GetString(tenantCtxKey)
	var before *domain.Position
	if values, ok := query["cursor"]; ok {
		raw := values[0]
		if len(raw) == 0 || len(raw) > MaxCursorBytes {
			respondError(c, domain.ErrInvalid)
			return
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		var pos cursor
		if err != nil || json.Unmarshal(data, &pos) != nil || pos.Tenant != tenant || pos.CreatedAt.IsZero() || pos.ID == nil || *pos.ID < 0 {
			respondError(c, domain.ErrInvalid)
			return
		}
		before = &domain.Position{CreatedAt: pos.CreatedAt.UTC(), ID: *pos.ID}
	}
	rows, err := h.svc.List(c.Request.Context(), tenant, before, limit+1)
	if err != nil {
		respondError(c, err)
		return
	}
	out := ListResponse{Items: make([]Metadata, 0, limit)}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		data, err := json.Marshal(cursor{Tenant: tenant, CreatedAt: last.CreatedAt.UTC(), ID: &last.ID})
		if err != nil {
			respondError(c, err)
			return
		}
		next := base64.RawURLEncoding.EncodeToString(data)
		out.NextCursor = &next
	}
	for _, row := range rows {
		out.Items = append(out.Items, h.metadata(row))
	}
	c.JSON(http.StatusOK, out)
}

func urlQuery(c *gin.Context) (map[string][]string, error) {
	// ParseQuery reports malformed escaping rather than silently dropping fields.
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	for key, values := range query {
		if (key != "limit" && key != "cursor") || len(values) != 1 {
			return nil, domain.ErrInvalid
		}
	}
	return query, err
}

func validCode(code string) bool {
	id, err := idgen.Decode(code)
	return err == nil && id <= math.MaxInt64
}

func (h *Handlers) metadata(row domain.Link) Metadata {
	state := link.StateActive
	var disabledAt *time.Time
	if row.DisabledAt != nil {
		state = link.StateDisabled
		utc := row.DisabledAt.UTC()
		disabledAt = &utc
	}
	return Metadata{Code: row.Code, ShortURL: h.publicBase + "/" + row.Code, LongURL: row.LongURL, State: state, CreatedAt: row.CreatedAt.UTC(), DisabledAt: disabledAt}
}

func respondError(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, ErrorInternal
	var rate *app.RateLimited
	switch {
	case errors.As(err, &rate):
		status, message = http.StatusTooManyRequests, ErrorRateLimited
		seconds := max(1, int64(math.Ceil(rate.RetryAfter.Seconds())))
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	case errors.Is(err, domain.ErrInvalid):
		status, message = http.StatusBadRequest, ErrorInvalidRequest
	case errors.Is(err, domain.ErrNotFound):
		status, message = http.StatusNotFound, ErrorNotFound
	case errors.Is(err, domain.ErrDisabled):
		status, message = http.StatusConflict, ErrorLinkDisabled
	case errors.Is(err, domain.ErrUnavailable):
		status, message = http.StatusServiceUnavailable, ErrorUnavailable
	}
	c.JSON(status, ErrorResponse{Error: message})
}
