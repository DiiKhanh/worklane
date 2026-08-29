package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/templating"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// TemplateAPI is the inbound port the template endpoints need. Defining it here (at the
// point of use) keeps this adapter decoupled from the concrete *app.TemplateService.
type TemplateAPI interface {
	List(ctx context.Context) ([]app.Template, error)
	Get(ctx context.Context, id string) (app.Template, []app.TemplateVersion, error)
	Create(ctx context.Context, in app.CreateTemplateInput) (app.Template, error)
	AddVersion(ctx context.Context, in app.AddVersionInput) (app.TemplateVersion, error)
	Publish(ctx context.Context, templateID, versionID string) error
	Preview(subject, body string) (string, string)
}

// TemplateHandlers holds the template inbound port.
type TemplateHandlers struct{ svc TemplateAPI }

// author returns the JWT email that owns this request. Template management is a human
// (dashboard) action: an API-key caller has no actor email and is rejected with 403.
func author(c *gin.Context) (string, bool) {
	email := c.GetString(authorCtxKey)
	if email == "" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "template management requires a user login"})
		return "", false
	}
	return email, true
}

func (h *TemplateHandlers) List(c *gin.Context) {
	if _, ok := author(c); !ok {
		return
	}
	rows, err := h.svc.List(c.Request.Context())
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	out := make([]templateDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTemplateDTO(t))
	}
	c.JSON(http.StatusOK, out)
}

func (h *TemplateHandlers) Get(c *gin.Context) {
	if _, ok := author(c); !ok {
		return
	}
	t, versions, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	vs := make([]versionDTO, 0, len(versions))
	for _, v := range versions {
		vs = append(vs, toVersionDTO(v))
	}
	c.JSON(http.StatusOK, templateDetailDTO{Template: toTemplateDTO(t), Versions: vs})
}

func (h *TemplateHandlers) Create(c *gin.Context) {
	email, ok := author(c)
	if !ok {
		return
	}
	var body createTemplateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.svc.Create(c.Request.Context(), app.CreateTemplateInput{
		Name: body.Name, Channel: body.Channel, Locale: body.Locale,
		Subject: body.Subject, Body: body.Body, Note: body.Note, Author: email,
	})
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toTemplateDTO(t))
}

func (h *TemplateHandlers) AddVersion(c *gin.Context) {
	email, ok := author(c)
	if !ok {
		return
	}
	var body addVersionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.svc.AddVersion(c.Request.Context(), app.AddVersionInput{
		TemplateID: c.Param("id"), Subject: body.Subject, Body: body.Body, Note: body.Note, Author: email,
	})
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toVersionDTO(v))
}

func (h *TemplateHandlers) Publish(c *gin.Context) {
	if _, ok := author(c); !ok {
		return
	}
	if err := h.svc.Publish(c.Request.Context(), c.Param("id"), c.Param("vid")); err != nil {
		writeTemplateError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TemplateHandlers) Preview(c *gin.Context) {
	if _, ok := author(c); !ok {
		return
	}
	var body previewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := templating.Validate(body.Channel, body.Subject, body.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subject, rendered := h.svc.Preview(body.Subject, body.Body)
	c.JSON(http.StatusOK, previewResponse{Subject: subject, Body: rendered})
}

func toTemplateDTO(t app.Template) templateDTO {
	return templateDTO{
		ID: t.ID, Name: t.Name, Channel: t.Channel, Locale: t.Locale, Status: t.Status,
		ActiveVersionID: t.ActiveVersionID, UpdatedAt: t.UpdatedAt.UTC(),
	}
}

func toVersionDTO(v app.TemplateVersion) versionDTO {
	return versionDTO{
		ID: v.ID, VersionNo: v.VersionNo, Subject: v.Subject, Body: v.Body, Status: v.Status,
		Note: v.Note, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt.UTC(),
	}
}

func writeTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, app.ErrTemplateNotFound), errors.Is(err, app.ErrVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrTemplateExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, templating.ErrUnknownVariable), errors.Is(err, templating.ErrSubjectRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
