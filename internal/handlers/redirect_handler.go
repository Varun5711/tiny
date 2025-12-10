package handlers

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/cassandra"
	"github.com/Varun5711/shorternit/internal/events"
	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/url"
)

type RedirectHandler struct {
	grpcClient    pb.URLServiceClient
	clickProducer *events.ClickProducer
	cache         *cache.Cache
	cassandra     *cassandra.CassandraClient
	log           *logger.Logger
}

func NewRedirectHandler(urlServiceAddr string, producer *events.ClickProducer, urlCache *cache.Cache, cassandraClient *cassandra.CassandraClient) (*RedirectHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &RedirectHandler{
		grpcClient:    client,
		clickProducer: producer,
		cache:         urlCache,
		cassandra:     cassandraClient,
		log:           logger.New("redirect"),
	}, nil
}

func (h *RedirectHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortCode := r.URL.Path[1:]
	if shortCode == "" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	var longURL string

	cacheKey := "url:" + shortCode
	cachedURL, found := h.cache.Get(ctx, cacheKey)

	if found {
		longURL = cachedURL
		h.log.Debug("Cache hit for %s", shortCode)
	} else {
		h.log.Debug("Cache miss for %s", shortCode)

		grpcReq := &pb.GetURLRequest{
			ShortCode: shortCode,
		}

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

		longURL = grpcResp.Url.LongUrl

		if err := h.cache.Set(ctx, cacheKey, longURL); err != nil {
			h.log.Warn("Failed to cache URL: %v", err)
		}
	}

	clientIP := getClientIP(r)

	h.log.Debug("Writing click to Cassandra for %s (IP: %s)", shortCode, clientIP)
	session := h.cassandra.GetSession()
	err := session.Query(`
		INSERT INTO recent_clicks (
			short_code, clicked_at, click_id,
			ip_address, user_agent, referer
		) VALUES (?, ?, now(), ?, ?, ?)
	`, shortCode, time.Now(), clientIP, r.UserAgent(), r.Referer()).Exec()

	if err != nil {
		h.log.Error("Failed to write to Cassandra: %v", err)
	} else {
		h.log.Debug("Successfully wrote click to Cassandra")
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

	http.Redirect(w, r, longURL, http.StatusFound)
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	if ip == "::1" {
		return "127.0.0.1"
	}

	return ip
}
