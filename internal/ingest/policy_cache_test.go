package ingest

import (
	"context"
	"errors"
	"testing"
	"time"
)

type changingPolicySource struct {
	version     string
	expressions []string
	found       bool
	err         error
	calls       int
}

func (s *changingPolicySource) RedactionPolicy(context.Context, string) (string, []string, bool, error) {
	s.calls++
	return s.version, s.expressions, s.found, s.err
}

func TestPolicyCacheHotReloadRetainsLastValidPolicy(t *testing.T) {
	source := &changingPolicySource{version: "v1", expressions: []string{`account-[0-9]+`}, found: true}
	cache := NewCachedPolicyResolver(source, 30*time.Second)
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }
	version, expressions, found, err := cache.RedactionPolicy(context.Background(), "tenant-a")
	if err != nil || !found || version != "v1" || len(expressions) != 1 {
		t.Fatalf("initial policy = %q %#v %v %v", version, expressions, found, err)
	}
	source.version = "v2"
	source.expressions = []string{"("}
	now = now.Add(31 * time.Second)
	version, expressions, found, err = cache.RedactionPolicy(context.Background(), "tenant-a")
	if err != nil || !found || version != "v1" || len(expressions) != 1 {
		t.Fatalf("invalid refresh replaced the last valid policy: %q %#v %v %v", version, expressions, found, err)
	}
	source.err = errors.New("database unavailable")
	now = now.Add(31 * time.Second)
	version, _, _, err = cache.RedactionPolicy(context.Background(), "tenant-a")
	if err != nil || version != "v1" {
		t.Fatalf("source outage discarded the last valid policy: %q %v", version, err)
	}
}
