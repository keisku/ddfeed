package middleware

import "net/http"

// Option is a function that modifies an http.Handler.
type Option func(http.Handler) http.Handler

// Wrap applies a chain of options to a handler.
func Wrap(h http.Handler, opts ...Option) http.Handler {
	for i := len(opts) - 1; i >= 0; i-- {
		h = opts[i](h)
	}
	return h
}

// CORS returns an option that adds CORS headers to every response.
func CORS() Option {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*") // TODO: restrict to frontend
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
