package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
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

type URLService struct {
	pb.UnimplementedURLServiceServer
	store       storage.Storage
	idGen       *idgen.Generator
	cache       *cache.Cache
	redisClient *redis.Client
	baseURL     string
	defaultTTL  time.Duration
}

func NewURLService(store storage.Storage, idGen *idgen.Generator, urlCache *cache.Cache, redisClient *redis.Client, baseURL string, defaultTTL time.Duration) *URLService {
	return &URLService{
		store:       store,
		idGen:       idGen,
		cache:       urlCache,
		redisClient: redisClient,
		baseURL:     baseURL,
		defaultTTL:  defaultTTL,
	}
}

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

	if err := s.store.Save(url); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save URL: %v", err)
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

func (s *URLService) GetURL(ctx context.Context, req *pb.GetURLRequest) (*pb.GetURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	url, err := s.store.GetByShortCode(req.ShortCode)
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

	var allURLs []*models.URL
	var err error

	if req.UserId != "" {
		allURLs, err = s.store.ListByUserID(req.UserId)
	} else {
		allURLs, err = s.store.List()
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list URLs: %v", err)
	}

	total := int32(len(allURLs))
	start := int(offset)
	end := int(offset + limit)

	if start >= len(allURLs) {
		return &pb.ListURLsResponse{
			Urls:    []*pb.URL{},
			Total:   total,
			HasMore: false,
		}, nil
	}

	if end > len(allURLs) {
		end = len(allURLs)
	}

	urls := allURLs[start:end]
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

	hasMore := end < len(allURLs)

	return &pb.ListURLsResponse{
		Urls:    pbURLs,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *URLService) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest) (*pb.DeleteURLResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	url, err := s.store.GetByShortCode(req.ShortCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get URL: %v", err)
	}

	if url == nil {
		return &pb.DeleteURLResponse{
			Success: false,
		}, nil
	}

	cacheKey := "url:" + req.ShortCode
	_ = s.cache.Delete(ctx, cacheKey)

	return &pb.DeleteURLResponse{
		Success: true,
	}, nil
}

func (s *URLService) IncrementClicks(ctx context.Context, req *pb.IncrementClicksRequest) (*pb.IncrementClicksResponse, error) {
	if req.ShortCode == "" {
		return nil, status.Error(codes.InvalidArgument, "short_code is required")
	}

	if err := s.store.IncrementClicks(req.ShortCode); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to increment clicks: %v", err)
	}

	url, err := s.store.GetByShortCode(req.ShortCode)
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
	defer distributedLock.Release(ctx)

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

type CreateURLResult struct {
	ShortCode string
	ShortURL  string
	LongURL   string
	CreatedAt time.Time
}

func (s *URLService) getQRCode(ctx context.Context, shortCode string) (string, error) {
	url, err := s.store.GetByShortCode(shortCode)
	if err != nil || url == nil {
		return "", err
	}
	return url.QRCode, nil
}
