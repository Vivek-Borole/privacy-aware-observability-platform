package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsOnlyConfiguredConsole(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), "http://localhost:5173")
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/v1/traces/x", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed request rejected: %d %#v", allowed.Code, allowed.Header())
	}
	if got := allowed.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" || allowed.Header().Get("Access-Control-Allow-Headers") != "X-PAOP-API-Key, Content-Type, X-PAOP-Delete-Confirm" {
		t.Fatalf("deletion CORS controls missing: %#v", allowed.Header())
	}
	blocked := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodOptions, "/v1/traces/x", nil)
	blockedRequest.Header.Set("Origin", "https://untrusted.example")
	blockedRequest.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("unexpected status %d", blocked.Code)
	}
}
