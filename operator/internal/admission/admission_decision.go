// Package admission holds pure validation and mutation logic for the
// platform's CRDs — no controller-runtime webhook framework dependency
// here, so this package is unit-testable without envtest. The
// internal/webhook package wires this logic into actual
// admission.CustomValidator / admission.CustomDefaulter handlers.
package admission

// AdmissionDecision is the structured result every validation/mutation
// check returns — one consistent shape instead of nine different ad-hoc
// error strings across the checklist.
type AdmissionDecision struct {
	Allowed    bool
	Code       string // e.g. "TENANT_NOT_FOUND", "PROFILE_INVALID"
	Reason     string // human-readable explanation
	Suggestion string // optional — what the caller should do about it
}

func Allow() *AdmissionDecision {
	return &AdmissionDecision{Allowed: true}
}

func Deny(code, reason, suggestion string) *AdmissionDecision {
	return &AdmissionDecision{
		Allowed:    false,
		Code:       code,
		Reason:     reason,
		Suggestion: suggestion,
	}
}

// FirstDenial returns the first non-allowed decision in the list, or
// Allow() if every check passed. Validators call each check in order
// and pass all results here — this is what lets validation.go run all
// nine checks as small, independently-testable functions while the
// webhook only ever needs to report one reason for a rejection.
func FirstDenial(decisions ...*AdmissionDecision) *AdmissionDecision {
	for _, d := range decisions {
		if !d.Allowed {
			return d
		}
	}
	return Allow()
}
