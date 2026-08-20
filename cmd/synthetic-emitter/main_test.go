package main

import "testing"

func TestSampledMatchesDocumentedFNVSelection(t *testing.T) {
	if !sampled("smoke-trace-007", 10) {
		t.Fatal("expected documented selected trace to be retained by modulo 10")
	}
	if sampled("smoke-trace-001", 10) {
		t.Fatal("expected documented non-selected trace to be dropped by modulo 10")
	}
}
