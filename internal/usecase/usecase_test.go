package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

// fakeSourceRepo SourceRepositoryのインメモリ実装です。
type fakeSourceRepo struct {
	orders    []domain.Order
	events    []domain.OutboxEvent
	published map[int64]bool
	createErr error
	fetchErr  error
	markErr   error
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{published: map[int64]bool{}}
}

func (f *fakeSourceRepo) CreateOrderWithOutbox(_ context.Context, order domain.Order, event domain.OutboxEvent) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.orders = append(f.orders, order)
	event.ID = int64(len(f.events) + 1)
	f.events = append(f.events, event)
	return nil
}

func (f *fakeSourceRepo) FetchUnpublished(_ context.Context, limit int) ([]domain.OutboxEvent, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	var out []domain.OutboxEvent
	for _, e := range f.events {
		if !f.published[e.ID] && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeSourceRepo) MarkPublished(_ context.Context, ids []int64) error {
	if f.markErr != nil {
		return f.markErr
	}
	for _, id := range ids {
		f.published[id] = true
	}
	return nil
}

// fakePublisher EventPublisherのフェイク実装です。
type fakePublisher struct {
	published [][]domain.OutboxEvent
	err       error
}

func (f *fakePublisher) Publish(_ context.Context, events []domain.OutboxEvent) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, events)
	return nil
}

func TestOrderUsecaseCreateOrder(t *testing.T) {
	t.Run("注文とoutboxイベントが同時に記録される", func(t *testing.T) {
		repo := newFakeSourceRepo()
		uc := NewOrderUsecase(repo)
		order, err := uc.CreateOrder(context.Background(), CreateOrderInput{CustomerID: "cust-1", Amount: "300"})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(repo.orders) != 1 || len(repo.events) != 1 {
			t.Fatalf("記録件数が不正です: orders=%d events=%d", len(repo.orders), len(repo.events))
		}
		if repo.events[0].AggregateID != order.ID {
			t.Errorf("イベントの集約IDが注文IDと一致しません")
		}
	})

	t.Run("入力が不正ならリポジトリは呼ばれない", func(t *testing.T) {
		repo := newFakeSourceRepo()
		uc := NewOrderUsecase(repo)
		if _, err := uc.CreateOrder(context.Background(), CreateOrderInput{}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("ErrInvalidInputを期待しました: %v", err)
		}
		if len(repo.orders) != 0 {
			t.Error("リポジトリが呼ばれています")
		}
	})

	t.Run("リポジトリのエラーが伝播する", func(t *testing.T) {
		repo := newFakeSourceRepo()
		repo.createErr = errors.New("db down")
		uc := NewOrderUsecase(repo)
		if _, err := uc.CreateOrder(context.Background(), CreateOrderInput{CustomerID: "c", Amount: "1"}); err == nil {
			t.Error("エラーを期待しました")
		}
	})
}

func TestRelayUsecaseRelayOnce(t *testing.T) {
	seed := func(t *testing.T, repo *fakeSourceRepo, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			order, err := domain.NewOrder("cust", "100", time.Now())
			if err != nil {
				t.Fatalf("注文生成に失敗しました: %v", err)
			}
			event, err := domain.NewOrderCreatedEvent(order)
			if err != nil {
				t.Fatalf("イベント生成に失敗しました: %v", err)
			}
			if err := repo.CreateOrderWithOutbox(context.Background(), order, event); err != nil {
				t.Fatalf("挿入に失敗しました: %v", err)
			}
		}
	}

	t.Run("未発行イベントが発行済みになる", func(t *testing.T) {
		repo := newFakeSourceRepo()
		seed(t, repo, 3)
		pub := &fakePublisher{}
		relay := NewRelayUsecase(repo, pub, 10)
		n, err := relay.RelayOnce(context.Background())
		if err != nil || n != 3 {
			t.Fatalf("3件の中継を期待しました: n=%d err=%v", n, err)
		}
		remaining, err := repo.FetchUnpublished(context.Background(), 10)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("未発行が残っています: %d件", len(remaining))
		}
	})

	t.Run("発行に失敗したらpublishedを更新しない", func(t *testing.T) {
		repo := newFakeSourceRepo()
		seed(t, repo, 2)
		pub := &fakePublisher{err: errors.New("kinesis down")}
		relay := NewRelayUsecase(repo, pub, 10)
		if _, err := relay.RelayOnce(context.Background()); err == nil {
			t.Fatal("エラーを期待しました")
		}
		remaining, err := repo.FetchUnpublished(context.Background(), 10)
		if err != nil {
			t.Fatalf("取得に失敗しました: %v", err)
		}
		if len(remaining) != 2 {
			t.Errorf("再送対象が残っているべきです: %d件", len(remaining))
		}
	})

	t.Run("未発行が無ければ何もしない", func(t *testing.T) {
		repo := newFakeSourceRepo()
		pub := &fakePublisher{}
		relay := NewRelayUsecase(repo, pub, 0)
		n, err := relay.RelayOnce(context.Background())
		if err != nil || n != 0 {
			t.Errorf("0件を期待しました: n=%d err=%v", n, err)
		}
		if len(pub.published) != 0 {
			t.Error("発行が呼ばれています")
		}
	})
}

// fakeTargetRepo TargetRepositoryのインメモリ実装です。
type fakeTargetRepo struct {
	processed map[string]bool
	orders    map[string]domain.ReplicatedOrder
}

func newFakeTargetRepo() *fakeTargetRepo {
	return &fakeTargetRepo{processed: map[string]bool{}, orders: map[string]domain.ReplicatedOrder{}}
}

func (f *fakeTargetRepo) ReplicateOrder(_ context.Context, in domain.ReplicatedOrder) error {
	if f.processed[in.EventID] {
		return domain.ErrDuplicateEvent
	}
	f.processed[in.EventID] = true
	f.orders[in.OrderID] = in
	return nil
}

func (f *fakeTargetRepo) FindOrder(_ context.Context, orderID string) (domain.ReplicatedOrder, error) {
	order, ok := f.orders[orderID]
	if !ok {
		return domain.ReplicatedOrder{}, domain.ErrNotFound
	}
	return order, nil
}

func TestReplicationUsecase(t *testing.T) {
	valid := domain.ReplicatedOrder{
		EventID: "ev-1", OrderID: "ord-1", CustomerID: "cust-1", Amount: "100", Status: "created", Seq: 1,
	}

	t.Run("反映して取得できる", func(t *testing.T) {
		uc := NewReplicationUsecase(newFakeTargetRepo())
		if err := uc.Replicate(context.Background(), valid); err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		got, err := uc.GetOrder(context.Background(), "ord-1")
		if err != nil || got.EventID != "ev-1" {
			t.Errorf("取得結果が不正です: %+v err=%v", got, err)
		}
	})

	t.Run("同じイベントIDはErrDuplicateEvent", func(t *testing.T) {
		uc := NewReplicationUsecase(newFakeTargetRepo())
		if err := uc.Replicate(context.Background(), valid); err != nil {
			t.Fatalf("1回目に失敗しました: %v", err)
		}
		if err := uc.Replicate(context.Background(), valid); !errors.Is(err, domain.ErrDuplicateEvent) {
			t.Errorf("ErrDuplicateEventを期待しました: %v", err)
		}
	})

	t.Run("入力が不正ならErrInvalidInput", func(t *testing.T) {
		uc := NewReplicationUsecase(newFakeTargetRepo())
		if err := uc.Replicate(context.Background(), domain.ReplicatedOrder{}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ErrInvalidInputを期待しました: %v", err)
		}
	})

	t.Run("存在しない注文はErrNotFound", func(t *testing.T) {
		uc := NewReplicationUsecase(newFakeTargetRepo())
		if _, err := uc.GetOrder(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ErrNotFoundを期待しました: %v", err)
		}
	})

	t.Run("IDが空ならErrInvalidInput", func(t *testing.T) {
		uc := NewReplicationUsecase(newFakeTargetRepo())
		if _, err := uc.GetOrder(context.Background(), ""); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ErrInvalidInputを期待しました: %v", err)
		}
	})
}
