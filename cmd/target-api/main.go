// ターゲットサービスのエントリポイント。反映APIを起動します。
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/adapter/persistence"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/adapter/rest"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dsn := os.Getenv("TARGET_DB_DSN")
	if dsn == "" {
		logger.Error("TARGET_DB_DSNが未設定です")
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8082"
	}

	authToken := os.Getenv("AUTH_TOKEN")
	if authToken == "" {
		// fail-closed: 明示的にローカル環境を宣言した場合のみ無認証を許容する
		if os.Getenv("APP_ENV") != "local" {
			logger.Error("AUTH_TOKENが未設定です。ローカルで無認証にする場合はAPP_ENV=localを設定してください")
			os.Exit(1)
		}
		logger.Warn("認証なしで起動します（APP_ENV=local）")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Error("DB接続の初期化に失敗しました", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("DB接続のクローズに失敗しました", "error", err)
		}
	}()
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	mux := http.NewServeMux()
	handler := rest.NewTargetHandler(usecase.NewReplicationUsecase(persistence.NewTargetMySQL(db)), logger)
	handler.Register(mux)
	root := rest.WithBearerAuth(authToken, mux)

	srv := &http.Server{Addr: addr, Handler: root, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("ターゲットAPIを起動します", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("サーバーが異常終了しました", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("シャットダウンに失敗しました", "error", err)
	}
}
