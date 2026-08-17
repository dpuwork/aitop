package provider

import (
	"errors"
	"fmt"
	"testing"
)

func TestWindowRemainingClamps(t *testing.T) {
	tests := []struct {
		percent int
		want    int
	}{
		{percent: -20, want: 100},
		{percent: 0, want: 100},
		{percent: 25, want: 75},
		{percent: 100, want: 0},
		{percent: 120, want: 0},
	}
	for _, tt := range tests {
		if got := (Window{Percent: tt.percent}).Remaining(); got != tt.want {
			t.Errorf("Percent %d: Remaining() = %d, want %d", tt.percent, got, tt.want)
		}
	}
}

func TestIsUnavailable(t *testing.T) {
	if !IsUnavailable(ErrUnavailable) {
		t.Error("IsUnavailable(ErrUnavailable) = false")
	}
	wrapped := fmt.Errorf("probe failed: %w", ErrUnavailable)
	if !IsUnavailable(wrapped) {
		t.Error("IsUnavailable should recognize wrapped ErrUnavailable")
	}
	if IsUnavailable(errors.New("provider unavailable")) {
		t.Error("IsUnavailable matched an unrelated error")
	}
	if IsUnavailable(nil) {
		t.Error("IsUnavailable(nil) = true")
	}
}
