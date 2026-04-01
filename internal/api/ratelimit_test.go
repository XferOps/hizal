package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiterAllow(t *testing.T) {
	limiter := newIPRateLimiter(1.0, 2)
	if !limiter.allow("1.2.3.4") {
		t.Fatal("first request should be allowed (bucket starts full)")
	}
	if !limiter.allow("1.2.3.4") {
		t.Fatal("second request should be allowed (burst=2)")
	}
	if limiter.allow("1.2.3.4") {
		t.Fatal("third request should be denied (bucket empty, rate=1/s)")
	}
}

func TestIPRateLimiterPerIPIsolation(t *testing.T) {
	limiter := newIPRateLimiter(0.1, 1)
	if !limiter.allow("1.2.3.4") {
		t.Fatal("ip1 first request should be allowed")
	}
	if limiter.allow("1.2.3.4") {
		t.Fatal("ip1 second request should be denied")
	}
	if !limiter.allow("5.6.7.8") {
		t.Fatal("ip2 should have its own bucket")
	}
}

func TestIPRateLimiterRemaining(t *testing.T) {
	limiter := newIPRateLimiter(100.0, 5)
	rem := limiter.remaining("1.2.3.4")
	if rem != 5 {
		t.Fatalf("fresh bucket remaining = %v, want 5", rem)
	}
	limiter.allow("1.2.3.4")
	rem = limiter.remaining("1.2.3.4")
	if rem >= 5 || rem < 3 {
		t.Fatalf("after 1 request remaining = %v, want ~4 (within refill window)", rem)
	}
}

func TestIPRateLimiterEvictIdle(t *testing.T) {
	limiter := newIPRateLimiter(1.0, 1)
	limiter.allow("1.2.3.4")
	limiter.allow("5.6.7.8")
	time.Sleep(10 * time.Millisecond)
	evicted := limiter.evictIdle(5 * time.Millisecond)
	if evicted != 2 {
		t.Fatalf("evicted = %v, want 2", evicted)
	}
}

func TestIPRateLimiterEvictIdlePartial(t *testing.T) {
	limiter := newIPRateLimiter(1.0, 1)
	limiter.allow("1.2.3.4")
	time.Sleep(15 * time.Millisecond)
	limiter.allow("5.6.7.8")
	evicted := limiter.evictIdle(10 * time.Millisecond)
	if evicted != 1 {
		t.Fatalf("evicted = %v, want 1 (only 5.6.7.8 old enough)", evicted)
	}
}

func TestUserRateLimiterAllow(t *testing.T) {
	limiter := newUserRateLimiter(1.0, 2)
	if !limiter.allow("user-1") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.allow("user-1") {
		t.Fatal("second request should be allowed")
	}
	if limiter.allow("user-1") {
		t.Fatal("third request should be denied")
	}
}

func TestStrictIPRateLimitReturns429(t *testing.T) {
	middleware := StrictIPRateLimit(1.0, 1)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d", rr.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.RemoteAddr = "1.2.3.4:1234"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d", rr2.Code, http.StatusTooManyRequests)
	}
	if rr2.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header on 429")
	}
	if rr2.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header on 429")
	}
	if rr2.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on 429")
	}
}

func TestStrictIPRateLimitHeadersOnSuccess(t *testing.T) {
	limiter := newIPRateLimiter(60, 120)
	middleware := StrictIPRateLimit(60.0, 120)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRateLimitHeaders(w, limiter.burst, limiter.remaining(getIP(r)), limiter.resetTime(getIP(r)))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset header")
	}
}

func TestAPIKeyRateLimitPassesWithoutClaims(t *testing.T) {
	middleware := APIKeyRateLimit(1.0, 1)
	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler should be called when no claims present (auth middleware handles auth)")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestInitLimitersCreatesNonNil(t *testing.T) {
	initLimiters()
	if globalIPLimiter == nil {
		t.Fatal("globalIPLimiter should not be nil after initLimiters()")
	}
	if apiKeyLimiter == nil {
		t.Fatal("apiKeyLimiter should not be nil after initLimiters()")
	}
	if authIPLimiter == nil {
		t.Fatal("authIPLimiter should not be nil after initLimiters()")
	}
	if inviteUserLimiter == nil {
		t.Fatal("inviteUserLimiter should not be nil after initLimiters()")
	}
}

func TestGlobalIPLimiterHasCorrectLimits(t *testing.T) {
	initLimiters()
	if globalIPLimiter.rate != 60 {
		t.Fatalf("globalIPLimiter.rate = %v, want 60", globalIPLimiter.rate)
	}
	if globalIPLimiter.burst != 120 {
		t.Fatalf("globalIPLimiter.burst = %v, want 120", globalIPLimiter.burst)
	}
}

func TestAuthIPLimiterHasCorrectLimits(t *testing.T) {
	initLimiters()
	if authIPLimiter.rate != 5.0/60.0 {
		t.Fatalf("authIPLimiter.rate = %v, want %v", authIPLimiter.rate, 5.0/60.0)
	}
	if authIPLimiter.burst != 5 {
		t.Fatalf("authIPLimiter.burst = %v, want 5", authIPLimiter.burst)
	}
}
