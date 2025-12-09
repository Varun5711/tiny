package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/Varun5711/shorternit/internal/events"
	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/url"
)

type RedirectHandler struct {
	grpcClient    pb.URLServiceClient
	clickProducer *events.ClickProducer
	log           *logger.Logger
}

func NewRedirectHandler(urlServiceAddr string, producer *events.ClickProducer) (*RedirectHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &RedirectHandler{
		grpcClient:    client,
		clickProducer: producer,
		log:           logger.New("redirect"),
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
		h.log.Error("Failed to get URL: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !grpcResp.Found || grpcResp.Url == nil {
		http.NotFound(w, r)
		return
	}

	clickEvent := &events.ClickEvent{
		ShortCode: shortCode,
		Timestamp: time.Now().Unix(),
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if err := h.clickProducer.Publish(ctx, clickEvent); err != nil {
		h.log.Warn("Failed to publish click event: %v", err)
	}

	http.Redirect(w, r, grpcResp.Url.LongUrl, http.StatusFound)
}
