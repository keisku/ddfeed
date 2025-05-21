package post

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
)

type Post struct {
	UUID         string    `db:"uuid" json:"uuid"`
	Body         string    `db:"body" json:"body"`
	Comments     []Comment `json:"comments,omitempty"`
	CommentCount int       `json:"comment_count"`
}

type Comment struct {
	UUID   string `db:"uuid" json:"uuid"`
	Body   string `db:"body" json:"body"`
	PostID string `db:"post_id" json:"post_id"`
}

func Create(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var post Post
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		post.UUID = uuid.New().String()
		result, err := db.ExecContext(r.Context(), "INSERT INTO post (uuid, body) VALUES (UUID_TO_BIN(?), ?)", post.UUID, post.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id, err := result.LastInsertId()
		if err == nil {
			if err := vk.Do(r.Context(), vk.B().Set().Key(fmt.Sprintf("post_pk:%s", post.UUID)).Value(strconv.FormatInt(id, 10)).Build()).Error(); err != nil {
				slog.ErrorContext(r.Context(), "failed to set post pk in valkey", slog.Any("error", err))
			}
		} else {
			slog.ErrorContext(r.Context(), "failed to get last insert id", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Set().Key("post:"+post.UUID).Value(post.Body).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set post in valkey", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Incr().Key("post:total_count").Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to increment total post count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	}
}

func List(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 10
		}
		lastUUID := r.URL.Query().Get("last_uuid")
		var posts []Post
		var err error
		if lastUUID == "" {
			err = db.SelectContext(r.Context(), &posts,
				"SELECT BIN_TO_UUID(uuid) as uuid, body FROM post ORDER BY id DESC LIMIT ?",
				limit)
		} else {
			err = db.SelectContext(r.Context(), &posts,
				"SELECT BIN_TO_UUID(uuid) as uuid, body FROM post WHERE id < (SELECT id FROM post WHERE uuid = UUID_TO_BIN(?)) ORDER BY id DESC LIMIT ?",
				lastUUID, limit)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var total int
		if count, err := vk.Do(r.Context(), vk.B().Get().Key("post:total_count").Build()).AsInt64(); err == nil {
			total = int(count)
		} else {
			if err := db.GetContext(r.Context(), &total, "SELECT COUNT(*) FROM post"); err != nil {
				slog.ErrorContext(r.Context(), "failed to get total count", slog.Any("error", err))
			} else {
				if err := vk.Do(r.Context(), vk.B().Set().Key("post:total_count").Value(strconv.Itoa(total)).Build()).Error(); err != nil {
					slog.ErrorContext(r.Context(), "failed to set total count in valkey", slog.Any("error", err))
				}
			}
		}
		countKeys := make([]string, len(posts))
		for i := range posts {
			countKeys[i] = "post:" + posts[i].UUID + ":comment_count"
		}
		if len(countKeys) > 0 {
			if results, err := vk.Do(r.Context(), vk.B().Mget().Key(countKeys...).Build()).ToArray(); err == nil {
				for i, result := range results {
					if err := result.Error(); err != nil {
						slog.ErrorContext(r.Context(), "get comment count from valkey, fallback to db", slog.Any("error", err))
						if err := db.GetContext(r.Context(), &posts[i].CommentCount, "SELECT COUNT(*) FROM comment WHERE post_id = (SELECT id FROM post WHERE uuid = UUID_TO_BIN(?))", posts[i].UUID); err != nil {
							slog.ErrorContext(r.Context(), "failed to get comment count from db", slog.Any("error", err))
						}
						if err := vk.Do(r.Context(), vk.B().Set().Key(countKeys[i]).Value(strconv.Itoa(posts[i].CommentCount)).Build()).Error(); err != nil {
							slog.ErrorContext(r.Context(), "failed to set comment count in valkey", slog.Any("error", err))
						}
						continue
					}
					if count, err := result.AsInt64(); err == nil {
						posts[i].CommentCount = int(count)
					}
				}
			}
		}
		var nextLastUUID string
		if len(posts) > 0 {
			nextLastUUID = posts[len(posts)-1].UUID
		}
		response := struct {
			Posts      []Post `json:"posts"`
			Limit      int    `json:"limit"`
			Total      int    `json:"total"`
			NextLastID string `json:"next_last_uuid,omitempty"`
		}{
			Posts:      posts,
			Limit:      limit,
			Total:      total,
			NextLastID: nextLastUUID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func GetByID(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.PathValue("uuid")
		if uuidStr == "" {
			http.Error(w, "missing uuid", http.StatusBadRequest)
			return
		}
		var post Post
		if bytes, err := vk.Do(r.Context(), vk.B().Get().Key(fmt.Sprintf("post:%s", uuidStr)).Build()).AsBytes(); err == nil {
			post = Post{
				UUID: uuidStr,
				Body: string(bytes),
			}
		} else {
			if err := db.GetContext(r.Context(), &post, "SELECT body FROM post WHERE uuid = UUID_TO_BIN(?)", uuidStr); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "no post found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		var comments []Comment
		if err := db.SelectContext(r.Context(), &comments, "SELECT uuid, body, post_id FROM comment WHERE post_id = (SELECT id FROM post WHERE uuid = UUID_TO_BIN(?))", uuidStr); err == nil {
			post.Comments = comments
			post.CommentCount = len(comments)
		} else {
			if err == sql.ErrNoRows {
				post.Comments = []Comment{}
				post.CommentCount = 0
			} else {
				slog.ErrorContext(r.Context(), "failed to fetch comments from db", slog.Any("error", err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		countKey := fmt.Sprintf("post:%s:comment_count", uuidStr)
		if err := vk.Do(r.Context(), vk.B().Set().Key(countKey).Value(strconv.Itoa(post.CommentCount)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set comment count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	}
}

func Delete(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.PathValue("uuid")
		if uuidStr == "" {
			http.Error(w, "missing uuid", http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), "DELETE FROM post WHERE uuid = UUID_TO_BIN(?)", uuidStr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := vk.Do(r.Context(), vk.B().Del().Key(fmt.Sprintf("post:%s", uuidStr)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete post in valkey", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Del().Key(fmt.Sprintf("post_pk:%s", uuidStr)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete post pk in valkey", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Del().Key(fmt.Sprintf("post:%s:comment_count", uuidStr)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete comment count in valkey", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Decr().Key("post:total_count").Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to decrement total post count in valkey", slog.Any("error", err))
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func AddComment(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postUUID := r.PathValue("uuid")
		if postUUID == "" {
			http.Error(w, "missing uuid", http.StatusBadRequest)
			return
		}
		var postID int
		pkKey := fmt.Sprintf("post_pk:%s", postUUID)
		if pkStr, err := vk.Do(r.Context(), vk.B().Get().Key(pkKey).Build()).AsBytes(); err == nil {
			postID, err = strconv.Atoi(string(pkStr))
		} else {
			if err := db.GetContext(r.Context(), &postID, "SELECT id FROM post WHERE uuid = UUID_TO_BIN(?)", postUUID); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "no post found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := vk.Do(r.Context(), vk.B().Set().Key(pkKey).Value(strconv.Itoa(postID)).Build()).Error(); err != nil {
				slog.ErrorContext(r.Context(), "failed to set post pk in valkey", slog.Any("error", err))
			}
		}
		var comment Comment
		if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		comment.UUID = uuid.New().String()
		_, err := db.ExecContext(r.Context(), "INSERT INTO comment (uuid, body, post_id) VALUES (UUID_TO_BIN(?), ?, ?)", comment.UUID, comment.Body, postID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := vk.Do(r.Context(), vk.B().Incr().Key(fmt.Sprintf("post:%s:comment_count", postUUID)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to increment comment count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comment)
	}
}
