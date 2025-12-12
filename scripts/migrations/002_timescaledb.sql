CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS clicks (
    id BIGSERIAL,
    short_code VARCHAR(10) NOT NULL,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    click_id UUID NOT NULL,
    ip_address INET,
    user_agent TEXT,
    referer TEXT,
    country VARCHAR(2),
    city VARCHAR(100),
    device_type VARCHAR(50),
    browser VARCHAR(50),
    os VARCHAR(50)
);

SELECT create_hypertable('clicks', 'clicked_at', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_clicks_short_code
    ON clicks (short_code, clicked_at DESC);

CREATE INDEX IF NOT EXISTS idx_clicks_timestamp
    ON clicks (clicked_at DESC);

CREATE MATERIALIZED VIEW IF NOT EXISTS daily_stats
WITH (timescaledb.continuous) AS
SELECT
    short_code,
    time_bucket('1 day', clicked_at) AS day,
    COUNT(*) as total_clicks,
    COUNT(DISTINCT ip_address) as unique_visitors,
    COUNT(DISTINCT CASE WHEN device_type = 'mobile' THEN ip_address END) as mobile_clicks,
    COUNT(DISTINCT CASE WHEN device_type = 'desktop' THEN ip_address END) as desktop_clicks
FROM clicks
GROUP BY short_code, day
WITH NO DATA;

SELECT add_continuous_aggregate_policy('daily_stats',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE);

SELECT add_retention_policy('clicks', INTERVAL '90 days', if_not_exists => TRUE);

ALTER TABLE clicks SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'short_code'
);

SELECT add_compression_policy('clicks', INTERVAL '7 days', if_not_exists => TRUE);
