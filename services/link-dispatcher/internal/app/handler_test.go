package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
)

type fakeRepo struct {
	clicks []Click
	err    error
}

func (f *fakeRepo) InsertClick(_ context.Context, c Click) error {
	if f.err != nil {
		return f.err
	}
	f.clicks = append(f.clicks, c)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var (
	now    = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	ipHash = strings.Repeat("a", 64)
)

func newHandler(repo *fakeRepo) *Handler {
	return NewHandler(Deps{Repo: repo, Clock: fixedClock{t: now}})
}

func validEvent() contracts.ClickedEvent {
	return contracts.ClickedEvent{
		Code: "aB3xYz", TenantID: "11111111-1111-1111-1111-111111111111", TS: now.Add(-time.Minute),
		Referer: "https://news.example.com/", UA: "Mozilla/5.0 (iPhone)", IPHash: ipHash,
	}
}

func TestHandlePersistsClick(t *testing.T) {
	repo := &fakeRepo{}
	evt := validEvent()
	if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := Click{Code: evt.Code, TenantID: evt.TenantID, TS: evt.TS, Referer: evt.Referer, UA: evt.UA, IPHash: ipHash}
	if len(repo.clicks) != 1 || repo.clicks[0] != want {
		t.Fatalf("clicks = %+v, want [%+v]", repo.clicks, want)
	}
}

func TestHandleNormalizesTimestampToUTC(t *testing.T) {
	repo := &fakeRepo{}
	evt := validEvent()
	evt.TS = now.In(time.FixedZone("ICT", 7*3600))
	if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := repo.clicks[0].TS
	if got.Location() != time.UTC || !got.Equal(now) {
		t.Fatalf("ts = %v, want %v in UTC", got, now)
	}
}

func TestHandleZeroTimestampFallsBackToClock(t *testing.T) {
	repo := &fakeRepo{}
	evt := validEvent()
	evt.TS = time.Time{}
	if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := repo.clicks[0].TS; !got.Equal(now) {
		t.Fatalf("ts = %v, want clock time %v", got, now)
	}
}

func TestHandleTruncatesOversizedMeta(t *testing.T) {
	repo := &fakeRepo{}
	evt := validEvent()
	evt.Referer = strings.Repeat("r", 300)
	evt.UA = strings.Repeat("é", 300) // 2 bytes per rune
	if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := repo.clicks[0]
	if len(got.Referer) != 255 {
		t.Errorf("referer length = %d, want 255", len(got.Referer))
	}
	if n := utf8.RuneCountInString(got.UA); n != 255 || !utf8.ValidString(got.UA) {
		t.Errorf("ua = %d runes (valid=%v), want 255 valid runes", n, utf8.ValidString(got.UA))
	}
}

func TestHandleKeepsMultiByteMetaWithinRuneLimit(t *testing.T) {
	repo := &fakeRepo{}
	evt := validEvent()
	evt.UA = strings.Repeat("é", 200) // 400 bytes but only 200 characters
	if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := repo.clicks[0].UA; got != evt.UA {
		t.Errorf("ua was truncated to %d runes, want it untouched", utf8.RuneCountInString(got))
	}
}

func TestHandleDropsMalformedIPHash(t *testing.T) {
	for name, hash := range map[string]string{"empty": "", "short": "abc", "long": strings.Repeat("a", 65)} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			evt := validEvent()
			evt.IPHash = hash
			if err := newHandler(repo).Handle(context.Background(), evt); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if got := repo.clicks[0].IPHash; got != "" {
				t.Errorf("ip hash = %q, want empty", got)
			}
		})
	}
}

func TestHandleRejectsInvalidEvent(t *testing.T) {
	cases := map[string]func(*contracts.ClickedEvent){
		"empty code":      func(e *contracts.ClickedEvent) { e.Code = "" },
		"code too long":   func(e *contracts.ClickedEvent) { e.Code = strings.Repeat("a", 17) },
		"code not base62": func(e *contracts.ClickedEvent) { e.Code = "ab-cd" },
		"empty tenant":    func(e *contracts.ClickedEvent) { e.TenantID = "" },
		"tenant too long": func(e *contracts.ClickedEvent) { e.TenantID = strings.Repeat("t", 37) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			evt := validEvent()
			mutate(&evt)
			err := newHandler(repo).Handle(context.Background(), evt)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("err = %v, want ErrInvalidEvent", err)
			}
			if len(repo.clicks) != 0 {
				t.Fatalf("invalid event was persisted: %+v", repo.clicks)
			}
		})
	}
}

func TestHandleReturnsRepoErrorForRedelivery(t *testing.T) {
	boom := errors.New("boom")
	err := newHandler(&fakeRepo{err: boom}).Handle(context.Background(), validEvent())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the repo error", err)
	}
	if errors.Is(err, ErrInvalidEvent) {
		t.Fatal("a repo failure must stay retryable, not be reported as an invalid event")
	}
}
