package main

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"backend/internal/endpoint"
	"backend/internal/middleware"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dataSourceName := os.Getenv("DDFEED_BACKEND_DATA_SOURCE_NAME")
	if dataSourceName == "" {
		slog.Error("DDFEED_BACKEND_DATA_SOURCE_NAME is required")
		return
	}
	var db *sqlx.DB
	var dbConnectionError error
	for i := range 10 {
		db, dbConnectionError = sqlx.Connect("mysql", dataSourceName)
		if dbConnectionError != nil {
			slog.Debug("Failed to connect to database", slog.Any("error", dbConnectionError))
			time.Sleep(time.Second * time.Duration(i))
			continue
		}
		if db != nil {
			break
		}
	}
	if db == nil {
		slog.Error("Failed to connect to database", slog.Any("error", dbConnectionError))
		return
	}
	defer db.Close()

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{"valkey:6379"},
	})
	if err != nil {
		slog.Error("Failed to create Valkey client", slog.Any("error", err))
		return
	}

	endpoint.Register(http.HandleFunc, db, vk)

	port := os.Getenv("DDFEED_BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	slog.Info("Starting server on port " + port)
	go func() {
		if err := http.ListenAndServe(":"+port, middleware.Wrap(http.DefaultServeMux, middleware.CORS())); err != nil {
			slog.Error("Failed to start server", slog.Any("error", err))
		}
	}()

	<-ctx.Done()
	slog.Info("Server stopped")
}
