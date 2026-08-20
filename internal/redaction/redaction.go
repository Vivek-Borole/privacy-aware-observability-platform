// Package redaction sanitizes untrusted telemetry before any durable boundary.
package redaction

import (
	"regexp"
	"strings"
)

const DefaultPolicyVersion = "v1"

var emailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

// Receipt records what was changed without retaining the removed data.
type Receipt struct {
	PolicyVersion string   `json:"policyVersion"`
	RedactedPaths []string `json:"redactedPaths"`
}

// Sanitize copies attributes, redacts sensitive keys and values, and returns a
// receipt safe to persist beside the sanitized event.
func Sanitize(attributes map[string]string, policyVersion string, patterns []*regexp.Regexp) (map[string]string, Receipt) {
	if policyVersion == "" {
		policyVersion = DefaultPolicyVersion
	}
	out := make(map[string]string, len(attributes))
	receipt := Receipt{PolicyVersion: policyVersion}
	for key, value := range attributes {
		path := "attributes." + key
		if sensitiveKey(key) {
			out[key] = "[REDACTED]"
			receipt.RedactedPaths = append(receipt.RedactedPaths, path)
			continue
		}
		redactedValue := value
		redacted := false
		if emailPattern.MatchString(redactedValue) {
			redactedValue = emailPattern.ReplaceAllString(redactedValue, "[REDACTED_EMAIL]")
			redacted = true
		}
		for _, pattern := range patterns {
			if pattern.MatchString(redactedValue) {
				redactedValue = pattern.ReplaceAllString(redactedValue, "[REDACTED_PATTERN]")
				redacted = true
			}
		}
		if redacted {
			out[key] = redactedValue
			receipt.RedactedPaths = append(receipt.RedactedPaths, path)
		} else {
			out[key] = value
		}
	}
	return out, receipt
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
	for _, marker := range []string{"authorization", "cookie", "api-key", "apikey", "token", "secret", "password"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
