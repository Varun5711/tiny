-- Create analytics database
CREATE DATABASE IF NOT EXISTS analytics;

-- Create click_events table with MergeTree engine
-- Optimized for analytical queries with partitioning by date
CREATE TABLE IF NOT EXISTS analytics.click_events
(
    event_id String,
    short_code String,
    original_url String,
    clicked_at DateTime64(3),
    clicked_date Date DEFAULT toDate(clicked_at),

    -- IP and Location data
    ip_address String,
    country String,
    country_code String,
    region String,
    city String,
    latitude Float64,
    longitude Float64,
    timezone String,

    -- User Agent data
    user_agent String,
    browser String,
    browser_version String,
    os String,
    os_version String,
    device_type String,
    device_brand String,
    device_model String,
    is_mobile UInt8,
    is_tablet UInt8,
    is_desktop UInt8,
    is_bot UInt8,

    -- Request metadata
    referer String,
    query_params String,

    -- Processing metadata
    processed_at DateTime64(3) DEFAULT now64(),

    INDEX idx_short_code short_code TYPE bloom_filter GRANULARITY 4,
    INDEX idx_country country TYPE bloom_filter GRANULARITY 4,
    INDEX idx_browser browser TYPE bloom_filter GRANULARITY 4,
    INDEX idx_os os TYPE bloom_filter GRANULARITY 4
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(clicked_date)
ORDER BY (short_code, clicked_at)
TTL clicked_date + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;

-- Create materialized view for daily aggregations by short_code
CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.daily_clicks_by_url
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(clicked_date)
ORDER BY (short_code, clicked_date)
AS SELECT
    short_code,
    clicked_date,
    count() AS click_count,
    uniq(ip_address) AS unique_visitors
FROM analytics.click_events
GROUP BY short_code, clicked_date;

-- Create materialized view for country statistics
CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.clicks_by_country
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(clicked_date)
ORDER BY (short_code, clicked_date, country)
AS SELECT
    short_code,
    clicked_date,
    country,
    country_code,
    count() AS click_count
FROM analytics.click_events
WHERE country != ''
GROUP BY short_code, clicked_date, country, country_code;

-- Create materialized view for device statistics
CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.clicks_by_device
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(clicked_date)
ORDER BY (short_code, clicked_date, device_type, browser, os)
AS SELECT
    short_code,
    clicked_date,
    device_type,
    browser,
    os,
    count() AS click_count
FROM analytics.click_events
GROUP BY short_code, clicked_date, device_type, browser, os;

-- Create materialized view for hourly time series
CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.hourly_clicks
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(clicked_date)
ORDER BY (short_code, clicked_hour)
AS SELECT
    short_code,
    toStartOfHour(clicked_at) AS clicked_hour,
    toDate(clicked_at) AS clicked_date,
    count() AS click_count,
    uniq(ip_address) AS unique_visitors
FROM analytics.click_events
GROUP BY short_code, clicked_hour, clicked_date;
