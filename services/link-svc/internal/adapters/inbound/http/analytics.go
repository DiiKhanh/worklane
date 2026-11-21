package http

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

const analyticsReadTimeout = 2 * time.Second

type Analytics interface {
	Analytics(context.Context, string, string) (domain.Analytics, error)
}

func analytics(reader Analytics) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !validCode(c.Param("code")) {
			respondError(c, domain.ErrNotFound)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), analyticsReadTimeout)
		defer cancel()
		result, err := reader.Analytics(ctx, c.GetString(tenantCtxKey), c.Param("code"))
		if err != nil {
			respondError(c, err)
			return
		}
		out := AnalyticsResponse{TotalClicks: result.TotalClicks, Series: make([]DailyClicks, 0, len(result.Series)), Recent: make([]Visit, 0, len(result.Recent))}
		for _, day := range result.Series {
			out.Series = append(out.Series, DailyClicks{Date: day.Date, Clicks: day.Clicks})
		}
		for _, visit := range result.Recent {
			out.Recent = append(out.Recent, Visit{TS: visit.TS.UTC(), ReferrerOrigin: visit.ReferrerOrigin, Device: link.Device(visit.Device)})
		}
		c.JSON(200, out)
	}
}
