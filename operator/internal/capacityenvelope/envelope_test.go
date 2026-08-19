package capacityenvelope

import "testing"

func TestCheck(t *testing.T) {
	e, err := Check("qwen-7b", "protected-gpu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.Available {
		t.Errorf("expected stub envelope to report available")
	}
}
