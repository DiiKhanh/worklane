// Package templating is the shared, dependency-free render engine for OTP messages.
// It performs allowlisted variable substitution ({{code}}, {{expiry}}) so the exact
// same code renders a preview in otp-api and a delivered message in otp-dispatcher -
// a preview therefore cannot diverge from what is sent. Substitution is a literal
// allowlist replace, never a template interpreter: there is no field walking and no
// code-execution surface.
package templating

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrUnknownVariable = errors.New("templating: unknown variable")
	ErrSubjectRequired = errors.New("templating: subject required for email")
)

// Vars carries the values available in the OTP flow today. Extend this (and the
// allowlist below) when sub-projects B/C add {{name}} and {{link}}.
type Vars struct {
	Code   string
	Expiry string
}

// tokenRe matches {{ name }} with optional inner whitespace.
var tokenRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// allowed is the substitution allowlist; the key is the variable name.
func allowed(v Vars) map[string]string {
	return map[string]string{"code": v.Code, "expiry": v.Expiry}
}

// Render replaces every allowlisted token in subject and body. Unknown tokens are
// left untouched (Validate is the gate that keeps them from ever being saved).
func Render(subject, body string, v Vars) (string, string) {
	vals := allowed(v)
	repl := func(s string) string {
		return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
			name := tokenRe.FindStringSubmatch(m)[1]
			if val, ok := vals[name]; ok {
				return val
			}
			return m
		})
	}
	return repl(subject), repl(body)
}

// Validate enforces authoring rules: only allowlisted variables may appear, and email
// requires a non-empty subject.
func Validate(channel, subject, body string) error {
	if channel == "email" && subject == "" {
		return ErrSubjectRequired
	}
	names := allowed(Vars{})
	for _, field := range []string{subject, body} {
		for _, m := range tokenRe.FindAllStringSubmatch(field, -1) {
			if _, ok := names[m[1]]; !ok {
				return fmt.Errorf("%w: %q", ErrUnknownVariable, m[1])
			}
		}
	}
	return nil
}
