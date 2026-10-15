package wlr

import "testing"

func TestPkgConfigWlroots(t *testing.T) {
	if ABI != "0.20" {
		t.Fatalf("ABI = %q, want 0.20", ABI)
	}
}
