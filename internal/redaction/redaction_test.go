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
