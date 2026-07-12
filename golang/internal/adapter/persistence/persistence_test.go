package persistence

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

var (
	sourceDB *sql.DB
	targetDB *sql.DB
)

// TestMain MySQL 8コンテナを1回だけ起動し、両スキーマを適用します。
func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("source_orders"),
		mysql.WithUsername("root"),
		mysql.WithPassword("test"),
		mysql.WithScripts(
			filepath.Join("..", "..", "..", "..", "db", "source_schema.sql"),
			filepath.Join("..", "..", "..", "..", "db", "target_schema.sql"),
		),
	)
	if err != nil {
		log.Fatalf("MySQLコンテナの起動に失敗しました: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			log.Printf("コンテナの停止に失敗しました: %v", err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		log.Fatalf("接続文字列の取得に失敗しました: %v", err)
	}

	sourceDB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("ソースDB接続に失敗しました: %v", err)
	}
	targetDB, err = sql.Open("mysql", strings.Replace(dsn, "/source_orders?", "/target_orders?", 1))
	if err != nil {
		log.Fatalf("ターゲットDB接続に失敗しました: %v", err)
	}

	code := m.Run()
	if err := sourceDB.Close(); err != nil {
		log.Printf("ソースDBのクローズに失敗しました: %v", err)
	}
	if err := targetDB.Close(); err != nil {
		log.Printf("ターゲットDBのクローズに失敗しました: %v", err)
	}
	os.Exit(code)
}

// resetSourceTables サブテスト間の共有状態を排除するため、ソース側テーブルを空にします。
func resetSourceTables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"outbox", "orders"} {
		if _, err := sourceDB.Exec("TRUNCATE TABLE " + table); err != nil {
			t.Fatalf("%sのTRUNCATEに失敗しました: %v", table, err)
		}
	}
}

// resetTargetTables ターゲット側テーブルを空にします。
func resetTargetTables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"orders_replica", "processed_events"} {
		if _, err := targetDB.Exec("TRUNCATE TABLE " + table); err != nil {
			t.Fatalf("%sのTRUNCATEに失敗しました: %v", table, err)
		}
	}
}

func mustOrderAndEvent(t *testing.T) (domain.Order, domain.OutboxEvent) {
	t.Helper()
	order, err := domain.NewOrder("cust-1", "1200.50", time.Now())
	if err != nil {
		t.Fatalf("注文生成に失敗しました: %v", err)
	}
	event, err := domain.NewOrderCreatedEvent(order)
	if err != nil {
		t.Fatalf("イベント生成に失敗しました: %v", err)
	}
	return order, event
}

func TestSourceMySQL(t *testing.T) {
	ctx := context.Background()
	repo := NewSourceMySQL(sourceDB)

	t.Run("注文とoutboxが同一トランザクションで挿入される", func(t *testing.T) {
		resetSourceTables(t)
		order, event := mustOrderAndEvent(t)
		if err := repo.CreateOrderWithOutbox(ctx, order, event); err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		var orderCount, outboxCount int
		if err := sourceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM orders WHERE id = ?`, order.ID).Scan(&orderCount); err != nil {
			t.Fatalf("注文件数の取得に失敗しました: %v", err)
		}
		if err := sourceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM outbox WHERE event_id = ?`, event.EventID).Scan(&outboxCount); err != nil {
			t.Fatalf("outbox件数の取得に失敗しました: %v", err)
		}
		if orderCount != 1 || outboxCount != 1 {
			t.Errorf("挿入件数が不正です: orders=%d outbox=%d", orderCount, outboxCount)
		}
	})

	t.Run("outbox挿入が失敗すると注文もロールバックされる", func(t *testing.T) {
		resetSourceTables(t)
		order, event := mustOrderAndEvent(t)
		// 同じevent_idを2回挿入してUNIQUE制約違反を誘発する
		if err := repo.CreateOrderWithOutbox(ctx, order, event); err != nil {
			t.Fatalf("1回目の挿入に失敗しました: %v", err)
		}
		order2, _ := mustOrderAndEvent(t)
		err := repo.CreateOrderWithOutbox(ctx, order2, event)
		if err == nil {
			t.Fatal("UNIQUE制約違反を期待しました")
		}
		var count int
		if err := sourceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM orders WHERE id = ?`, order2.ID).Scan(&count); err != nil {
			t.Fatalf("件数の取得に失敗しました: %v", err)
		}
		if count != 0 {
			t.Errorf("注文がロールバックされていません: count=%d", count)
		}
	})

	t.Run("未発行の取得と発行済み更新がID昇順で機能する", func(t *testing.T) {
		resetSourceTables(t)
		var wantEventIDs []string
		for i := 0; i < 3; i++ {
			order, event := mustOrderAndEvent(t)
			if err := repo.CreateOrderWithOutbox(ctx, order, event); err != nil {
				t.Fatalf("挿入に失敗しました: %v", err)
			}
			wantEventIDs = append(wantEventIDs, event.EventID)
		}
		events, err := repo.FetchUnpublished(ctx, 10)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if len(events) != 3 {
			t.Fatalf("3件を期待しました: %d件", len(events))
		}
		for i, e := range events {
			if e.EventID != wantEventIDs[i] {
				t.Errorf("順序が不正です: %d番目 %s != %s", i, e.EventID, wantEventIDs[i])
			}
		}
		ids := []int64{events[0].ID, events[1].ID, events[2].ID}
		if err := repo.MarkPublished(ctx, ids); err != nil {
			t.Fatalf("発行済み更新に失敗しました: %v", err)
		}
		remaining, err := repo.FetchUnpublished(ctx, 10)
		if err != nil {
			t.Fatalf("再取得に失敗しました: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("未発行が残っています: %d件", len(remaining))
		}
	})

	t.Run("空のID配列では何も更新しない", func(t *testing.T) {
		resetSourceTables(t)
		if err := repo.MarkPublished(ctx, nil); err != nil {
			t.Errorf("エラーは想定外です: %v", err)
		}
	})
}

func TestTargetMySQL(t *testing.T) {
	ctx := context.Background()
	repo := NewTargetMySQL(targetDB)

	newInput := func(t *testing.T) domain.ReplicatedOrder {
		t.Helper()
		eventID, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("UUID生成に失敗しました: %v", err)
		}
		orderID, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("UUID生成に失敗しました: %v", err)
		}
		return domain.ReplicatedOrder{
			EventID: eventID, OrderID: orderID, CustomerID: "cust-1", Amount: "980.00", Status: "created", Seq: 1,
		}
	}

	t.Run("反映した注文が取得できる", func(t *testing.T) {
		resetTargetTables(t)
		in := newInput(t)
		if err := repo.ReplicateOrder(ctx, in); err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		got, err := repo.FindOrder(ctx, in.OrderID)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if got != in {
			t.Errorf("取得結果が一致しません: %+v != %+v", got, in)
		}
	})

	t.Run("同じイベントIDの2回目はErrDuplicateEventで反映されない", func(t *testing.T) {
		resetTargetTables(t)
		in := newInput(t)
		if err := repo.ReplicateOrder(ctx, in); err != nil {
			t.Fatalf("1回目に失敗しました: %v", err)
		}
		dup := in
		dup.Amount = "1.00"
		if err := repo.ReplicateOrder(ctx, dup); !errors.Is(err, domain.ErrDuplicateEvent) {
			t.Fatalf("ErrDuplicateEventを期待しました: %v", err)
		}
		got, err := repo.FindOrder(ctx, in.OrderID)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if got.Amount != "980.00" {
			t.Errorf("重複リクエストで金額が書き換わっています: %s", got.Amount)
		}
	})

	t.Run("順序が逆転して届いた古いイベントでは上書きされない", func(t *testing.T) {
		resetTargetTables(t)
		newer := newInput(t)
		newer.Seq = 20
		newer.Status = "shipped"
		if err := repo.ReplicateOrder(ctx, newer); err != nil {
			t.Fatalf("新イベントの反映に失敗しました: %v", err)
		}
		older := newer
		olderEventID, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("UUID生成に失敗しました: %v", err)
		}
		older.EventID = olderEventID
		older.Seq = 10
		older.Status = "created"
		older.Amount = "1.00"
		// 古いイベントはべき等キーとしては新規なので受理されるが、状態は巻き戻らない
		if err := repo.ReplicateOrder(ctx, older); err != nil {
			t.Fatalf("旧イベントの処理に失敗しました: %v", err)
		}
		got, err := repo.FindOrder(ctx, newer.OrderID)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if got.Status != "shipped" || got.Amount != "980.00" || got.Seq != 20 {
			t.Errorf("古いイベントで状態が巻き戻っています: %+v", got)
		}
	})

	t.Run("同一seqのイベントでは上書きされない", func(t *testing.T) {
		resetTargetTables(t)
		first := newInput(t)
		first.Seq = 5
		if err := repo.ReplicateOrder(ctx, first); err != nil {
			t.Fatalf("反映に失敗しました: %v", err)
		}
		same := first
		sameEventID, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("UUID生成に失敗しました: %v", err)
		}
		same.EventID = sameEventID
		same.Amount = "2.00"
		if err := repo.ReplicateOrder(ctx, same); err != nil {
			t.Fatalf("処理に失敗しました: %v", err)
		}
		got, err := repo.FindOrder(ctx, first.OrderID)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if got.Amount != "980.00" {
			t.Errorf("同一seqで上書きされています: %+v", got)
		}
	})

	t.Run("存在しない注文はErrNotFound", func(t *testing.T) {
		resetTargetTables(t)
		if _, err := repo.FindOrder(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ErrNotFoundを期待しました: %v", err)
		}
	})
}
