package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/auth-svc/internal/domain"
)

type fakeRepo struct {
	u   domain.User
	err error
}

func (f fakeRepo) FindUserByEmail(context.Context, string) (domain.User, error) {
	return f.u, f.err
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(_, _, _ string, now time.Time) (string, time.Time, error) {
	return "tok", now.Add(time.Hour), nil
}

type allowLimiter struct{ ok bool }

func (a allowLimiter) Allow(context.Context, string) (bool, error) { return a.ok, nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }

func newSvc(repo Repo, allow bool) *Service {
	return New(Deps{Repo: repo, Issuer: fakeIssuer{}, Limiter: allowLimiter{ok: allow}, Clock: fixedClock{}})
}

func activeUser(t *testing.T, pw string) domain.User {
	t.Helper()
	h, _ := security.HashPassword(pw)
	return domain.User{ID: "u1", TenantID: "t1", Email: "a@b.co", PasswordHash: h, Status: "active"}
}

func TestLogin_Success(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, true)
	res, err := svc.Login(context.Background(), "a@b.co", "pw")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Token != "tok" || res.User.TenantID != "t1" {
		t.Fatalf("result wrong: %+v", res)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, true)
	_, err := svc.Login(context.Background(), "a@b.co", "nope")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownEmail_Generic(t *testing.T) {
	svc := newSvc(fakeRepo{err: domain.ErrUserNotFound}, true)
	_, err := svc.Login(context.Background(), "x@y.co", "pw")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown email must map to generic ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	svc := newSvc(fakeRepo{u: activeUser(t, "pw")}, false)
	_, err := svc.Login(context.Background(), "a@b.co", "pw")
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestLogin_Inactive(t *testing.T) {
	u := activeUser(t, "pw")
	u.Status = "disabled"
	svc := newSvc(fakeRepo{u: u}, true)
	_, err := svc.Login(context.Background(), "a@b.co", "pw")
	if !errors.Is(err, domain.ErrInactive) {
		t.Fatalf("want ErrInactive, got %v", err)
	}
}
