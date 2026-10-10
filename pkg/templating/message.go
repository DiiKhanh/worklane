package templating

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
)

var (
	ErrMalformedToken = errors.New("templating: malformed token")
	ErrInvalidLink    = errors.New("templating: link must be an absolute http(s) URL")
	ErrNoShortener    = errors.New("templating: template has a link but no shortener is configured")
)

// Shortener turns the long URL of a {{link "..."}} directive into a short URL. The
// notification worker and the preview endpoint pass the same link-svc backed
// implementation, which is what keeps a preview identical to the delivered message.
type Shortener func(ctx context.Context, longURL string) (shortURL string, err error)

// Message is a fully rendered notification. Missing lists (sorted, de-duplicated) the
// variables the template referenced that the caller did not supply; they rendered empty.
type Message struct {
	Subject string
	Body    string
	Missing []string
}

// messageTokenRe matches the two notification token forms, with optional inner
// whitespace: {{ link "https://..." }} (group 1 = URL) and {{ name }} (group 2 = name).
var messageTokenRe = regexp.MustCompile(`\{\{\s*(?:link\s+"([^"]*)"|([a-zA-Z0-9_]+))\s*\}\}`)

// looseTokenRe matches anything that looks like a token, so ValidateMessage can reject
// the ones messageTokenRe would silently leave in the delivered text.
var looseTokenRe = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// lineBreakRe matches a run of line breaks, which a single-line subject cannot carry.
var lineBreakRe = regexp.MustCompile(`[\r\n]+`)

// RenderMessage substitutes every {{var}} with its value from vars and every
// {{link "url"}} with the short URL returned by shorten. Substitution is a single
// literal pass: a substituted value is never scanned again, so a variable whose value
// contains "{{...}}" cannot inject a token. A variable absent from vars renders empty
// and is reported in Message.Missing. Each distinct URL is shortened once per render.
// The subject is a single header line, so line breaks in it (from the template or from
// a variable) collapse to a space: a variable cannot add mail headers.
func RenderMessage(ctx context.Context, subject, body string, vars map[string]string, shorten Shortener) (Message, error) {
	short := map[string]string{}
	for _, field := range []string{subject, body} {
		for _, m := range messageTokenRe.FindAllStringSubmatch(field, -1) {
			long := m[1]
			if m[2] != "" {
				continue
			}
			if _, done := short[long]; done {
				continue
			}
			if shorten == nil {
				return Message{}, ErrNoShortener
			}
			s, err := shorten(ctx, long)
			if err != nil {
				return Message{}, fmt.Errorf("templating: shorten link: %w", err)
			}
			short[long] = s
		}
	}

	missing := map[string]struct{}{}
	repl := func(s string) string {
		return messageTokenRe.ReplaceAllStringFunc(s, func(tok string) string {
			m := messageTokenRe.FindStringSubmatch(tok)
			if m[2] == "" {
				return short[m[1]]
			}
			val, ok := vars[m[2]]
			if !ok {
				missing[m[2]] = struct{}{}
			}
			return val
		})
	}
	msg := Message{Subject: lineBreakRe.ReplaceAllString(repl(subject), " "), Body: repl(body)}
	for name := range missing {
		msg.Missing = append(msg.Missing, name)
	}
	sort.Strings(msg.Missing)
	return msg, nil
}

// Variables returns the sorted, de-duplicated variable names a template references.
func Variables(subject, body string) []string {
	seen := map[string]struct{}{}
	names := []string{}
	for _, field := range []string{subject, body} {
		for _, m := range messageTokenRe.FindAllStringSubmatch(field, -1) {
			if _, dup := seen[m[2]]; m[2] == "" || dup {
				continue
			}
			seen[m[2]] = struct{}{}
			names = append(names, m[2])
		}
	}
	sort.Strings(names)
	return names
}

// ValidateMessage enforces the authoring rules for notification templates: email
// requires a subject, every {{...}} must be a well-formed variable or link directive,
// and every link target must be an absolute http(s) URL. Variable names are free-form
// (the sender supplies them), so unlike Validate there is no allowlist.
func ValidateMessage(channel, subject, body string) error {
	if channel == "email" && subject == "" {
		return ErrSubjectRequired
	}
	for _, field := range []string{subject, body} {
		for _, tok := range looseTokenRe.FindAllString(field, -1) {
			m := messageTokenRe.FindStringSubmatch(tok)
			if m == nil || m[0] != tok {
				return fmt.Errorf("%w: %q", ErrMalformedToken, tok)
			}
			if m[2] != "" {
				continue
			}
			u, err := url.Parse(m[1])
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("%w: %q", ErrInvalidLink, m[1])
			}
		}
	}
	return nil
}
