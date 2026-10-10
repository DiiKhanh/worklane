package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	contracts "github.com/duykhanh/worklane/pkg/contracts/link"
	"github.com/duykhanh/worklane/pkg/idgen"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

var errBoom = errors.New("boom")

const (
	tenantA = "tenant-a"
	tenantB = "tenant-b"
	topic   = "link.clicked"
)

var now = time.Date(2026, 10, 10, 15, 30, 0, 0, time.UTC)

type fakeRepo struct {
	links       map[string]domain.Link // by code
	insertErr   error
	findCodeErr error
	findCodeN   int
	// raceWinner, when set, is stored on the next Insert, which then reports a
	// duplicate: it simulates a concurrent request winning the unique index.
	raceWinner *domain.Link
	total      int64
	daily      []DayCount
	dailySince time.Time
	recent     []Click
	list       []LinkStat
}

func newFakeRepo() *fakeRepo { return &fakeRepo{links: map[string]domain.Link{}} }

func (f *fakeRepo) Insert(_ context.Context, l domain.Link) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	if f.raceWinner != nil {
		f.links[f.raceWinner.Code] = *f.raceWinner
		f.raceWinner = nil
		return domain.ErrAlreadyExists
	}
	f.links[l.Code] = l
	return nil
}
func (f *fakeRepo) FindByCode(_ context.Context, code string) (domain.Link, error) {
	f.findCodeN++
	if f.findCodeErr != nil {
		return domain.Link{}, f.findCodeErr
	}
	l, ok := f.links[code]
	if !ok {
		return domain.Link{}, domain.ErrNotFound
	}
	return l, nil
}
func (f *fakeRepo) FindByURLHash(_ context.Context, tenantID, hash string) (domain.Link, error) {
	for _, l := range f.links {
		if l.TenantID == tenantID && l.LongURLHash == hash {
			return l, nil
		}
	}
	return domain.Link{}, domain.ErrNotFound
}
func (f *fakeRepo) ListByTenant(context.Context, string, int) ([]LinkStat, error) {
	return f.list, nil
}
func (f *fakeRepo) CountClicks(context.Context, string) (int64, error) { return f.total, nil }
func (f *fakeRepo) DailyClicks(_ context.Context, _ string, since time.Time) ([]DayCount, error) {
	f.dailySince = since
	return f.daily, nil
}
func (f *fakeRepo) RecentClicks(context.Context, string, int) ([]Click, error) {
	return f.recent, nil
}

type fakeCache struct {
	m      map[string]Target
	ttl    time.Duration
	getErr error
	setErr error
}

func newFakeCache() *fakeCache { return &fakeCache{m: map[string]Target{}} }
func (f *fakeCache) Get(_ context.Context, code string) (Target, bool, error) {
	if f.getErr != nil {
		return Target{}, false, f.getErr
	}
	t, ok := f.m[code]
	return t, ok, nil
}
func (f *fakeCache) Set(_ context.Context, code string, t Target, ttl time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.m[code] = t
	f.ttl = ttl
	return nil
}

type fakeCounter struct {
	n   map[string]int64
	ttl time.Duration
	err error
}

func newFakeCounter() *fakeCounter { return &fakeCounter{n: map[string]int64{}} }
func (f *fakeCounter) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.n[key]++
	f.ttl = ttl
	return f.n[key], nil
}

type published struct {
	topic       string
	event       any
	ctxErr      error // ctx.Err() observed while publishing
	hasDeadline bool
}

// fakePub hands every publish to a channel so tests can wait for the background
// goroutine deterministically instead of sleeping.
type fakePub struct {
	ch    chan published
	err   error
	block chan struct{} // when non-nil, Publish waits for it to close
}

func newFakePub() *fakePub { return &fakePub{ch: make(chan published, 16)} }
func (f *fakePub) Publish(ctx context.Context, topic string, event any) error {
	if f.block != nil {
		<-f.block
	}
	_, hasDeadline := ctx.Deadline()
	f.ch <- published{topic: topic, event: event, ctxErr: ctx.Err(), hasDeadline: hasDeadline}
	return f.err
}

func (f *fakePub) wait(t *testing.T) published {
	t.Helper()
	select {
	case p := <-f.ch:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("no event published")
		return published{}
	}
}

type seqIDs struct {
	mu   sync.Mutex
	next int64
}

func (s *seqIDs) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return s.next
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type fixture struct {
	svc     *Service
	repo    *fakeRepo
	cache   *fakeCache
	counter *fakeCounter
	pub     *fakePub
}

func newFixture() fixture {
	f := fixture{repo: newFakeRepo(), cache: newFakeCache(), counter: newFakeCounter(), pub: newFakePub()}
	f.svc = New(Deps{
		Repo: f.repo, Cache: f.cache, Counter: f.counter, Pub: f.pub,
		IDs: &seqIDs{next: 999}, Clock: fixedClock{t: now},
	}, Config{
		PublicBase: "https://link.example.com/", CacheTTL: time.Hour, ClickedTopic: topic,
		CreateLimitMax: 3, CreateLimitWindow: time.Minute,
		MaxInflightClicks: 2, ClickPublishTimeout: time.Second,
	})
	return f
}

func (f fixture) seed(l domain.Link) {
	l.LongURLHash = domain.HashURL(l.LongURL)
	f.repo.links[l.Code] = l
}

// --- Shorten ---

func TestShorten_NewLink(t *testing.T) {
	f := newFixture()
	res, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: " https://example.com/a "})
	if err != nil {
		t.Fatal(err)
	}
	wantCode := idgen.Encode(1000)
	if res.Code != wantCode || !res.Created {
		t.Fatalf("res = %+v, want code %s created", res, wantCode)
	}
	if res.ShortURL != "https://link.example.com/"+wantCode {
		t.Fatalf("short url = %s (base trailing slash must not double up)", res.ShortURL)
	}

	l := f.repo.links[wantCode]
	if l.ID != 1000 || l.TenantID != tenantA || l.LongURL != "https://example.com/a" ||
		l.LongURLHash != domain.HashURL("https://example.com/a") || !l.CreatedAt.Equal(now) {
		t.Fatalf("stored link = %+v", l)
	}
	if got := f.cache.m[wantCode]; got != (Target{LongURL: "https://example.com/a", TenantID: tenantA}) {
		t.Fatalf("cache not warmed: %+v", got)
	}
	if f.cache.ttl != time.Hour {
		t.Fatalf("cache ttl = %v", f.cache.ttl)
	}
}

func TestShorten_DedupReturnsExistingCode(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	first, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantA, LongURL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantA, LongURL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Code != first.Code || again.ShortURL != first.ShortURL || again.Created {
		t.Fatalf("dedup: first %+v, again %+v", first, again)
	}
	if len(f.repo.links) != 1 {
		t.Fatalf("links = %d, want 1", len(f.repo.links))
	}
}

func TestShorten_DedupIsPerTenant(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	a, _ := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantA, LongURL: "https://example.com/a"})
	b, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantB, LongURL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Code == b.Code || !b.Created {
		t.Fatalf("tenants must not share a code: a %+v, b %+v", a, b)
	}
}

func TestShorten_ConcurrentDuplicateReturnsWinner(t *testing.T) {
	f := newFixture()
	url := "https://example.com/race"
	f.repo.raceWinner = &domain.Link{
		ID: 7, Code: "winner", TenantID: tenantA, LongURL: url, LongURLHash: domain.HashURL(url),
	}
	res, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: url})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != "winner" || res.Created {
		t.Fatalf("res = %+v, want the winning row's code", res)
	}
	if _, cached := f.cache.m[idgen.Encode(1000)]; cached {
		t.Fatal("the losing code must not be cached")
	}
}

func TestShorten_InvalidURL(t *testing.T) {
	f := newFixture()
	_, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: "javascript:alert(1)"})
	if !errors.Is(err, domain.ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
	if len(f.counter.n) != 0 {
		t.Fatal("an invalid url must not consume the create quota")
	}
}

func TestShorten_RateLimitedPerTenant(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	for i, u := range []string{"https://e.com/1", "https://e.com/2", "https://e.com/3"} {
		if _, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantA, LongURL: u}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	_, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantA, LongURL: "https://e.com/4"})
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if len(f.repo.links) != 3 {
		t.Fatalf("links = %d, want 3", len(f.repo.links))
	}
	if f.counter.ttl != time.Minute {
		t.Fatalf("limit window = %v", f.counter.ttl)
	}
	if _, err := f.svc.Shorten(ctx, ShortenInput{TenantID: tenantB, LongURL: "https://e.com/4"}); err != nil {
		t.Fatalf("another tenant must have its own quota: %v", err)
	}
}

func TestShorten_CacheWarmFailureIsNotFatal(t *testing.T) {
	f := newFixture()
	f.cache.setErr = errBoom
	res, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: "https://example.com/a"})
	if err != nil || !res.Created {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestShorten_PropagatesInfraErrors(t *testing.T) {
	t.Run("counter", func(t *testing.T) {
		f := newFixture()
		f.counter.err = errBoom
		if _, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: "https://e.com"}); !errors.Is(err, errBoom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("insert", func(t *testing.T) {
		f := newFixture()
		f.repo.insertErr = errBoom
		if _, err := f.svc.Shorten(context.Background(), ShortenInput{TenantID: tenantA, LongURL: "https://e.com"}); !errors.Is(err, errBoom) {
			t.Fatalf("err = %v", err)
		}
		if len(f.cache.m) != 0 {
			t.Fatal("a failed insert must not warm the cache")
		}
	})
}

// --- Resolve ---

func TestResolve_CacheHitSkipsDBAndPublishesClick(t *testing.T) {
	f := newFixture()
	f.cache.m["abc"] = Target{LongURL: "https://example.com/a", TenantID: tenantA}

	got, err := f.svc.Resolve(context.Background(), ResolveInput{
		Code: "abc", Referer: "https://ref.example", UA: "curl/8", ClientIP: "203.0.113.7",
	})
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if f.repo.findCodeN != 0 {
		t.Fatal("a cache hit must not read the DB")
	}

	p := f.pub.wait(t)
	evt, ok := p.event.(contracts.ClickedEvent)
	if !ok || p.topic != topic {
		t.Fatalf("published %T on %s", p.event, p.topic)
	}
	want := contracts.ClickedEvent{
		Code: "abc", TenantID: tenantA, TS: now, Referer: "https://ref.example", UA: "curl/8",
		IPHash: domain.HashIP("203.0.113.7"),
	}
	if evt != want {
		t.Fatalf("event = %+v, want %+v", evt, want)
	}
}

func TestResolve_CacheMissReadsDBAndPopulatesCache(t *testing.T) {
	f := newFixture()
	f.seed(domain.Link{ID: 1, Code: "abc", TenantID: tenantA, LongURL: "https://example.com/a"})
	ctx := context.Background()

	got, err := f.svc.Resolve(ctx, ResolveInput{Code: "abc"})
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if f.cache.m["abc"] != (Target{LongURL: "https://example.com/a", TenantID: tenantA}) || f.cache.ttl != time.Hour {
		t.Fatalf("cache not populated: %+v ttl %v", f.cache.m["abc"], f.cache.ttl)
	}
	if evt := f.pub.wait(t).event.(contracts.ClickedEvent); evt.TenantID != tenantA || evt.IPHash != "" {
		t.Fatalf("event = %+v", evt)
	}

	if _, err := f.svc.Resolve(ctx, ResolveInput{Code: "abc"}); err != nil {
		t.Fatal(err)
	}
	f.pub.wait(t)
	if f.repo.findCodeN != 1 {
		t.Fatalf("db reads = %d, want 1 (the repeat must be served from cache)", f.repo.findCodeN)
	}
}

func TestResolve_UnknownCode(t *testing.T) {
	f := newFixture()
	_, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "nope"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(f.cache.m) != 0 {
		t.Fatal("a miss must not be cached (no negative caching)")
	}
	select {
	case p := <-f.pub.ch:
		t.Fatalf("unknown code published %+v", p)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestResolve_MalformedCodeSkipsLookups(t *testing.T) {
	f := newFixture()
	f.cache.getErr = errBoom // would be logged if the cache were consulted
	_, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "favicon.ico"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if f.repo.findCodeN != 0 {
		t.Fatal("a malformed code must not reach the DB")
	}
}

func TestResolve_CacheErrorsDegradeToDB(t *testing.T) {
	f := newFixture()
	f.seed(domain.Link{ID: 1, Code: "abc", TenantID: tenantA, LongURL: "https://example.com/a"})
	f.cache.getErr = errBoom
	f.cache.setErr = errBoom
	got, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "abc"})
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("got %q, err %v", got, err)
	}
	f.pub.wait(t)
}

func TestResolve_DBErrorIsNotReportedAsNotFound(t *testing.T) {
	f := newFixture()
	f.repo.findCodeErr = errBoom
	_, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "abc"})
	if !errors.Is(err, errBoom) || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
}

func TestResolve_PublishFailureDoesNotFailRedirect(t *testing.T) {
	f := newFixture()
	f.pub.err = errBoom
	f.cache.m["abc"] = Target{LongURL: "https://example.com/a", TenantID: tenantA}
	got, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "abc"})
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("got %q, err %v", got, err)
	}
	f.pub.wait(t)
}

func TestResolve_PublishOutlivesRequestContext(t *testing.T) {
	f := newFixture()
	f.pub.block = make(chan struct{})
	f.cache.m["abc"] = Target{LongURL: "https://example.com/a", TenantID: tenantA}

	ctx, cancel := context.WithCancel(context.Background())
	if _, err := f.svc.Resolve(ctx, ResolveInput{Code: "abc"}); err != nil {
		t.Fatal(err)
	}
	cancel() // the handler has written the 302 and returned
	close(f.pub.block)

	p := f.pub.wait(t)
	if p.ctxErr != nil {
		t.Fatalf("publish context died with the request: %v", p.ctxErr)
	}
	if !p.hasDeadline {
		t.Fatal("publish context must carry the publish timeout")
	}
}

func TestResolve_DropsClicksWhenPublisherIsSaturated(t *testing.T) {
	f := newFixture() // MaxInflightClicks: 2
	f.pub.block = make(chan struct{})
	f.cache.m["abc"] = Target{LongURL: "https://example.com/a", TenantID: tenantA}

	for i := 0; i < 5; i++ {
		got, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "abc"})
		if err != nil || got != "https://example.com/a" {
			t.Fatalf("redirect %d must not block or fail: %q %v", i, got, err)
		}
	}
	close(f.pub.block)
	f.pub.wait(t)
	f.pub.wait(t)
	select {
	case <-f.pub.ch:
		t.Fatal("more publishes than MaxInflightClicks were started")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestResolve_TruncatesOversizedHeaders(t *testing.T) {
	f := newFixture()
	f.cache.m["abc"] = Target{LongURL: "https://example.com/a", TenantID: tenantA}
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := f.svc.Resolve(context.Background(), ResolveInput{Code: "abc", Referer: string(long), UA: string(long)}); err != nil {
		t.Fatal(err)
	}
	evt := f.pub.wait(t).event.(contracts.ClickedEvent)
	if len(evt.Referer) != domain.MaxClickMetaLen || len(evt.UA) != domain.MaxClickMetaLen {
		t.Fatalf("referer %d, ua %d, want %d", len(evt.Referer), len(evt.UA), domain.MaxClickMetaLen)
	}
}

// --- ListLinks / LinkDetail ---

func TestListLinks_MapsStats(t *testing.T) {
	f := newFixture()
	created := now.Add(-time.Hour)
	f.repo.list = []LinkStat{
		{Link: domain.Link{Code: "abc", LongURL: "https://example.com/a", CreatedAt: created}, Clicks: 12},
		{Link: domain.Link{Code: "def", LongURL: "https://example.com/b", CreatedAt: created}},
	}
	got, err := f.svc.ListLinks(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	want := []LinkSummary{
		{Code: "abc", Target: "https://example.com/a", Clicks: 12, CreatedAt: created},
		{Code: "def", Target: "https://example.com/b", Clicks: 0, CreatedAt: created},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListLinks_EmptyIsNonNil(t *testing.T) {
	f := newFixture()
	got, err := f.svc.ListLinks(context.Background(), tenantA)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, err %v (must be an empty slice so JSON renders [])", got, err)
	}
}

func TestLinkDetail_AggregatesSeriesAndRecent(t *testing.T) {
	f := newFixture()
	created := now.Add(-48 * time.Hour)
	f.seed(domain.Link{ID: 1, Code: "abc", TenantID: tenantA, LongURL: "https://example.com/a", CreatedAt: created})
	today := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	f.repo.total = 40
	f.repo.daily = []DayCount{
		{Day: today, Clicks: 5},
		{Day: today.AddDate(0, 0, -1), Clicks: 3},
		{Day: today.AddDate(0, 0, -13), Clicks: 7},
		{Day: today.AddDate(0, 0, -14), Clicks: 99}, // outside the window: ignored
		{Day: today.AddDate(0, 0, 1), Clicks: 99},   // future (clock skew): ignored
	}
	clickTS := now.Add(-time.Minute)
	f.repo.recent = []Click{
		{TS: clickTS, Referer: "https://ref.example", UA: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X)"},
		{TS: clickTS.Add(-time.Minute), UA: "curl/8"},
	}

	got, err := f.svc.LinkDetail(context.Background(), tenantA, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "abc" || got.Target != "https://example.com/a" || got.Clicks != 40 || !got.CreatedAt.Equal(created) {
		t.Fatalf("detail = %+v", got)
	}
	wantSeries := []int64{7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 5}
	if len(got.Series) != SeriesDays {
		t.Fatalf("series len = %d", len(got.Series))
	}
	for i := range wantSeries {
		if got.Series[i] != wantSeries[i] {
			t.Fatalf("series = %v, want %v", got.Series, wantSeries)
		}
	}
	if want := today.AddDate(0, 0, -13); !f.repo.dailySince.Equal(want) {
		t.Fatalf("series window starts %v, want %v", f.repo.dailySince, want)
	}
	wantRecent := []RecentClick{
		{TS: clickTS, Referer: "https://ref.example", Device: domain.DeviceIOS},
		{TS: clickTS.Add(-time.Minute), Device: domain.DeviceOther},
	}
	if len(got.Recent) != 2 || got.Recent[0] != wantRecent[0] || got.Recent[1] != wantRecent[1] {
		t.Fatalf("recent = %+v, want %+v", got.Recent, wantRecent)
	}
}

func TestLinkDetail_NoClicksYieldsZeroSeriesAndEmptyRecent(t *testing.T) {
	f := newFixture()
	f.seed(domain.Link{ID: 1, Code: "abc", TenantID: tenantA, LongURL: "https://example.com/a"})
	got, err := f.svc.LinkDetail(context.Background(), tenantA, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Series) != SeriesDays || got.Recent == nil || len(got.Recent) != 0 || got.Clicks != 0 {
		t.Fatalf("detail = %+v", got)
	}
}

func TestLinkDetail_OtherTenantsLinkIsNotFound(t *testing.T) {
	f := newFixture()
	f.seed(domain.Link{ID: 1, Code: "abc", TenantID: tenantA, LongURL: "https://example.com/a"})
	for _, tc := range []struct{ tenant, code string }{{tenantB, "abc"}, {tenantA, "missing"}, {tenantA, "bad code"}} {
		if _, err := f.svc.LinkDetail(context.Background(), tc.tenant, tc.code); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("LinkDetail(%s, %q) err = %v, want ErrNotFound", tc.tenant, tc.code, err)
		}
	}
}
