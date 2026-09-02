package telemetry

import (
	"testing"

	"github.com/Vivek-Borole/privacy-aware-observability-platform/internal/ingest"
)

func TestSupportedEnvelopeVersionMigrationWindow(t *testing.T) {
	tests := []struct {
		name    string
		input   int
		want    int
		allowed bool
	}{
		{name: "legacy missing version migrates to previous", input: 0, want: ingest.PreviousEnvelopeVersion, allowed: true},
		{name: "previous remains readable", input: ingest.PreviousEnvelopeVersion, want: ingest.PreviousEnvelopeVersion, allowed: true},
		{name: "current remains readable", input: ingest.CurrentEnvelopeVersion, want: ingest.CurrentEnvelopeVersion, allowed: true},
		{name: "future version is rejected", input: ingest.CurrentEnvelopeVersion + 1, want: ingest.CurrentEnvelopeVersion + 1, allowed: false},
		{name: "obsolete version is rejected", input: -1, want: -1, allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, allowed := supportedEnvelopeVersion(test.input)
			if got != test.want || allowed != test.allowed {
				t.Fatalf("supportedEnvelopeVersion(%d) = (%d, %t), want (%d, %t)", test.input, got, allowed, test.want, test.allowed)
			}
		})
	}
}
