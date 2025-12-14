package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Database   DatabaseConfig
	Redis      RedisConfig
	ClickHouse ClickHouseConfig
	Services   ServicesConfig
	Analytics  AnalyticsConfig
	Snowflake  SnowflakeConfig
	Cache      CacheConfig
	RateLimit  RateLimitConfig
}

type DatabaseConfig struct {
	PrimaryDSN      string
	ReplicaDSNs     []string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type RedisConfig struct {
	Addr       string
	Password   string
	DB         int
	StreamName string
}

type ClickHouseConfig struct {
	Addr     string
	Database string
	Username string
	Password string
	MaxConns int
}

type ServicesConfig struct {
	URLServiceAddr      string
	APIGatewayPort      string
	RedirectServicePort string
	BaseURL             string
	DefaultURLTTL       time.Duration
}

type AnalyticsConfig struct {
	ConsumerGroup string
	ConsumerName  string
	BatchSize     int
	PollInterval  time.Duration
	BlockTime     time.Duration
}

type SnowflakeConfig struct {
	DatacenterID int64
	WorkerID     int64
}

type CacheConfig struct {
	L1Capacity int
	L2TTL      time.Duration
}

type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

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
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
