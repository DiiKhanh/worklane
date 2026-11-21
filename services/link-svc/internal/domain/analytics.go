package domain

import (
	"time"

	"github.com/duykhanh/worklane/pkg/contracts/link"
)

const (
	AnalyticsDays    = 14
	RecentVisitLimit = 20
)

type Analytics struct {
	TotalClicks int64
	Series      []DailyClicks
	Recent      []Visit
}

type DailyClicks struct {
	Date   string
	Clicks int64
}

type Visit struct {
	TS             time.Time
	ReferrerOrigin string
	Device         link.Device
}
