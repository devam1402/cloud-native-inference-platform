package priority

import "testing"

func TestCalculate(t *testing.T) {
	cases := []struct {
		name          string
		tenant        int32
		workloadClass string
		override      *int32
		want          int32
	}{
		{"interactive keeps tenant priority", 100, "interactive", nil, 100},
		{"batch scales down", 90, "batch", nil, 30},
		{"background scales down more", 100, "background", nil, 10},
		{"override wins regardless of class", 100, "interactive", int32Ptr(5), 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Calculate(c.tenant, c.workloadClass, c.override)
			if got != c.want {
				t.Errorf("Calculate(%d, %q, %v) = %d, want %d", c.tenant, c.workloadClass, c.override, got, c.want)
			}
		})
	}
}

func int32Ptr(v int32) *int32 { return &v }
