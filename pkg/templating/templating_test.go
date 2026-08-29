package templating

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderSubstitutesAllowlistedVars(t *testing.T) {
	sub, body := Render("Your code: {{code}}", "It is {{code}}, expires in {{ expiry }}.",
		Vars{Code: "123456", Expiry: "5 minutes"})
	if sub != "Your code: 123456" {
		t.Fatalf("subject: got %q", sub)
	}
	if body != "It is 123456, expires in 5 minutes." {
		t.Fatalf("body: got %q", body)
	}
}

func TestValidateRejectsUnknownVariable(t *testing.T) {
	err := Validate("email", "Hi {{code}}", "Hello {{name}}, code {{code}}")
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("want ErrUnknownVariable, got %v", err)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should name the offending variable, got %v", err)
	}
}

func TestValidateEmailRequiresSubject(t *testing.T) {
	if err := Validate("email", "", "body {{code}}"); !errors.Is(err, ErrSubjectRequired) {
		t.Fatalf("want ErrSubjectRequired, got %v", err)
	}
	if err := Validate("sms", "", "body {{code}}"); err != nil {
		t.Fatalf("sms with empty subject is valid, got %v", err)
	}
}

func TestValidateAllowsKnownVarsAndPlainText(t *testing.T) {
	if err := Validate("email", "Code {{code}}", "It is {{code}}, expires {{expiry}}."); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
}
