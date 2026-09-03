package ingest

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"
)

type cachedPolicy struct {
	version     string
	expressions []string
	found       bool
	refreshedAt time.Time
}

// CachedPolicyResolver atomically activates only fully compiled policy sets.
// A temporary database or compilation failure retains the tenant's last valid
// policy instead of weakening redaction or partially applying an update.
type CachedPolicyResolver struct {
	source   PolicyResolver
	interval time.Duration
	now      func() time.Time
	mu       sync.RWMutex
	policies map[string]cachedPolicy
}

func NewCachedPolicyResolver(source PolicyResolver, interval time.Duration) *CachedPolicyResolver {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &CachedPolicyResolver{source: source, interval: interval, now: time.Now, policies: make(map[string]cachedPolicy)}
}

func (c *CachedPolicyResolver) RedactionPolicy(ctx context.Context, tenantID string) (string, []string, bool, error) {
	now := c.now()
	c.mu.RLock()
	cached, exists := c.policies[tenantID]
	c.mu.RUnlock()
	if exists && now.Sub(cached.refreshedAt) < c.interval {
		return cached.version, append([]string(nil), cached.expressions...), cached.found, nil
	}
	version, expressions, found, err := c.source.RedactionPolicy(ctx, tenantID)
	if err == nil {
		err = validatePolicy(version, expressions, found)
	}
	if err != nil {
		if exists {
			return cached.version, append([]string(nil), cached.expressions...), cached.found, nil
		}
		return "", nil, false, err
	}
	next := cachedPolicy{version: version, expressions: append([]string(nil), expressions...), found: found, refreshedAt: now}
	c.mu.Lock()
	c.policies[tenantID] = next
	c.mu.Unlock()
	return next.version, append([]string(nil), next.expressions...), next.found, nil
}

func validatePolicy(version string, expressions []string, found bool) error {
	if !found {
		return nil
	}
	if version == "" {
		return errors.New("redaction policy version missing")
	}
	for _, expression := range expressions {
		if len(expression) == 0 || len(expression) > 512 {
			return errors.New("redaction policy expression invalid")
		}
		if _, err := regexp.Compile(expression); err != nil {
			return errors.New("redaction policy expression invalid")
		}
	}
	return nil
}
