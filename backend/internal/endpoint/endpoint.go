package endpoint

import (
	"backend/internal/healthcheck"
	"backend/internal/post"
	"log/slog"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
)

type RegisterFunc func(pattern string, handler func(http.ResponseWriter, *http.Request))

func Register(register RegisterFunc, db *sqlx.DB, vk valkey.Client) {
	register("GET /api/v1/liveness", healthcheck.LivenessHandler())
	register("GET /api/v1/readiness", healthcheck.ReadinessHandler(db))
	register("POST /ui/v1/posts", post.Create(db, vk))
	register("GET /ui/v1/posts", post.List(db, vk))
	register("GET /ui/v1/posts/{id}", post.GetByID(db, vk))
	register("DELETE /ui/v1/posts/{id}", post.Delete(db, vk))
	register("POST /ui/v1/posts/{id}/comment", post.AddComment(db, vk))
	slog.Info("Registered endpoints")
}
