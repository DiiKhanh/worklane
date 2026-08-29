package app

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/duykhanh/worklane/pkg/templating"
)

type fakeTemplateRepo struct {
	publishChannel string
	publishLocale  string
	created        *Template
	addedVersion   *TemplateVersion
}

func (f *fakeTemplateRepo) List(context.Context) ([]Template, error) { return nil, nil }
func (f *fakeTemplateRepo) Get(context.Context, string) (Template, []TemplateVersion, error) {
	return Template{ID: "tpl1", Channel: "email", Locale: "en"}, nil, nil
}
func (f *fakeTemplateRepo) Create(_ context.Context, t Template, _ TemplateVersion) error {
	f.created = &t
	return nil
}
func (f *fakeTemplateRepo) AddVersion(_ context.Context, v TemplateVersion) (int, error) {
	f.addedVersion = &v
	return 2, nil
}
func (f *fakeTemplateRepo) Publish(context.Context, string, string) (string, string, error) {
	return f.publishChannel, f.publishLocale, nil
}

type fakeCache struct{ invalidated string }

func (f *fakeCache) Invalidate(_ context.Context, channel, locale string) error {
	f.invalidated = channel + "/" + locale
	return nil
}

type fixedID struct{ n int }

func (g *fixedID) New() string { g.n++; return "id" + strconv.Itoa(g.n) }

func newTemplateSvc(repo TemplateRepo, cache TemplateCache) *TemplateService {
	return NewTemplateService(repo, cache, fixedClock{t: time.Unix(1000, 0)}, &fixedID{})
}

func TestCreateRejectsUnknownVariable(t *testing.T) {
	svc := newTemplateSvc(&fakeTemplateRepo{}, &fakeCache{})
	_, err := svc.Create(context.Background(), CreateTemplateInput{
		Name: "Welcome", Channel: "email", Locale: "en",
		Subject: "Hi", Body: "Hello {{name}}", Author: "a@b.co",
	})
	if !errors.Is(err, templating.ErrUnknownVariable) {
		t.Fatalf("want ErrUnknownVariable, got %v", err)
	}
}

func TestPublishInvalidatesCache(t *testing.T) {
	repo := &fakeTemplateRepo{publishChannel: "email", publishLocale: "en"}
	cache := &fakeCache{}
	svc := newTemplateSvc(repo, cache)
	if err := svc.Publish(context.Background(), "tpl1", "ver2"); err != nil {
		t.Fatal(err)
	}
	if cache.invalidated != "email/en" {
		t.Fatalf("publish must invalidate the (channel,locale) cache key, got %q", cache.invalidated)
	}
}

func TestPreviewRendersThroughEngine(t *testing.T) {
	svc := newTemplateSvc(&fakeTemplateRepo{}, &fakeCache{})
	sub, body := svc.Preview("Code {{code}}", "It is {{code}}, expires {{expiry}}.")
	if sub != "Code 123456" || body != "It is 123456, expires 5 minutes." {
		t.Fatalf("preview mismatch: %q / %q", sub, body)
	}
}
