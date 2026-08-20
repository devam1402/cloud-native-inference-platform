package admission

import "fmt"

// Error implements the error interface so a denied AdmissionDecision can
// be returned directly as a Go error from a webhook handler, with the
// code/reason/suggestion preserved in the message rather than collapsed
// into a generic string.
func (d *AdmissionDecision) Error() string {
	if d.Allowed {
		return ""
	}
	if d.Suggestion != "" {
		return fmt.Sprintf("[%s] %s (%s)", d.Code, d.Reason, d.Suggestion)
	}
	return fmt.Sprintf("[%s] %s", d.Code, d.Reason)
}
