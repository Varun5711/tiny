DROP TABLE IF EXISTS urls CASCADE;
DROP TABLE IF EXISTS url_analytics CASCADE;
CREATE TABLE urls (
    short_code VARCHAR(10) PRIMARY KEY,
    long_url TEXT NOT NULL,
    clicks BIGINT DEFAULT 0 NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT long_url_not_empty CHECK (length(long_url) > 0),
    CONSTRAINT clicks_non_negative CHECK (clicks >= 0)
);

CREATE INDEX idx_urls_created_at ON urls(created_at DESC);

CREATE INDEX idx_urls_expires_at ON urls(expires_at)
WHERE expires_at IS NOT NULL;

CREATE TABLE url_analytics (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL REFERENCES urls(short_code) ON DELETE CASCADE,
    ip_address INET,
    user_agent TEXT,
    referer TEXT,
    country VARCHAR(2),
    city VARCHAR(100),

    clicked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_analytics_short_code ON url_analytics(short_code, clicked_at DESC);
CREATE INDEX idx_analytics_clicked_at ON url_analytics(clicked_at DESC);
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_urls_updated_at
    BEFORE UPDATE ON urls
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE urls IS 'Stores short code to long URL mappings';
COMMENT ON COLUMN urls.short_code IS 'Base62-encoded Snowflake ID (unique, short identifier)';
COMMENT ON COLUMN urls.clicks IS 'Total click count (incremented on each redirect)';
COMMENT ON COLUMN urls.expires_at IS 'Optional expiration timestamp (NULL = never expires)';

COMMENT ON TABLE url_analytics IS 'Detailed click analytics (optional, can be disabled for high-traffic URLs)';
COMMENT ON INDEX idx_urls_created_at IS 'Optimizes queries for recently created URLs';
COMMENT ON INDEX idx_urls_expires_at IS 'Partial index for expired URL cleanup jobs';
