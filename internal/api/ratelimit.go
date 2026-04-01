package api

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	lastUsed   time.Time
}

func newTokenBucket(ratePerSec, burst float64) *tokenBucket {
	now := time.Now()
	return &tokenBucket{
		tokens:     burst,
		maxTokens:  burst,
		refillRate: ratePerSec,
		lastRefill: now,
		lastUsed:   now,
	}
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		b.lastUsed = now
		return true
	}
	return false
}

func (b *tokenBucket) remaining() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	tokens := b.tokens + elapsed*b.refillRate
	if tokens > b.maxTokens {
		tokens = b.maxTokens
	}
	return tokens
}

func (b *tokenBucket) resetTime() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastRefill.Add(time.Duration(float64(b.maxTokens-b.tokens) / b.refillRate * float64(time.Second)))
}

type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   float64
}

func newIPRateLimiter(ratePerSec, burst float64) *ipRateLimiter {
	return &ipRateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    ratePerSec,
		burst:   burst,
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	b, ok := l.buckets[ip]
	if !ok {
		b = newTokenBucket(l.rate, l.burst)
		l.buckets[ip] = b
	}
	l.mu.Unlock()
	return b.allow()
}

func (l *ipRateLimiter) remaining(ip string) float64 {
	l.mu.Lock()
	b, ok := l.buckets[ip]
	l.mu.Unlock()
	if !ok {
		return l.burst
	}
	return b.remaining()
}

func (l *ipRateLimiter) resetTime(ip string) time.Time {
	l.mu.Lock()
	b, ok := l.buckets[ip]
	l.mu.Unlock()
	if !ok {
		return time.Now().Add(time.Duration(l.burst / l.rate * float64(time.Second)))
	}
	return b.resetTime()
}

type userRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   float64
}

func newUserRateLimiter(ratePerSec, burst float64) *userRateLimiter {
	return &userRateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    ratePerSec,
		burst:   burst,
	}
}

func (l *userRateLimiter) allow(userID string) bool {
	l.mu.Lock()
	b, ok := l.buckets[userID]
	if !ok {
		b = newTokenBucket(l.rate, l.burst)
		l.buckets[userID] = b
	}
	l.mu.Unlock()
	return b.allow()
}

func (l *userRateLimiter) remaining(userID string) float64 {
	l.mu.Lock()
	b, ok := l.buckets[userID]
	l.mu.Unlock()
	if !ok {
		return l.burst
	}
	return b.remaining()
}

func (l *userRateLimiter) resetTime(userID string) time.Time {
	l.mu.Lock()
	b, ok := l.buckets[userID]
	l.mu.Unlock()
	if !ok {
		return time.Now().Add(time.Duration(l.burst / l.rate * float64(time.Second)))
	}
	return b.resetTime()
}

func (l *userRateLimiter) evictIdle(maxAge time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	evicted := 0
	for ip, b := range l.buckets {
		if now.Sub(b.lastUsed) > maxAge {
			delete(l.buckets, ip)
			evicted++
		}
	}
	return evicted
}

func (l *ipRateLimiter) evictIdle(maxAge time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	evicted := 0
	for ip, b := range l.buckets {
		if now.Sub(b.lastUsed) > maxAge {
			delete(l.buckets, ip)
			evicted++
		}
	}
	return evicted
}

var (
	globalIPLimiter   *ipRateLimiter
	apiKeyLimiter     *userRateLimiter
	authIPLimiter     *ipRateLimiter
	inviteUserLimiter *userRateLimiter
	limiterCleanupDone chan struct{}
	limiterCleanupOnce sync.Once
)

func initLimiters() {
	globalIPLimiter = newIPRateLimiter(60, 120)
	apiKeyLimiter = newUserRateLimiter(60.0/60.0, 30)
	authIPLimiter = newIPRateLimiter(5.0/60.0, 5)
	inviteUserLimiter = newUserRateLimiter(10.0/3600.0, 10)
}

func startLimiterCleanup(interval time.Duration, maxAge time.Duration) {
	limiterCleanupOnce.Do(func() {
		limiterCleanupDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					n1 := globalIPLimiter.evictIdle(maxAge)
					n2 := authIPLimiter.evictIdle(maxAge)
					n3 := apiKeyLimiter.evictIdle(maxAge)
					n4 := inviteUserLimiter.evictIdle(maxAge)
					if n1+n2+n3+n4 > 0 {
						// Silently evict idle buckets
						_ = n1 + n2 + n3 + n4
					}
				case <-limiterCleanupDone:
					return
				}
			}
		}()
	})
}

func stopLimiterCleanup() {
	if limiterCleanupDone != nil {
		close(limiterCleanupDone)
	}
}

func writeRateLimitHeaders(w http.ResponseWriter, limit float64, remaining float64, reset time.Time) {
	w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(limit, 'f', 0, 64))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remaining, 'f', 0, 64))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
}

func write429(w http.ResponseWriter, reset time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(reset).Seconds()+1)))
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"error":"rate limit exceeded"}`))
}

func getIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

func IPAllow(limiter *ipRateLimiter, w http.ResponseWriter, r *http.Request) bool {
	ip := getIP(r)
	if !limiter.allow(ip) {
		writeRateLimitHeaders(w, limiter.burst, limiter.remaining(ip), limiter.resetTime(ip))
		write429(w, limiter.resetTime(ip))
		return false
	}
	return true
}

func UserAllow(limiter *userRateLimiter, userID string, w http.ResponseWriter) bool {
	if !limiter.allow(userID) {
		writeRateLimitHeaders(w, limiter.burst, limiter.remaining(userID), limiter.resetTime(userID))
		write429(w, limiter.resetTime(userID))
		return false
	}
	return true
}

func UserRateLimit(ratePerSec, burst float64) func(http.Handler) http.Handler {
	limiter := newUserRateLimiter(ratePerSec, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			user, ok := JWTUserFrom(req.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
				return
			}
			if !limiter.allow(user.ID) {
				writeRateLimitHeaders(w, burst, limiter.remaining(user.ID), limiter.resetTime(user.ID))
				write429(w, limiter.resetTime(user.ID))
				return
			}
			writeRateLimitHeaders(w, burst, limiter.remaining(user.ID), limiter.resetTime(user.ID))
			next.ServeHTTP(w, req)
		})
	}
}

func RateLimit(ratePerSec, burst float64) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(ratePerSec, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := getIP(req)
			if !limiter.allow(ip) {
				writeRateLimitHeaders(w, burst, limiter.remaining(ip), limiter.resetTime(ip))
				write429(w, limiter.resetTime(ip))
				return
			}
			writeRateLimitHeaders(w, burst, limiter.remaining(ip), limiter.resetTime(ip))
			next.ServeHTTP(w, req)
		})
	}
}

func StrictIPRateLimit(ratePerSec, burst float64) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(ratePerSec, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := getIP(req)
			if !limiter.allow(ip) {
				writeRateLimitHeaders(w, burst, limiter.remaining(ip), limiter.resetTime(ip))
				write429(w, limiter.resetTime(ip))
				return
			}
			writeRateLimitHeaders(w, burst, limiter.remaining(ip), limiter.resetTime(ip))
			next.ServeHTTP(w, req)
		})
	}
}

func StrictUserRateLimit(ratePerSec, burst float64) func(http.Handler) http.Handler {
	limiter := newUserRateLimiter(ratePerSec, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			user, ok := JWTUserFrom(req.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
				return
			}
			if !limiter.allow(user.ID) {
				writeRateLimitHeaders(w, burst, limiter.remaining(user.ID), limiter.resetTime(user.ID))
				write429(w, limiter.resetTime(user.ID))
				return
			}
			writeRateLimitHeaders(w, burst, limiter.remaining(user.ID), limiter.resetTime(user.ID))
			next.ServeHTTP(w, req)
		})
	}
}

func APIKeyRateLimit(ratePerSec, burst float64) func(http.Handler) http.Handler {
	limiter := newUserRateLimiter(ratePerSec, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims, ok := ClaimsFrom(req.Context())
			if !ok {
				next.ServeHTTP(w, req)
				return
			}
			if !limiter.allow(claims.KeyID) {
				writeRateLimitHeaders(w, burst, limiter.remaining(claims.KeyID), limiter.resetTime(claims.KeyID))
				write429(w, limiter.resetTime(claims.KeyID))
				return
			}
			writeRateLimitHeaders(w, burst, limiter.remaining(claims.KeyID), limiter.resetTime(claims.KeyID))
			next.ServeHTTP(w, req)
		})
	}
}
