package redaction

import (
	"regexp"
	"strings"
	"testing"
)

func TestSanitizeRedactsBeforePersistence(t *testing.T) {
	attributes := map[string]string{
		"http.authorization": "Bearer seeded-super-secret-token",
		"http.cookie":        "session=seeded-cookie",
		"customer.email":     "person@example.test",
		"order.id":           "invoice=INV-31415",
		"service.name":       "checkout",
	}
	result, receipt := Sanitize(attributes, "policy-2026-08", []*regexp.Regexp{regexp.MustCompile(`INV-\d+`)})
	if result["service.name"] != "checkout" || receipt.PolicyVersion != "policy-2026-08" {
		t.Fatalf("expected safe values and policy version, got %#v %#v", result, receipt)
	}
	serialized := strings.Join([]string{result["http.authorization"], result["http.cookie"], result["customer.email"], result["order.id"]}, " ")
	for _, forbidden := range []string{"seeded-super-secret-token", "seeded-cookie", "person@example.test", "INV-31415"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("sanitized envelope leaked %q: %s", forbidden, serialized)
		}
	}
	if len(receipt.RedactedPaths) != 4 {
		t.Fatalf("expected four redaction receipts, got %#v", receipt)
	}
}

func TestSanitizeAppliesEmailAndConfiguredPatternsToOneValue(t *testing.T) {
	result, receipt := Sanitize(map[string]string{"log.body": "person@example.test customer-7"}, "policy-test", []*regexp.Regexp{regexp.MustCompile(`customer-\d+`)})
	if strings.Contains(result["log.body"], "person@example.test") || strings.Contains(result["log.body"], "customer-7") {
		t.Fatalf("combined redaction leaked value: %q", result["log.body"])
	}
	if len(receipt.RedactedPaths) != 1 || receipt.RedactedPaths[0] != "attributes.log.body" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}
