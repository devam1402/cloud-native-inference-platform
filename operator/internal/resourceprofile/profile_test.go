package resourceprofile

import "testing"

func TestResolve(t *testing.T) {
	p, err := Resolve("protected-gpu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Preemptible {
		t.Errorf("protected-gpu should not be preemptible")
	}

	if _, err := Resolve("does-not-exist"); err == nil {
		t.Errorf("expected error for unknown profile")
	}
}
