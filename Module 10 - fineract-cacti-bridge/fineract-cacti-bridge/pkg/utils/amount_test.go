package utils

import "testing"

func TestIsPositiveAmount(t *testing.T) {
	for _, s := range []string{"1", "0.01", "1000.50", "999999999.999"} {
		if !IsPositiveAmount(s) {
			t.Errorf("expected %q positive", s)
		}
	}
	for _, s := range []string{"0", "-1", "-0.01", "abc", ""} {
		if IsPositiveAmount(s) {
			t.Errorf("expected %q not positive", s)
		}
	}
}
