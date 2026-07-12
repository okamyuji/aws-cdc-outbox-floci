package envelope

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewInsertRecordAndParse(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	row := OutboxRow{
		ID:          1,
		EventID:     "ev-1",
		AggregateID: "ord-1",
		EventType:   "order.created",
		Payload:     `{"id":"ord-1"}`,
		CreatedAt:   now.Format(time.RFC3339Nano),
	}

	t.Run("生成したレコードを解析すると同じ行に戻る", func(t *testing.T) {
		rec, err := NewInsertRecord(row, now)
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("JSON変換に失敗しました: %v", err)
		}
		parsed, ok, err := ParseOutboxInsert(raw)
		if err != nil || !ok {
			t.Fatalf("解析に失敗しました: ok=%v err=%v", ok, err)
		}
		if parsed != row {
			t.Errorf("行が一致しません: %+v != %+v", parsed, row)
		}
	})

	t.Run("outbox以外のテーブルは読み飛ばす", func(t *testing.T) {
		raw := []byte(`{"data":{},"metadata":{"record-type":"data","operation":"insert","table-name":"orders"}}`)
		_, ok, err := ParseOutboxInsert(raw)
		if err != nil || ok {
			t.Errorf("読み飛ばしを期待しました: ok=%v err=%v", ok, err)
		}
	})

	t.Run("INSERT以外の操作は読み飛ばす", func(t *testing.T) {
		raw := []byte(`{"data":{},"metadata":{"record-type":"data","operation":"update","table-name":"outbox"}}`)
		_, ok, err := ParseOutboxInsert(raw)
		if err != nil || ok {
			t.Errorf("読み飛ばしを期待しました: ok=%v err=%v", ok, err)
		}
	})

	t.Run("event_idが空ならエラー", func(t *testing.T) {
		raw := []byte(`{"data":{"aggregate_id":"a"},"metadata":{"record-type":"data","operation":"insert","table-name":"outbox"}}`)
		if _, _, err := ParseOutboxInsert(raw); err == nil {
			t.Error("エラーを期待しました")
		}
	})

	t.Run("JSONとして不正ならエラー", func(t *testing.T) {
		if _, _, err := ParseOutboxInsert([]byte("{broken")); err == nil {
			t.Error("エラーを期待しました")
		}
	})
}
