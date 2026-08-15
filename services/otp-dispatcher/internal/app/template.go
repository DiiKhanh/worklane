package app

import "fmt"

// Template renders the OTP email. BodyFmt is a printf format with a single %s for the
// code, e.g. "Your verification code is %s. It expires in 5 minutes."
type Template struct {
	Subject string
	BodyFmt string
}

// Render returns the subject and the body with the code substituted.
func (t Template) Render(code string) (subject, body string) {
	return t.Subject, fmt.Sprintf(t.BodyFmt, code)
}
