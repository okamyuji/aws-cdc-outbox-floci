// ローカル環境専用のリレー。outboxテーブルをポーリングし、
// DMS互換エンベロープでKinesisへ中継します。stg環境ではAWS DMSがこの役割を担います。
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	_ "github.com/go-sql-driver/mysql"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/adapter/persistence"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/adapter/publisher"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dsn := os.Getenv("SOURCE_DB_DSN")
	streamName := os.Getenv("KINESIS_STREAM_NAME")
	if dsn == "" || streamName == "" {
		logger.Error("SOURCE_DB_DSNとKINESIS_STREAM_NAMEは必須です")
		os.Exit(1)
	}
	interval := 500 * time.Millisecond
	if v := os.Getenv("POLL_INTERVAL_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil || ms <= 0 {
			logger.Error("POLL_INTERVAL_MSが不正です", "value", v)
			os.Exit(1)
		}
		interval = time.Duration(ms) * time.Millisecond
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// AWS_ENDPOINT_URLが設定されていればSDKが自動でflociへ向く
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("AWS設定の読み込みに失敗しました", "error", err)
		os.Exit(1)
	}

	relay := usecase.NewRelayUsecase(
		persistence.NewSourceMySQL(db),
		publisher.NewKinesisPublisher(kinesis.NewFromConfig(awsCfg), streamName),
		100,
	)

	logger.Info("リレーを起動します", "stream", streamName, "interval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("リレーを停止します")
			return
		case <-ticker.C:
			n, err := relay.RelayOnce(ctx)
			if err != nil {
				logger.Error("中継に失敗しました。次のポーリングで再試行します", "error", err)
				continue
			}
			if n > 0 {
				logger.Info("outboxイベントを中継しました", "count", n)
			}
		}
	}
}
