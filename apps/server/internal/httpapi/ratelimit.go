package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateLimiter is a per-IP token bucket. Cheap, in-memory, no external deps.
//
// We don't bother with a global limiter or a more sophisticated algorithm
// because the threats we're guarding against are:
//
//   - A user accidentally hitting `SELECT * FROM events` in a tight loop
//   - An authenticated client running a script that paginates exports
//
// 10 requests/minute per IP catches both without surprising legitimate users.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rpm     int
	gcEvery int // run cleanup once every N requests
	calls   int
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

func newRateLimiter(rpm int) *rateLimiter {
	if rpm <= 0 {
		rpm = 10
	}
	return &rateLimiter{
		buckets: map[string]*bucket{},
		rpm:     rpm,
		gcEvery: 100,
	}
}

// allow returns (true, "") if the IP is under the limit, otherwise
// (false, retry-after-seconds-as-string).
func (r *rateLimiter) allow(ip string) (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	if r.calls%r.gcEvery == 0 {
		r.gcLocked()
	}

	now := time.Now()
	b, ok := r.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(r.rpm), lastFill: now}
		r.buckets[ip] = b
	}
	// Refill: rpm tokens per minute → rpm/60 per second.
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * float64(r.rpm) / 60.0
	if b.tokens > float64(r.rpm) {
		b.tokens = float64(r.rpm)
	}
	b.lastFill = now

	if b.tokens < 1 {
		// Time until 1 token: (1-tokens) * 60 / rpm seconds.
		retry := int((1.0 - b.tokens) * 60.0 / float64(r.rpm))
		if retry < 1 {
			retry = 1
		}
		return false, strconv.Itoa(retry)
	}
	b.tokens--
	return true, ""
}

// gcLocked drops buckets we haven't seen in 5 minutes. Caller must hold mu.
func (r *rateLimiter) gcLocked() {
	cut := time.Now().Add(-5 * time.Minute)
	for ip, b := range r.buckets {
		if b.lastFill.Before(cut) {
			delete(r.buckets, ip)
		}
	}
}

// middleware applies the limiter to subsequent handlers.
func (r *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := clientIP(req)
		ok, retry := r.allow(ip)
		if !ok {
			w.Header().Set("Retry-After", retry)
			http.Error(w, fmt.Sprintf("rate limit exceeded; retry after %ss", retry), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// clientIP extracts the client IP. Honors X-Real-IP and X-Forwarded-For
// when set by middleware (chi's RealIP populates RemoteAddr).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
