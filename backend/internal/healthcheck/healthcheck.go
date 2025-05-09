package healthcheck

import (
	"context"
	"database/sql"
	"fmt"
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

		if err := checkDatabaseSchema(r.Context(), db); err != nil {
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

func checkDatabaseSchema(ctx context.Context, db *sqlx.DB) error {
	// Check post table
	postRows, err := db.QueryContext(ctx, "DESC ddfeed.post")
	if err != nil {
		return fmt.Errorf("failed to check post table: %w", err)
	}
	defer postRows.Close()

	postColumns := make(map[string]struct{})
	for postRows.Next() {
		var field, type_, null, key, default_, extra sql.NullString
		if err := postRows.Scan(&field, &type_, &null, &key, &default_, &extra); err != nil {
			return fmt.Errorf("failed to scan post table columns: %w", err)
		}
		postColumns[field.String] = struct{}{}
	}

	requiredPostColumns := map[string]struct{}{
		"id":         {},
		"body":       {},
		"created_at": {},
		"updated_at": {},
	}

	for col := range requiredPostColumns {
		if _, exists := postColumns[col]; !exists {
			return fmt.Errorf("post table missing required column: %s", col)
		}
	}

	// Check comment table
	commentRows, err := db.QueryContext(ctx, "DESC ddfeed.comment")
	if err != nil {
		return fmt.Errorf("failed to check comment table: %w", err)
	}
	defer commentRows.Close()

	commentColumns := make(map[string]struct{})
	for commentRows.Next() {
		var field, type_, null, key, default_, extra sql.NullString
		if err := commentRows.Scan(&field, &type_, &null, &key, &default_, &extra); err != nil {
			return fmt.Errorf("failed to scan comment table columns: %w", err)
		}
		commentColumns[field.String] = struct{}{}
	}

	requiredCommentColumns := map[string]struct{}{
		"id":         {},
		"body":       {},
		"post_id":    {},
		"created_at": {},
		"updated_at": {},
	}

	for col := range requiredCommentColumns {
		if _, exists := commentColumns[col]; !exists {
			return fmt.Errorf("comment table missing required column: %s", col)
		}
	}

	return nil
}
