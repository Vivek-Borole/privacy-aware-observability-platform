package main

import "testing"

func TestApplySplitsSchemaStatements(t *testing.T) {
	// The behavior is kept simple intentionally: the versioned DDL contains no
	// string literals with semicolons, and each request must carry one query.
	parts := 0
	for _, statement := range splitStatements("create database x; create table x.y (id String);") {
		if statement != "" {
			parts++
		}
	}
	if parts != 2 {
		t.Fatalf("expected 2 statements, got %d", parts)
	}
}
