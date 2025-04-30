package post

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
)

type Post struct {
	ID           int       `db:"id" json:"id"`
	Body         string    `db:"body" json:"body"`
	Comments     []Comment `json:"comments,omitempty"`
	CommentCount int       `json:"comment_count"`
}

type Comment struct {
	ID     int    `db:"id" json:"id"`
	Body   string `db:"body" json:"body"`
	PostID int    `db:"post_id" json:"post_id"`
}

func Create(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var post Post
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := db.ExecContext(r.Context(), "INSERT INTO post (body) VALUES (?)", post.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id, err := result.LastInsertId()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := vk.Do(r.Context(), vk.B().Set().Key(fmt.Sprintf("post:%d", id)).Value(post.Body).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set post in valkey", slog.Any("error", err))
		}
		countKey := fmt.Sprintf("post:%d:comment_count", id)
		if err := vk.Do(r.Context(), vk.B().Set().Key(countKey).Value("0").Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set comment count in valkey", slog.Any("error", err))
		}
		post.ID = int(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	}
}

func List(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var posts []Post
		if err := db.SelectContext(r.Context(), &posts, "SELECT id, body FROM post"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range posts {
			countKey := fmt.Sprintf("post:%d:comment_count", posts[i].ID)
			if count, err := vk.Do(r.Context(), vk.B().Get().Key(countKey).Build()).AsInt64(); err == nil {
				posts[i].CommentCount = int(count)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	}
}

func GetByID(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var post Post
		if bytes, err := vk.Do(r.Context(), vk.B().Get().Key(fmt.Sprintf("post:%d", id)).Build()).AsBytes(); err == nil {
			post = Post{
				ID:   int(id),
				Body: string(bytes),
			}
		}
		if post.ID == 0 {
			if err := db.GetContext(r.Context(), &post, "SELECT id, body FROM post WHERE id = ?", id); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "no post found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		var comments []Comment
		if err := db.SelectContext(r.Context(), &comments, "SELECT id, body, post_id FROM comment WHERE post_id = ?", id); err == nil {
			post.Comments = comments
			post.CommentCount = len(comments)
		} else {
			slog.ErrorContext(r.Context(), "failed to fetch comments from db", slog.Any("error", err))
		}
		countKey := fmt.Sprintf("post:%d:comment_count", id)
		if err := vk.Do(r.Context(), vk.B().Set().Key(countKey).Value(strconv.Itoa(post.CommentCount)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set comment count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	}
}

func Delete(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), "DELETE FROM post WHERE id = ?", id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := vk.Do(r.Context(), vk.B().Del().Key(fmt.Sprintf("post:%d", id)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete post in valkey", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Del().Key(fmt.Sprintf("post:%d:comment_count", id)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete comment count in valkey", slog.Any("error", err))
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func AddComment(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var post Post
		if err := db.GetContext(r.Context(), &post, "SELECT id, body FROM post WHERE id = ?", postID); err != nil {
			if err == sql.ErrNoRows {
				slog.ErrorContext(r.Context(), "no post found", slog.Int64("post_id", postID))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var comment Comment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := db.ExecContext(r.Context(), "INSERT INTO comment (body, post_id) VALUES (?, ?)", comment.Body, postID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commentID, err := result.LastInsertId()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		comment.ID = int(commentID)
		comment.PostID = int(postID)

		// Increment comment counter in valkey
		countKey := fmt.Sprintf("post:%d:comment_count", postID)
		count, err := vk.Do(r.Context(), vk.B().Get().Key(countKey).Build()).AsInt64()
		if err != nil {
			// If key doesn't exist, initialize with 1
			count = 1
		} else {
			count++
		}
		if err := vk.Do(r.Context(), vk.B().Set().Key(countKey).Value(strconv.FormatInt(count, 10)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to update comment count in valkey", slog.Any("error", err))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comment)
	}
}
