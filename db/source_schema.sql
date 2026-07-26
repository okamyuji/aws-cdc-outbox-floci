-- ソース側スキーマ（注文サービス）
CREATE DATABASE IF NOT EXISTS source_orders;
USE source_orders;

-- 業務テーブル
CREATE TABLE IF NOT EXISTS orders (
    id           VARCHAR(36)    NOT NULL,
    customer_id  VARCHAR(36)    NOT NULL,
    amount       DECIMAL(12, 2) NOT NULL,
    status       VARCHAR(32)    NOT NULL,
    created_at   DATETIME(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at   DATETIME(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id)
) ENGINE = InnoDB;

-- Outboxテーブル（業務テーブルと同一トランザクションで書き込む）
CREATE TABLE IF NOT EXISTS outbox (
    -- 符号付き。seqとして下流へ運ぶ値の上限をGoのint64・Railsの受理上限(2^63-1)と揃える
    id            BIGINT NOT NULL AUTO_INCREMENT,
    event_id      VARCHAR(36)     NOT NULL,
    aggregate_id  VARCHAR(36)     NOT NULL,
    event_type    VARCHAR(64)     NOT NULL,
    payload       JSON            NOT NULL,
    published     TINYINT(1)      NOT NULL DEFAULT 0,
    created_at    DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_outbox_event_id (event_id),
    KEY idx_outbox_published (published, id)
) ENGINE = InnoDB;
