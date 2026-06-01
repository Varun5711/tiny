// Package config provides centralized, environment-based configuration for
// all Tiny URL shortener services. It uses godotenv to optionally load a .env
// file for local development, while in production (Kubernetes) configuration
// is injected via ConfigMaps and Secrets as real environment variables.
//
// Every configuration value has a sensible default for local development, so
// the application can start with zero configuration. Production deployments
// should override security-sensitive values (DB_PRIMARY_DSN, JWT_SECRET,
// REDIS_PASSWORD, etc.) via their orchestration layer.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the top-level configuration struct that aggregates all subsystem
// configs. It is typically created once at startup via Load() and passed by
// pointer to service constructors via dependency injection.
type Config struct {
	Database      DatabaseConfig
	Redis         RedisConfig
	ClickHouse    ClickHouseConfig
	Elasticsearch ElasticsearchConfig
	Tracing       TracingConfig
	Services      ServicesConfig
	Analytics     AnalyticsConfig
	Snowflake     SnowflakeConfig
	Cache         CacheConfig
	RateLimit     RateLimitConfig
	CORS          CORSConfig
	JWT           JWTConfig
}

// TracingConfig holds settings for distributed tracing via OpenTelemetry/Jaeger.
// When Enabled is false, the tracing middleware becomes a no-op to avoid
// overhead in environments without a tracing backend.
type TracingConfig struct {
	Enabled        bool
	JaegerEndpoint string
	SampleRate     float64
}

// ElasticsearchConfig holds connection parameters for Elasticsearch, used for
// full-text search over shortened URLs and analytics data. When Enabled is
// false, search features degrade gracefully to database-backed queries.
type ElasticsearchConfig struct {
	Addresses   []string
	Username    string
	Password    string
	IndexPrefix string
	Enabled     bool
}

// CORSConfig specifies which origins are allowed to make cross-origin requests
// to the API gateway. In production this should be set to the frontend domain(s).
type CORSConfig struct {
	AllowedOrigins []string
}

// JWTConfig holds the HMAC-SHA256 secret and token lifetime for the auth
// package's JWTManager. The Secret must be kept confidential; the
// TokenDuration controls how long access tokens remain valid before the
// client must re-authenticate.
type JWTConfig struct {
	Secret        string
	TokenDuration time.Duration
}

// DatabaseConfig holds PostgreSQL connection parameters for both the primary
// (read-write) instance and up to N read replicas. The connection pool
// settings (MaxConns, MinConns, lifetimes) apply uniformly to all pools.
type DatabaseConfig struct {
	// PrimaryDSN is the PostgreSQL connection string for the read-write primary.
	PrimaryDSN string

	// ReplicaDSNs is a list of connection strings for read-only replicas.
	// The database manager distributes read queries across these replicas
	// using round-robin load balancing.
	ReplicaDSNs []string

	// MaxConns is the maximum number of connections in each pool (primary
	// and per-replica). This should be tuned based on the expected query
	// concurrency and the PostgreSQL max_connections setting.
	MaxConns int32

	// MinConns is the minimum number of idle connections maintained in each
	// pool. Keeping warm connections avoids the latency of establishing new
	// TCP connections under sudden load spikes.
	MinConns int32

	// MaxConnLifetime is the maximum duration a connection can be reused
	// before being closed and replaced. This helps distribute connections
	// across database nodes after a failover or rebalancing event.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum duration a connection can sit idle
	// before being closed. This prevents accumulation of stale connections
	// during low-traffic periods.
	MaxConnIdleTime time.Duration
}

// RedisConfig holds connection parameters for the Redis instance used for
// caching (L2 cache layer) and as a message broker (click event streams).
type RedisConfig struct {
	Addr       string
	Password   string
	DB         int
	StreamName string
}

// ClickHouseConfig holds connection parameters for the ClickHouse analytics
// database, which stores aggregated click event data for reporting.
type ClickHouseConfig struct {
	Addr     string
	Database string
	Username string
	Password string
	MaxConns int
}

// ServicesConfig holds addresses, ports, and settings for inter-service
// communication and the public-facing URL base.
type ServicesConfig struct {
	// URLServiceAddr is the gRPC address of the URL shortening service.
	URLServiceAddr string

	// APIGatewayPort is the HTTP port the API gateway listens on.
	APIGatewayPort string

	// RedirectServicePort is the HTTP port the redirect service listens on.
	RedirectServicePort string

	// BaseURL is the public-facing base URL prepended to short codes
	// (e.g., "https://tiny.example.com") to form complete short URLs.
	BaseURL string

	// DefaultURLTTL is the default time-to-live for shortened URLs when
	// the user does not specify a custom expiration.
	DefaultURLTTL time.Duration
}

// AnalyticsConfig holds settings for the Redis Streams consumer that
// processes click events and writes them to ClickHouse in batches.
type AnalyticsConfig struct {
	ConsumerGroup string
	ConsumerName  string
	BatchSize     int
	PollInterval  time.Duration
	BlockTime     time.Duration
}

// SnowflakeConfig holds the datacenter and worker IDs passed to the
// Snowflake ID generator. Each deployment instance must have a unique
// (DatacenterID, WorkerID) pair to guarantee globally unique IDs.
type SnowflakeConfig struct {
	DatacenterID int64
	WorkerID     int64
}

// CacheConfig controls the two-tier caching layer. L1 is an in-process LRU
// cache (bounded by entry count), while L2 is the Redis-backed cache
// (bounded by TTL).
type CacheConfig struct {
	// L1Capacity is the maximum number of entries in the in-process LRU cache.
	L1Capacity int

	// L2TTL is the time-to-live for entries in the Redis L2 cache.
	L2TTL time.Duration
}

// RateLimitConfig controls the sliding-window rate limiter applied to API
// requests. Requests is the maximum allowed count within the Window duration.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

// Load reads configuration from environment variables, with optional .env
// file support via godotenv. It returns a fully populated Config with
// defaults suitable for local development.
//
// The .env file load error is intentionally ignored: in production the file
// does not exist and all values come from real environment variables injected
// by Kubernetes. In local development the .env file provides convenience
// overrides.
func Load() (*Config, error) {
	// Load .env if it exists (local dev), ignore if not (K8s uses ConfigMaps/Secrets)
	_ = godotenv.Load()

	cfg := &Config{
		Database: DatabaseConfig{
			PrimaryDSN: getEnv("DB_PRIMARY_DSN", ""),
			ReplicaDSNs: []string{
				getEnv("DB_REPLICA1_DSN", ""),
				getEnv("DB_REPLICA2_DSN", ""),
				getEnv("DB_REPLICA3_DSN", ""),
			},
			MaxConns:        int32(getEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:        int32(getEnvAsInt("DB_MIN_CONNS", 5)),
			MaxConnLifetime: getEnvAsDuration("DB_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: getEnvAsDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		},
		Redis: RedisConfig{
			Addr:       getEnv("REDIS_ADDR", "localhost:6379"),
			Password:   getEnv("REDIS_PASSWORD", ""),
			DB:         getEnvAsInt("REDIS_DB", 0),
			StreamName: getEnv("REDIS_STREAM_NAME", "clicks:stream"),
		},
		Services: ServicesConfig{
			URLServiceAddr:      getEnv("URL_SERVICE_ADDR", "localhost:50051"),
			APIGatewayPort:      getEnv("API_GATEWAY_PORT", "8080"),
			RedirectServicePort: getEnv("REDIRECT_SERVICE_PORT", "8081"),
			BaseURL:             getEnv("BASE_URL", "http://localhost:8081"),
			DefaultURLTTL:       getEnvAsDuration("DEFAULT_URL_TTL", 3*24*time.Hour),
		},
		Analytics: AnalyticsConfig{
			ConsumerGroup: getEnv("ANALYTICS_CONSUMER_GROUP", "analytics-group"),
			ConsumerName:  getEnv("ANALYTICS_CONSUMER_NAME", "worker-1"),
			BatchSize:     getEnvAsInt("ANALYTICS_BATCH_SIZE", 100),
			PollInterval:  getEnvAsDuration("ANALYTICS_POLL_INTERVAL", time.Second),
			BlockTime:     getEnvAsDuration("ANALYTICS_BLOCK_TIME", 5*time.Second),
		},
		ClickHouse: ClickHouseConfig{
			Addr:     getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
			Database: getEnv("CLICKHOUSE_DATABASE", "analytics"),
			Username: getEnv("CLICKHOUSE_USERNAME", "clickhouse"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
			MaxConns: getEnvAsInt("CLICKHOUSE_MAX_CONNS", 10),
		},
		Cache: CacheConfig{
			L1Capacity: getEnvAsInt("CACHE_L1_CAPACITY", 10000),
			L2TTL:      getEnvAsDuration("CACHE_L2_TTL", time.Hour),
		},
		RateLimit: RateLimitConfig{
			Requests: getEnvAsInt("RATE_LIMIT_REQUESTS", 100),
			Window:   getEnvAsDuration("RATE_LIMIT_WINDOW", time.Minute),
		},
		Snowflake: SnowflakeConfig{
			DatacenterID: int64(getEnvAsInt("SNOWFLAKE_DATACENTER_ID", 1)),
			WorkerID:     int64(getEnvAsInt("SNOWFLAKE_WORKER_ID", 1)),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		},
		Tracing: TracingConfig{
			Enabled:        getEnv("TRACING_ENABLED", "false") == "true",
			JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
			SampleRate:     getEnvAsFloat("TRACING_SAMPLE_RATE", 1.0),
		},
		Elasticsearch: ElasticsearchConfig{
			Addresses:   getEnvAsSlice("ES_ADDRESSES", []string{"http://localhost:9200"}),
			Username:    getEnv("ES_USERNAME", ""),
			Password:    getEnv("ES_PASSWORD", ""),
			IndexPrefix: getEnv("ES_INDEX_PREFIX", "shorternit"),
			Enabled:     getEnv("ES_ENABLED", "false") == "true",
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", ""),
			TokenDuration: getEnvAsDuration("JWT_TOKEN_DURATION", 7*24*time.Hour),
		},
	}

	return cfg, nil
}

// getEnv retrieves a string environment variable, returning defaultValue if
// the variable is unset or empty.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt retrieves an environment variable and parses it as an integer.
// Returns defaultValue if the variable is unset, empty, or not a valid integer.
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvAsSlice retrieves an environment variable and splits it on commas
// into a string slice. Each element is trimmed of whitespace. Returns
// defaultValue if the variable is unset, empty, or produces no non-empty
// elements after splitting.
func getEnvAsSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// getEnvAsFloat retrieves an environment variable and parses it as a float64.
// Returns defaultValue if the variable is unset, empty, or not a valid
// floating-point number.
func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// getEnvAsDuration retrieves an environment variable and parses it as a
// time.Duration using Go's duration string format (e.g., "30m", "1h30m",
// "500ms"). Returns defaultValue if the variable is unset, empty, or not
// a valid duration string.
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
