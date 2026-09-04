package dashboard

import (
	"strings"
	"testing"
)

func TestDistRequiresIndex(t *testing.T) {
	_, err := Dist()
	if err == nil {
		return
	}
	if !strings.Contains(err.Error(), "dashboard dist is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
