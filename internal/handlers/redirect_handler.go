package handlers

import (
	"context"
	"log"
	"net/http"

	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	pb "github.com/Varun5711/shorternit/proto/url"
)

type RedirectHandler struct {
	grpcClient pb.URLServiceClient
}

func NewRedirectHandler(urlServiceAddr string) (*RedirectHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &RedirectHandler{
		grpcClient: client,
	}, nil
}

func (h *RedirectHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortCode := r.URL.Path[1:]
	if shortCode == "" {
		http.NotFound(w, r)
		return
	}

	grpcReq := &pb.GetURLRequest{
		ShortCode: shortCode,
	}

	ctx := context.Background()
	grpcResp, err := h.grpcClient.GetURL(ctx, grpcReq)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !grpcResp.Found || grpcResp.Url == nil {
		http.NotFound(w, r)
		return
	}

	incrementReq := &pb.IncrementClicksRequest{
		ShortCode: shortCode,
	}
	if _, err := h.grpcClient.IncrementClicks(ctx, incrementReq); err != nil {
		log.Printf("WARNING: Failed to increment clicks for %s: %v", shortCode, err)
	}

	http.Redirect(w, r, grpcResp.Url.LongUrl, http.StatusFound)
}
