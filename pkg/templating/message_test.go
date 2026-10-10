package templating

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func fakeShortener(calls *[]string) Shortener {
	return func(_ context.Context, long string) (string, error) {
		*calls = append(*calls, long)
		return "https://s.test/" + string(rune('a'+len(*calls)-1)), nil
	}
}

func TestRenderMessageSubstitutesVars(t *testing.T) {
	msg, err := RenderMessage(context.Background(), "Hi {{name}}", "Order {{ order_id }} for {{name}}.",
		map[string]string{"name": "An", "order_id": "42"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "Hi An" || msg.Body != "Order 42 for An." {
		t.Fatalf("got %+v", msg)
	}
	if len(msg.Missing) != 0 {
		t.Fatalf("nothing should be missing, got %v", msg.Missing)
	}
}

func TestRenderMessageMissingVarsRenderEmptyAndAreReported(t *testing.T) {
	msg, err := RenderMessage(context.Background(), "{{zeta}}", "a{{name}}b {{name}} {{alpha}} {{blank}}",
		map[string]string{"blank": ""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "" || msg.Body != "ab   " {
		t.Fatalf("got %+v", msg)
	}
	if want := []string{"alpha", "name", "zeta"}; !reflect.DeepEqual(msg.Missing, want) {
		t.Fatalf("missing: got %v want %v", msg.Missing, want)
	}
}

// A substituted value is never re-scanned, so user-supplied variables cannot smuggle in
// another variable or a link directive.
func TestRenderMessageIsInjectionSafe(t *testing.T) {
	var calls []string
	msg, err := RenderMessage(context.Background(), "", "{{a}} {{b}}",
		map[string]string{"a": `{{b}} {{link "https://evil.test"}}`, "b": "safe"}, fakeShortener(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{{b}} {{link "https://evil.test"}} safe`; msg.Body != want {
		t.Fatalf("body: got %q want %q", msg.Body, want)
	}
	if len(calls) != 0 {
		t.Fatalf("shortener must not see injected links, got %v", calls)
	}
}

func TestRenderMessageShortensLinksOncePerURL(t *testing.T) {
	var calls []string
	msg, err := RenderMessage(context.Background(),
		`See {{link "https://shop.test/a"}}`,
		`Go {{ link "https://shop.test/a" }} or {{link "https://shop.test/b"}}, {{name}}`,
		map[string]string{"name": "An"}, fakeShortener(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"https://shop.test/a", "https://shop.test/b"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("shorten calls: got %v want %v", calls, want)
	}
	if msg.Subject != "See https://s.test/a" || msg.Body != "Go https://s.test/a or https://s.test/b, An" {
		t.Fatalf("got %+v", msg)
	}
}

func TestRenderMessageLinkWithoutShortener(t *testing.T) {
	_, err := RenderMessage(context.Background(), "", `{{link "https://shop.test"}}`, nil, nil)
	if !errors.Is(err, ErrNoShortener) {
		t.Fatalf("want ErrNoShortener, got %v", err)
	}
}

func TestRenderMessagePropagatesShortenerError(t *testing.T) {
	boom := errors.New("link-svc down")
	_, err := RenderMessage(context.Background(), "", `{{link "https://shop.test"}}`, nil,
		func(context.Context, string) (string, error) { return "", boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want wrapped shortener error, got %v", err)
	}
}

func TestVariables(t *testing.T) {
	got := Variables(`{{name}} {{link "https://x.test"}}`, "{{order_id}} {{ name }} {{link}}")
	if want := []string{"link", "name", "order_id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := Variables("", "plain"); len(got) != 0 {
		t.Fatalf("plain text has no variables, got %v", got)
	}
}

func TestValidateMessage(t *testing.T) {
	cases := []struct {
		name, channel, subject, body string
		want                         error
	}{
		{"valid email", "email", "Hi {{name}}", `Go {{link "https://shop.test/a?x=1"}}`, nil},
		{"valid sms without subject", "sms", "", "Hi {{any_name}}", nil},
		{"email needs subject", "email", "", "body", ErrSubjectRequired},
		{"unquoted link", "sms", "", "{{link https://shop.test}}", ErrMalformedToken},
		{"empty token", "sms", "", "{{}}", ErrMalformedToken},
		{"expression", "sms", "", "{{ .Name | upper }}", ErrMalformedToken},
		{"malformed in subject", "email", "{{a b}}", "body", ErrMalformedToken},
		{"relative link", "sms", "", `{{link "/path"}}`, ErrInvalidLink},
		{"javascript link", "sms", "", `{{link "javascript:alert(1)"}}`, ErrInvalidLink},
		{"empty link", "sms", "", `{{link ""}}`, ErrInvalidLink},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMessage(tc.channel, tc.subject, tc.body)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
