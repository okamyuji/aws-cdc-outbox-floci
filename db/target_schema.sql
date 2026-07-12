-- ターゲット側スキーマ（反映先マイクロサービス）
CREATE DATABASE IF NOT EXISTS target_orders;
USE target_orders;

-- 反映先の注文テーブル
CREATE TABLE IF NOT EXISTS orders_replica (
    id           VARCHAR(36)    NOT NULL,
    customer_id  VARCHAR(36)    NOT NULL,
    amount       DECIMAL(12, 2) NOT NULL,
    status       VARCHAR(32)    NOT NULL,
    source_event_id VARCHAR(36) NOT NULL,
    -- ソースoutboxのID。順序逆行（古いイベントによる上書き）を防ぐ順序番号
    source_seq   BIGINT UNSIGNED NOT NULL,
    replicated_at DATETIME(6)   NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id)
) ENGINE = InnoDB;

-- べき等性担保テーブル（処理済みイベントIDを記録する）
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     VARCHAR(36) NOT NULL,
    processed_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (event_id)
) ENGINE = InnoDB;
