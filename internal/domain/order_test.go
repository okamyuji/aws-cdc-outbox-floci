package domain

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewOrder(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)

	t.Run("正常系: 妥当な入力で注文が生成される", func(t *testing.T) {
		order, err := NewOrder("cust-1", "1200.50", now)
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if order.CustomerID != "cust-1" || order.Amount != "1200.50" || order.Status != "created" {
			t.Errorf("フィールドが不正です: %+v", order)
		}
		if order.ID == "" {
			t.Error("IDが採番されていません")
		}
		if !order.CreatedAt.Equal(now) {
			t.Errorf("作成日時が指定値と一致しません: %v", order.CreatedAt)
		}
	})

	t.Run("異常系: customer_idが空ならErrInvalidInput", func(t *testing.T) {
		if _, err := NewOrder("", "100", now); err == nil || !strings.Contains(err.Error(), "customer_id") {
			t.Errorf("customer_idの検証エラーを期待しました: %v", err)
		}
	})

	t.Run("境界値: amountの受理される境界", func(t *testing.T) {
		// 最小値、小数2桁ちょうど、整数部10桁ちょうど、10桁+小数2桁
		for _, amount := range []string{"0", "0.01", "9999999999", "9999999999.99", "1.5"} {
			if _, err := NewOrder("cust-1", amount, now); err != nil {
				t.Errorf("amount=%qは受理されるべきです: %v", amount, err)
			}
		}
	})

	t.Run("境界値: amountの拒否される境界", func(t *testing.T) {
		// 整数部11桁、小数3桁、小数点のみ、先頭小数点、末尾小数点
		for _, amount := range []string{"12345678901", "1.234", ".", ".5", "1."} {
			if _, err := NewOrder("cust-1", amount, now); err == nil {
				t.Errorf("amount=%qは拒否されるべきです", amount)
			}
		}
	})

	t.Run("異常系: amountが数値以外なら拒否される", func(t *testing.T) {
		for _, amount := range []string{"", "abc", "-100", "1e3", "100円", " 100", "100 "} {
			if _, err := NewOrder("cust-1", amount, now); err == nil {
				t.Errorf("amount=%qは拒否されるべきです", amount)
			}
		}
	})

	t.Run("エッジケース: 日付のゼロ値と遠い未来でも生成できる", func(t *testing.T) {
		for _, at := range []time.Time{{}, time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC)} {
			order, err := NewOrder("cust-1", "100", at)
			if err != nil {
				t.Fatalf("エラーは想定外です: %v", err)
			}
			if !order.CreatedAt.Equal(at) {
				t.Errorf("作成日時が保持されていません: %v != %v", order.CreatedAt, at)
			}
		}
	})
}

func TestNewOrderCreatedEvent(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)

	t.Run("正常系: 注文からorder.createdイベントが生成される", func(t *testing.T) {
		order, err := NewOrder("cust-1", "500", now)
		if err != nil {
			t.Fatalf("注文生成に失敗しました: %v", err)
		}
		event, err := NewOrderCreatedEvent(order)
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if event.AggregateID != order.ID || event.EventType != "order.created" {
			t.Errorf("イベントのフィールドが不正です: %+v", event)
		}
		var decoded Order
		if err := json.Unmarshal([]byte(event.Payload), &decoded); err != nil {
			t.Fatalf("ペイロードがJSONとして解析できません: %v", err)
		}
		if decoded.ID != order.ID {
			t.Errorf("ペイロードの注文IDが一致しません: %s != %s", decoded.ID, order.ID)
		}
	})

	t.Run("異常系: 注文IDが空ならエラー", func(t *testing.T) {
		if _, err := NewOrderCreatedEvent(Order{}); err == nil {
			t.Error("検証エラーを期待しました")
		}
	})

	t.Run("エッジケース: 顧客IDに記号や日本語を含むペイロードも往復できる", func(t *testing.T) {
		order, err := NewOrder(`cust-"квота"-<注文>&'`, "100", now)
		if err != nil {
			t.Fatalf("注文生成に失敗しました: %v", err)
		}
		event, err := NewOrderCreatedEvent(order)
		if err != nil {
			t.Fatalf("イベント生成に失敗しました: %v", err)
		}
		var decoded Order
		if err := json.Unmarshal([]byte(event.Payload), &decoded); err != nil {
			t.Fatalf("ペイロードの解析に失敗しました: %v", err)
		}
		if decoded.CustomerID != order.CustomerID {
			t.Errorf("顧客IDが往復で壊れています: %q", decoded.CustomerID)
		}
	})
}

func TestReplicatedOrderValidate(t *testing.T) {
	valid := ReplicatedOrder{
		EventID:    "ev-1",
		OrderID:    "ord-1",
		CustomerID: "cust-1",
		Amount:     "100",
		Status:     "created",
		Seq:        1,
	}

	t.Run("正常系: 妥当な入力は検証を通過する", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Errorf("エラーは想定外です: %v", err)
		}
	})

	t.Run("異常系: 必須項目が欠けると検証エラー", func(t *testing.T) {
		cases := []func(ReplicatedOrder) ReplicatedOrder{
			func(r ReplicatedOrder) ReplicatedOrder { r.EventID = ""; return r },
			func(r ReplicatedOrder) ReplicatedOrder { r.OrderID = ""; return r },
			func(r ReplicatedOrder) ReplicatedOrder { r.CustomerID = ""; return r },
			func(r ReplicatedOrder) ReplicatedOrder { r.Amount = "x"; return r },
			func(r ReplicatedOrder) ReplicatedOrder { r.Status = ""; return r },
		}
		for i, mutate := range cases {
			if err := mutate(valid).Validate(); err == nil {
				t.Errorf("ケース%dで検証エラーを期待しました", i)
			}
		}
	})

	t.Run("境界値: seqは1以上のみ受理される", func(t *testing.T) {
		cases := []struct {
			seq  int64
			want bool
		}{
			{math.MinInt64, false},
			{-1, false},
			{0, false},
			{1, true},
			{math.MaxInt64, true},
		}
		for _, c := range cases {
			r := valid
			r.Seq = c.seq
			err := r.Validate()
			if c.want && err != nil {
				t.Errorf("seq=%dは受理されるべきです: %v", c.seq, err)
			}
			if !c.want && err == nil {
				t.Errorf("seq=%dは拒否されるべきです", c.seq)
			}
		}
	})
}

func TestNewUUID(t *testing.T) {
	t.Run("正常系: UUIDv7形式で毎回異なる値が生成される", func(t *testing.T) {
		pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		seen := map[string]bool{}
		for range 100 {
			id, err := NewUUID()
			if err != nil {
				t.Fatalf("エラーは想定外です: %v", err)
			}
			if !pattern.MatchString(id) {
				t.Fatalf("UUIDv7形式ではありません: %s", id)
			}
			if seen[id] {
				t.Fatalf("重複したUUIDが生成されました: %s", id)
			}
			seen[id] = true
		}
	})

	t.Run("エッジケース: 時間が進むと辞書順も進む（時系列局所性）", func(t *testing.T) {
		first, err := NewUUID()
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
		second, err := NewUUID()
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if first >= second {
			t.Errorf("後から生成したUUIDが辞書順で前になっています: %s >= %s", first, second)
		}
	})
}
