package middleware

import (
	"net/http"
	"sync"
	"time"
)

func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func RateLimit(max int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	hits := map[string][]time.Time{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			now := time.Now()
			mu.Lock()
			recent := hits[key][:0]
			for _, t := range hits[key] {
				if now.Sub(t) < window {
					recent = append(recent, t)
				}
			}
			if len(recent) >= max {
				hits[key] = recent
				mu.Unlock()
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			hits[key] = append(recent, now)
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}
