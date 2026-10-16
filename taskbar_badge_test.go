package main

import "testing"

func TestTaskbarBadgeLabel(t *testing.T) {
	tests := map[int]string{0: "", 1: "1", 42: "42", 99: "99", 100: "99+"}
	for count, expected := range tests {
		if actual := taskbarBadgeLabel(count); actual != expected {
			t.Fatalf("label for %d = %q, want %q", count, actual, expected)
		}
	}
}

func TestRenderTaskbarBadgeProducesOpaquePixels(t *testing.T) {
	andMask, xorMask := renderTaskbarBadge(8)
	if len(andMask) != taskbarBadgeSize*taskbarBadgeSize/8 {
		t.Fatalf("unexpected AND mask length: %d", len(andMask))
	}
	if len(xorMask) != taskbarBadgeSize*taskbarBadgeSize*4 {
		t.Fatalf("unexpected XOR mask length: %d", len(xorMask))
	}
	opaque := 0
	for index := 3; index < len(xorMask); index += 4 {
		if xorMask[index] != 0 {
			opaque++
		}
	}
	if opaque == 0 {
		t.Fatal("badge contains no opaque pixels")
	}
}
