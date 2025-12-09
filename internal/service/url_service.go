package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Varun5711/shorternit/internal/idgen"
	"github.com/Varun5711/shorternit/internal/models"
	"github.com/Varun5711/shorternit/internal/storage"
	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type URLService struct {
	pb.UnimplementedURLServiceServer
	store storage.Storage
	idGen *idgen.Generator
}

func NewURLService(store storage.Storage, idGen *idgen.Generator) *URLService {
	return &URLService{
		store: store,
		idGen: idGen,
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

	url := &models.URL{
		ID:        id,
		ShortCode: shortCode,
		LongURL:   req.LongUrl,
		Clicks:    0,
		CreatedAt: createdAt,
	}

	if err := s.store.Save(url); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save URL: %v", err)
	}

	return &pb.CreateURLResponse{
		ShortCode: shortCode,
		ShortUrl:  fmt.Sprintf("http://localhost:8080/%s", shortCode),
		LongUrl:   req.LongUrl,
		CreatedAt: createdAt.Unix(),
		Id:        id,
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
		Id:        url.ID,
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

	allURLs, err := s.store.List()
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
		pbURLs[i] = &pb.URL{
			Id:        url.ID,
			ShortCode: url.ShortCode,
			LongUrl:   url.LongURL,
			Clicks:    url.Clicks,
			CreatedAt: url.CreatedAt.Unix(),
			UpdatedAt: url.CreatedAt.Unix(),
			IsActive:  true,
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
