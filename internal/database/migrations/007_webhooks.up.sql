CREATE TABLE webhooks (
    id         BIGSERIAL PRIMARY KEY,
    repo_id    BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    secret     VARCHAR(255) NOT NULL DEFAULT '',
    events     TEXT[] NOT NULL DEFAULT '{}',
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_webhooks_repo ON webhooks(repo_id);

CREATE TABLE webhook_deliveries (
    id            BIGSERIAL PRIMARY KEY,
    webhook_id    BIGINT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event         VARCHAR(50) NOT NULL,
    payload       JSONB NOT NULL,
    response_code INTEGER,
    response_body TEXT,
    delivered_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms   INTEGER
);
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_time ON webhook_deliveries(delivered_at DESC);
