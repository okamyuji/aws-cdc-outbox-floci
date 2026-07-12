// Package envelope AWS DMSのKinesisターゲットが出力するJSONエンベロープを定義します。
// ローカル環境のリレーも同じ形式で送信し、下流のLambdaを環境差なしで共通化します。
package envelope

import (
	"encoding/json"
	"fmt"
	"time"
)

// Metadata DMSがレコードに付与するメタデータです。
type Metadata struct {
	Timestamp  string `json:"timestamp"`
	RecordType string `json:"record-type"`
	Operation  string `json:"operation"`
	SchemaName string `json:"schema-name"`
	TableName  string `json:"table-name"`
}

// Record DMSのKinesisターゲット互換のレコードです。
type Record struct {
	Data     json.RawMessage `json:"data"`
	Metadata Metadata        `json:"metadata"`
}

// OutboxRow outboxテーブル1行分のデータ部です。
type OutboxRow struct {
	ID          int64  `json:"id"`
	EventID     string `json:"event_id"`
	AggregateID string `json:"aggregate_id"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload"`
	CreatedAt   string `json:"created_at"`
}

// NewInsertRecord outboxへのINSERTを表すDMS互換レコードを組み立てます。
func NewInsertRecord(row OutboxRow, now time.Time) (Record, error) {
	data, err := json.Marshal(row)
	if err != nil {
		return Record{}, fmt.Errorf("outbox行のJSON変換に失敗しました: %w", err)
	}
	return Record{
		Data: data,
		Metadata: Metadata{
			Timestamp:  now.UTC().Format(time.RFC3339Nano),
			RecordType: "data",
			Operation:  "insert",
			SchemaName: "source_orders",
			TableName:  "outbox",
		},
	}, nil
}

// ParseOutboxInsert DMS互換レコードを解析し、outboxへのINSERTのみを返します。
// INSERT以外の操作やoutbox以外のテーブルのレコードはok=falseで読み飛ばします。
func ParseOutboxInsert(raw []byte) (OutboxRow, bool, error) {
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return OutboxRow{}, false, fmt.Errorf("エンベロープの解析に失敗しました: %w", err)
	}
	if rec.Metadata.RecordType != "data" || rec.Metadata.Operation != "insert" || rec.Metadata.TableName != "outbox" {
		return OutboxRow{}, false, nil
	}
	var row OutboxRow
	if err := json.Unmarshal(rec.Data, &row); err != nil {
		return OutboxRow{}, false, fmt.Errorf("outbox行の解析に失敗しました: %w", err)
	}
	if row.EventID == "" || row.AggregateID == "" {
		return OutboxRow{}, false, fmt.Errorf("event_idまたはaggregate_idが空です: event_id=%q aggregate_id=%q", row.EventID, row.AggregateID)
	}
	// ペイロード破損はSequenceNumberを持つ最も早い地点(fanout)で検知して追跡可能にする
	if row.Payload == "" {
		return OutboxRow{}, false, fmt.Errorf("payloadが空です: event_id=%q", row.EventID)
	}
	return row, true, nil
}
