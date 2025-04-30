package healthcheck

import (
	"net/http"

	"github.com/jmoiron/sqlx"
)

// ReadinessHandler returns an http.HandlerFunc for readiness checks.
func ReadinessHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// LivenessHandler returns an http.HandlerFunc for liveness checks.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}
