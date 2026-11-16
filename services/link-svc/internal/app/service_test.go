package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duykhanh/worklane/services/link-svc/internal/app"
	"github.com/duykhanh/worklane/services/link-svc/internal/domain"
)

type quotaResult struct{ err error }

func (q quotaResult) Take(context.Context, string) (time.Duration, error) { return 0, q.err }

type idResult struct{ err error }

func (g idResult) New() (int64, string, error) { return 42, "G", g.err }

type databaseResult struct {
	row        domain.Link
	disableErr error
}

func (r databaseResult) ByDestination(context.Context, string, string) (domain.Link, error) {
	return domain.Link{}, domain.ErrNotFound
}
func (r databaseResult) Insert(context.Context, domain.Link) (domain.Link, error) {
	return domain.Link{}, domain.ErrDuplicate
}
func (r databaseResult) Get(context.Context, string, string) (domain.Link, error) { return r.row, nil }
func (r databaseResult) List(context.Context, string, *domain.Position, int) ([]domain.Link, error) {
	return []domain.Link{r.row}, nil
}
func (r databaseResult) Disable(context.Context, string, string) (domain.Link, error) {
	return r.row, r.disableErr
}

type cachedMapping struct {
	code string
	err  error
}

func (c *cachedMapping) Evict(_ context.Context, code string) error {
	if c.err != nil {
		return c.err
	}
	if c.code == code {
		c.code = ""
	}
	return nil
}

func TestCreateFailsClosedWhenQuotaStorageFails(t *testing.T) {
	svc := app.New(nil, quotaResult{err: errors.New("storage failed with private details")}, nil, nil)
	_, _, err := svc.Create(context.Background(), "tenant", "https://example.com/")
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("quota failure must fail closed: %v", err)
	}
}

func TestCreateRejectsUnsafeIDIssuanceAndUnresolvedCollisions(t *testing.T) {
	for _, idErr := range []error{errors.New("clock rollback"), errors.New("sequence exhausted"), nil} {
		svc := app.New(databaseResult{}, quotaResult{}, nil, idResult{err: idErr})
		_, _, err := svc.Create(context.Background(), "tenant", "https://example.com/")
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("unsafe creation must be unavailable: %v", err)
		}
	}
}

func TestDisableEvictsOnlyAfterPersistenceSucceeds(t *testing.T) {
	disabledAt := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	row := domain.Link{Code: "G", TenantID: "tenant", LongURL: "https://example.com/", DisabledAt: &disabledAt}
	for _, scenario := range []struct {
		name            string
		dbErr, cacheErr error
		wantCached      bool
	}{
		{"persisted", nil, nil, false},
		{"eviction unavailable", nil, errors.New("redis unavailable"), true},
		{"database unavailable", domain.ErrUnavailable, nil, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cache := &cachedMapping{code: row.Code, err: scenario.cacheErr}
			svc := app.New(databaseResult{row: row, disableErr: scenario.dbErr}, nil, cache, nil)
			out, err := svc.Disable(context.Background(), "tenant", "G")
			if !errors.Is(err, scenario.dbErr) {
				t.Fatalf("disable result: %v", err)
			}
			if scenario.dbErr == nil && (out.DisabledAt == nil || !out.DisabledAt.Equal(disabledAt)) {
				t.Fatal("committed disable lost on cache failure")
			}
			if (cache.code != "") != scenario.wantCached {
				t.Fatal("cache entry does not reflect persistence/eviction outcome")
			}
		})
	}
}
