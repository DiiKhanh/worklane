package http

import (
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

// Request/response bodies for the link HTTP API. `binding` tags drive gin's validation
// at the boundary - a bad body is rejected with 400 before any use case runs.

type createRequest struct {
	LongURL string `json:"long_url" binding:"required"`
}

type createResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

type linkDTO struct {
	Code    string    `json:"code"`
	Target  string    `json:"target"`
	Clicks  int64     `json:"clicks"`
	Created time.Time `json:"created"`
}

type clickDTO struct {
	TS     time.Time `json:"ts"`
	Ref    string    `json:"ref"`
	Geo    string    `json:"geo"`
	Device string    `json:"device"`
}

type linkDetailDTO struct {
	Code    string     `json:"code"`
	Target  string     `json:"target"`
	Clicks  int64      `json:"clicks"`
	Created time.Time  `json:"created"`
	Series  []int64    `json:"series"` // daily clicks, oldest first, today (UTC) last
	Recent  []clickDTO `json:"recent"`
}

func linksFromApp(links []app.LinkSummary) []linkDTO {
	out := make([]linkDTO, 0, len(links))
	for _, l := range links {
		out = append(out, linkDTO{Code: l.Code, Target: l.Target, Clicks: l.Clicks, Created: l.CreatedAt.UTC()})
	}
	return out
}

func detailFromApp(d app.LinkDetail) linkDetailDTO {
	// Non-nil slices so an empty series/recent serializes as [] rather than null.
	series := append([]int64{}, d.Series...)
	recent := make([]clickDTO, 0, len(d.Recent))
	for _, c := range d.Recent {
		recent = append(recent, clickDTO{TS: c.TS.UTC(), Ref: c.Referer, Geo: c.Geo, Device: c.Device})
	}
	return linkDetailDTO{
		Code: d.Code, Target: d.Target, Clicks: d.Clicks, Created: d.CreatedAt.UTC(),
		Series: series, Recent: recent,
	}
}
