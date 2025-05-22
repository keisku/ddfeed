package post

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
	"github.com/valkey-io/valkey-go"
)

type Post struct {
	PublicID     string    `db:"public_id" json:"id"`
	Body         string    `db:"body" json:"body"`
	Comments     []Comment `json:"comments,omitempty"`
	CommentCount int       `json:"comment_count"`
}

type Comment struct {
	PublicID string `db:"public_id" json:"id"`
	Body     string `db:"body" json:"body"`
	PostID   string `db:"post_id" json:"post_id"`
}

func Create(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var post Post
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		post.PublicID = ulid.Make().String()
		result, err := db.ExecContext(r.Context(), "INSERT INTO post (public_id, body) VALUES (?, ?)", post.PublicID, post.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id, err := result.LastInsertId()
		if err == nil {
			if err := vk.Do(r.Context(), vk.B().Set().Key(fmt.Sprintf("post_pk:%s", post.PublicID)).Value(strconv.FormatInt(id, 10)).Build()).Error(); err != nil {
				slog.ErrorContext(r.Context(), "failed to set post pk in valkey", slog.Any("error", err))
			}
		} else {
			slog.ErrorContext(r.Context(), "failed to get last insert id", slog.Any("error", err))
		}
		if err := vk.Do(r.Context(), vk.B().Set().Key("post:"+post.PublicID).Value(post.Body).Build()).Error(); err != nil {
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
		lastID := r.URL.Query().Get("last_id")
		var posts []Post
		var err error
		if lastID == "" {
			err = db.SelectContext(r.Context(), &posts,
				"SELECT public_id, body FROM post ORDER BY id DESC LIMIT ?",
				limit)
		} else {
			pkStr, _ := vk.Do(r.Context(), vk.B().Get().Key(fmt.Sprintf("post_pk:%s", lastID)).Build()).AsBytes()
			postID, err := strconv.Atoi(string(pkStr))
			if err == nil {
				err = db.SelectContext(r.Context(), &posts,
					"SELECT public_id, body FROM post WHERE id < ? ORDER BY id DESC LIMIT ?",
					postID, limit)
			} else {
				err = db.SelectContext(r.Context(), &posts,
					"SELECT public_id, body FROM post WHERE id < (SELECT id FROM post WHERE public_id = ?) ORDER BY id DESC LIMIT ?",
					lastID, limit)
			}
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
			countKeys[i] = "post:" + posts[i].PublicID + ":comment_count"
		}
		if len(countKeys) > 0 {
			if results, err := vk.Do(r.Context(), vk.B().Mget().Key(countKeys...).Build()).ToArray(); err == nil {
				for i, result := range results {
					if err := result.Error(); err != nil {
						slog.ErrorContext(r.Context(), "get comment count from valkey, fallback to db", slog.Any("error", err))
						if err := db.GetContext(r.Context(), &posts[i].CommentCount, "SELECT COUNT(*) FROM comment WHERE post_id = (SELECT id FROM post WHERE public_id = ?)", posts[i].PublicID); err != nil {
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
		var nextLastPublicID string
		if len(posts) > 0 {
			nextLastPublicID = posts[len(posts)-1].PublicID
		}
		response := struct {
			Posts      []Post `json:"posts"`
			Limit      int    `json:"limit"`
			Total      int    `json:"total"`
			NextLastID string `json:"next_last_id,omitempty"`
		}{
			Posts:      posts,
			Limit:      limit,
			Total:      total,
			NextLastID: nextLastPublicID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func GetByID(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicIDStr := r.PathValue("id")
		if publicIDStr == "" {
			http.Error(w, "missing id from path", http.StatusBadRequest)
			return
		}
		var post Post
		if bytes, err := vk.Do(r.Context(), vk.B().Get().Key(fmt.Sprintf("post:%s", publicIDStr)).Build()).AsBytes(); err == nil {
			post = Post{
				PublicID: publicIDStr,
				Body:     string(bytes),
			}
		} else {
			if err := db.GetContext(r.Context(), &post, "SELECT body FROM post WHERE public_id = ?", publicIDStr); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "no post found", http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		var comments []Comment
		if err := db.SelectContext(r.Context(), &comments, "SELECT public_id, body, post_id FROM comment WHERE post_id = (SELECT id FROM post WHERE public_id = ?)", publicIDStr); err == nil {
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
		countKey := fmt.Sprintf("post:%s:comment_count", publicIDStr)
		if err := vk.Do(r.Context(), vk.B().Set().Key(countKey).Value(strconv.Itoa(post.CommentCount)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to set comment count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	}
}

func Delete(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicIDStr := r.PathValue("id")
		if publicIDStr == "" {
			http.Error(w, "missing id from path", http.StatusBadRequest)
			return
		}
		if _, err := db.ExecContext(r.Context(), "DELETE FROM post WHERE public_id = ?", publicIDStr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		delKeys := []string{
			fmt.Sprintf("post:%s", publicIDStr),
			fmt.Sprintf("post_pk:%s", publicIDStr),
			fmt.Sprintf("post:%s:comment_count", publicIDStr),
		}
		multi := []valkey.Completed{
			vk.B().Del().Key(delKeys...).Build(),
			vk.B().Decr().Key("post:total_count").Build(),
		}
		results := vk.DoMulti(r.Context(), multi...)
		for i, res := range results {
			if res.Error() != nil {
				slog.ErrorContext(r.Context(), "delete post caches from valkey", slog.Any("cmd_index", i), slog.Any("error", res.Error()))
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func AddComment(db *sqlx.DB, vk valkey.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postIDStr := r.PathValue("id")
		if postIDStr == "" {
			http.Error(w, "missing id from path", http.StatusBadRequest)
			return
		}
		var postID int
		pkKey := fmt.Sprintf("post_pk:%s", postIDStr)
		if pkStr, err := vk.Do(r.Context(), vk.B().Get().Key(pkKey).Build()).AsBytes(); err == nil {
			postID, err = strconv.Atoi(string(pkStr))
		} else {
			if err := db.GetContext(r.Context(), &postID, "SELECT id FROM post WHERE public_id = ?", postIDStr); err != nil {
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
		comment.PublicID = ulid.Make().String()
		_, err := db.ExecContext(r.Context(), "INSERT INTO comment (public_id, body, post_id) VALUES (?, ?, ?)", comment.PublicID, comment.Body, postID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := vk.Do(r.Context(), vk.B().Incr().Key(fmt.Sprintf("post:%s:comment_count", postIDStr)).Build()).Error(); err != nil {
			slog.ErrorContext(r.Context(), "failed to increment comment count in valkey", slog.Any("error", err))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comment)
	}
}
