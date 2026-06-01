// Package service implements the gRPC service layer for the tiny URL shortener.
//
// This package bridges incoming gRPC requests with the underlying storage,
// caching, search indexing, and ID generation subsystems. Each service struct
// embeds its corresponding protobuf UnimplementedServer to satisfy the gRPC
// interface contract, then overrides the methods it supports. The general
// request flow is:
//
//  1. Validate the inbound protobuf request.
//  2. Perform business logic (ID generation, locking, password hashing, etc.).
//  3. Persist changes through the storage layer (PostgreSQL via the Storage interface).
//  4. Update secondary stores (Redis cache, Elasticsearch) on a best-effort basis.
//  5. Map the domain model back into a protobuf response.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	es "github.com/Varun5711/shorternit/internal/elasticsearch"
	"github.com/Varun5711/shorternit/internal/idgen"
	"github.com/Varun5711/shorternit/internal/lock"
	"github.com/Varun5711/shorternit/internal/models"
	"github.com/Varun5711/shorternit/internal/qrcode"
	"github.com/Varun5711/shorternit/internal/storage"
	"github.com/Varun5711/shorternit/internal/validation"
	pb "github.com/Varun5711/shorternit/proto/url"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// URLService implements the gRPC URLServiceServer interface, orchestrating
// URL shortening, retrieval, deletion, and analytics. It embeds the
// UnimplementedURLServiceServer so that adding new RPCs to the proto
// definition does not break compilation until they are explicitly handled.
//
// Architecture: every write first goes to PostgreSQL (the source of truth),
// then updates Redis (cache) and Elasticsearch (search index) on a best-effort
// basis. Reads check the cache first and fall back to the database.
type URLService struct {
	pb.UnimplementedURLServiceServer
	store       storage.Storage     // Primary persistence (PostgreSQL via the Storage interface).
	idGen       *idgen.Generator    // Snowflake-based ID generator for globally unique short codes.
	cache       *cache.Cache        // Redis-backed cache mapping short codes to long URLs.
	redisClient *redis.Client       // Raw Redis client used for distributed locking (custom aliases).
	esClient    *es.Client          // Elasticsearch client for full-text search indexing; may be nil.
	baseURL     string              // Public-facing base URL prepended to short codes (e.g., "https://tiny.io").
	defaultTTL  time.Duration       // Default time-to-live applied when the caller does not specify an expiry.
}

// NewURLService constructs a URLService with all required dependencies. The
// esClient parameter may be nil if Elasticsearch is not configured, in which
// case indexing calls are silently skipped.
func NewURLService(store storage.Storage, idGen *idgen.Generator, urlCache *cache.Cache, redisClient *redis.Client, esClient *es.Client, baseURL string, defaultTTL time.Duration) *URLService {
	return &URLService{
		store:       store,
		idGen:       idGen,
		cache:       urlCache,
		redisClient: redisClient,
		esClient:    esClient,
		baseURL:     baseURL,
		defaultTTL:  defaultTTL,
	}
}

// CreateURL handles the gRPC CreateURL RPC. The flow is:
//  1. Generate a globally unique Snowflake ID and base62-encode it into a short code.
//  2. Determine the expiration time from the request or fall back to defaultTTL.
//  3. Generate a QR code image (base64 PNG) pointing to the short URL.
//  4. Persist the URL record to PostgreSQL via the Storage interface.
//  5. Index the document in Elasticsearch (best-effort, errors are swallowed).
//  6. Warm the Redis cache so the first redirect is served without a DB hit.
func (s *URLService) CreateURL(ctx context.Context, req *pb.CreateURLRequest) (*pb.CreateURLResponse, error) {
	if req.LongUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "long_url is required")
	}

	id, err := s.idGen.NextID()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate ID: %v", err)
	}

	shortCode := idgen.Encode(id)
	createdAt := time.Now()

	var expiresAt *time.Time
	if req.ExpiresAt > 0 {
		t := time.Unix(req.ExpiresAt, 0)
		expiresAt = &t
	} else if s.defaultTTL > 0 {
		t := createdAt.Add(s.defaultTTL)
		expiresAt = &t
	}

	shortURL := fmt.Sprintf("%s/%s", s.baseURL, shortCode)
	qrCodeData, err := qrcode.GenerateQRCode(shortURL)
	if err != nil {
		qrCodeData = ""
	}

	url := &models.URL{
		ShortCode: shortCode,
		LongURL:   req.LongUrl,
		Clicks:    0,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		QRCode:    qrCodeData,
		UserID:    req.UserId,
	}

	if err := s.store.Save(ctx, url); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save URL: %v", err)
	}

	if s.esClient != nil {
		_ = s.esClient.IndexURL(ctx, es.URLDocument{
			ShortCode: shortCode,
			LongURL:   req.LongUrl,
			UserID:    req.UserId,
			CreatedAt: createdAt,
			ExpiresAt: expiresAt,
			Clicks:    0,
		})
	}

	cacheKey := "url:" + shortCode
	_ = s.cache.Set(ctx, cacheKey, req.LongUrl)

	var expiresAtUnix int64
	if expiresAt != nil {
		expiresAtUnix = expiresAt.Unix()
	}

	return &pb.CreateURLResponse{
		ShortCode: shortCode,
		ShortUrl:  shortURL,
		LongUrl:   req.LongUrl,
		CreatedAt: createdAt.Unix(),
		ExpiresAt: expiresAtUnix,
		QrCode:    url.QRCode,
	}, nil
}

// GetURL handles the gRPC GetURL RPC. It looks up a URL by short code in
// PostgreSQL. If the URL exists and has not expired, it is returned wrapped
// in a protobuf response with Found=true. A missing or expired URL returns
// Found=false with a nil URL -- no gRPC error is raised for "not found" so
// the caller can distinguish "missing" from "server failure".
func (s *URLService) GetURL(ctx context.Context, req *pb.GetURLRequest) (*pb.GetURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	url, err := s.store.GetByShortCode(ctx, req.ShortCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get URL: %v", err)
	}

	if url == nil {
		return &pb.GetURLResponse{
			Found: false,
			Url:   nil,
		}, nil
	}

	pbURL := &pb.URL{
		ShortCode: url.ShortCode,
		LongUrl:   url.LongURL,
		Clicks:    url.Clicks,
		CreatedAt: url.CreatedAt.Unix(),
		UpdatedAt: url.CreatedAt.Unix(),
		IsActive:  true,
	}

	return &pb.GetURLResponse{
		Found: true,
		Url:   pbURL,
	}, nil
}

// ListURLs handles the gRPC ListURLs RPC with server-side pagination. When
// UserId is set on the request, only URLs belonging to that user are returned;
// otherwise all URLs are listed. Limit is clamped to [1, 1000] and offset
// defaults to 0 to prevent unbounded queries.
func (s *URLService) ListURLs(ctx context.Context, req *pb.ListURLsRequest) (*pb.ListURLsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	var urls []*models.URL
	var total int32
	var err error

	if req.UserId != "" {
		urls, total, err = s.store.ListByUserIDPaginated(ctx, req.UserId, limit, offset)
	} else {
		urls, total, err = s.store.ListPaginated(ctx, limit, offset)
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list URLs: %v", err)
	}

	pbURLs := make([]*pb.URL, len(urls))
	for i, url := range urls {
		var expiresAtUnix int64
		if url.ExpiresAt != nil {
			expiresAtUnix = url.ExpiresAt.Unix()
		}

		pbURLs[i] = &pb.URL{
			ShortCode: url.ShortCode,
			ShortUrl:  fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
			LongUrl:   url.LongURL,
			Clicks:    url.Clicks,
			CreatedAt: url.CreatedAt.Unix(),
			UpdatedAt: url.CreatedAt.Unix(),
			IsActive:  true,
			ExpiresAt: expiresAtUnix,
		}
	}

	hasMore := (offset + limit) < total

	return &pb.ListURLsResponse{
		Urls:    pbURLs,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

// DeleteURL handles the gRPC DeleteURL RPC. It removes the URL from
// PostgreSQL, Elasticsearch, and the Redis cache in that order. If the short
// code does not exist in PostgreSQL, Success=false is returned without a gRPC
// error. Secondary store deletions are best-effort -- their errors are
// intentionally ignored so a cache/search outage does not block the user.
func (s *URLService) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest) (*pb.DeleteURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	if err := s.store.Delete(ctx, req.ShortCode); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return &pb.DeleteURLResponse{Success: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "failed to delete URL: %v", err)
	}

	if s.esClient != nil {
		_ = s.esClient.DeleteURL(ctx, req.ShortCode)
	}

	cacheKey := "url:" + req.ShortCode
	_ = s.cache.Delete(ctx, cacheKey)

	return &pb.DeleteURLResponse{
		Success: true,
	}, nil
}

// IncrementClicks handles the gRPC IncrementClicks RPC. It atomically
// increments the click counter in PostgreSQL, then re-reads the URL to return
// the updated count. This two-step approach (UPDATE then SELECT) keeps the
// write path simple while giving the caller the freshest count.
func (s *URLService) IncrementClicks(ctx context.Context, req *pb.IncrementClicksRequest) (*pb.IncrementClicksResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	if err := s.store.IncrementClicks(ctx, req.ShortCode); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to increment clicks: %v", err)
	}

	url, err := s.store.GetByShortCode(ctx, req.ShortCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get URL: %v", err)
	}

	if url == nil {
		return nil, status.Error(codes.NotFound, "URL not found")
	}

	return &pb.IncrementClicksResponse{
		Clicks: url.Clicks,
	}, nil
}

// CreateCustomURL handles the gRPC CreateCustomURL RPC, allowing users to
// choose their own alias (e.g., "my-link") instead of accepting a random
// short code. Alias uniqueness is enforced by a two-layer strategy:
//
//  1. A Redis-based distributed lock prevents concurrent requests for the
//     same alias from racing.
//  2. A strongly-consistent read against the PostgreSQL primary (not a read
//     replica) confirms the alias is truly available before INSERT.
//
// If the alias is already taken, the response includes suggested alternatives
// generated by the validation package.
func (s *URLService) CreateCustomURL(ctx context.Context, req *pb.CreateCustomURLRequest) (*pb.CreateCustomURLResponse, error) {
	if req.Alias == "" {
		return nil, status.Error(codes.InvalidArgument, "alias is required")
	}

	if req.LongUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "long_url is required")
	}

	var expiresAt *time.Time
	if req.ExpiresAt > 0 {
		t := time.Unix(req.ExpiresAt, 0)
		expiresAt = &t
	} else if s.defaultTTL > 0 {
		t := time.Now().Add(s.defaultTTL)
		expiresAt = &t
	}

	result, err := s.createCustomURLInternal(ctx, req.Alias, req.LongUrl, expiresAt, req.UserId)
	if err != nil {
		if strings.Contains(err.Error(), "invalid alias") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if strings.Contains(err.Error(), "already taken") {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to create custom URL: %v", err)
	}

	var expiresAtUnix int64
	if expiresAt != nil {
		expiresAtUnix = expiresAt.Unix()
	}

	qrCode, _ := s.getQRCode(ctx, result.ShortCode)

	return &pb.CreateCustomURLResponse{
		ShortCode: result.ShortCode,
		ShortUrl:  result.ShortURL,
		LongUrl:   result.LongURL,
		CreatedAt: result.CreatedAt.Unix(),
		ExpiresAt: expiresAtUnix,
		QrCode:    qrCode,
	}, nil
}

// createCustomURLInternal contains the core logic for custom alias creation,
// separated from the gRPC handler so error classification (InvalidArgument vs.
// AlreadyExists vs. Internal) can be handled at the handler level. The method:
//
//  1. Validates the alias format (length, allowed characters).
//  2. Acquires a Redis distributed lock keyed to the alias with a 5-second TTL.
//  3. Asserts that the storage layer is PostgresStorage (custom aliases need
//     direct access to AliasExistsPrimary for strong consistency).
//  4. Checks alias availability on the primary database.
//  5. Persists the URL and warms the cache.
func (s *URLService) createCustomURLInternal(ctx context.Context, alias, longURL string, expiresAt *time.Time, userID string) (*CreateURLResult, error) {
	if err := validation.ValidateAlias(alias); err != nil {
		return nil, fmt.Errorf("invalid alias: %w", err)
	}

	lockKey := fmt.Sprintf("lock:alias:%s", alias)
	distributedLock := lock.NewDistributedLock(s.redisClient, lockKey, 5*time.Second)

	acquired, err := distributedLock.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("alias is being claimed by another request, please try again")
	}
	defer func() { _ = distributedLock.Release(ctx) }()

	postgresStore, ok := s.store.(*storage.PostgresStorage)
	if !ok {
		return nil, fmt.Errorf("storage layer doesn't support custom aliases")
	}

	exists, err := postgresStore.AliasExistsPrimary(ctx, alias)
	if err != nil {
		return nil, fmt.Errorf("failed to check availability: %w", err)
	}

	if exists {
		suggestions := validation.SuggestAlternatives(alias, 3)
		return nil, fmt.Errorf("alias '%s' is already taken. Try: %v", alias, suggestions)
	}

	shortURL := fmt.Sprintf("%s/%s", s.baseURL, alias)
	qrCodeData, err := qrcode.GenerateQRCode(shortURL)
	if err != nil {
		qrCodeData = ""
	}

	err = postgresStore.CreateCustomURL(ctx, alias, longURL, expiresAt, qrCodeData, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom URL: %w", err)
	}

	cacheKey := "url:" + alias
	_ = s.cache.Set(ctx, cacheKey, longURL)

	return &CreateURLResult{
		ShortCode: alias,
		ShortURL:  shortURL,
		LongURL:   longURL,
		CreatedAt: time.Now(),
	}, nil
}

// CreateURLResult is an internal value object returned by
// createCustomURLInternal. It bundles the fields needed to build the gRPC
// response without exposing protobuf types in the private method signature.
type CreateURLResult struct {
	ShortCode string
	ShortURL  string
	LongURL   string
	CreatedAt time.Time
}

// getQRCode retrieves the stored QR code for a given short code from the
// database. It is used after custom alias creation to attach the QR code to
// the response (the QR code is generated and stored during insertion).
func (s *URLService) getQRCode(ctx context.Context, shortCode string) (string, error) {
	url, err := s.store.GetByShortCode(ctx, shortCode)
	if err != nil || url == nil {
		return "", err
	}
	return url.QRCode, nil
}
