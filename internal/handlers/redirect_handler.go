package handlers

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/events"
	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	"github.com/Varun5711/shorternit/internal/logger"
	pb "github.com/Varun5711/shorternit/proto/url"
)

// RedirectHandler resolves short codes to their original URLs and issues HTTP
// 302 redirects. It uses a two-tier cache lookup strategy to minimize latency:
//
//  1. L1/L2 in-process + Redis cache (via the cache.Cache abstraction)
//  2. gRPC call to the URL service (authoritative source of truth)
//
// Every successful redirect asynchronously publishes a click event to Kafka
// for downstream analytics processing.
type RedirectHandler struct {
	grpcClient    pb.URLServiceClient
	clickProducer *events.ClickProducer
	cache         *cache.Cache
	log           *logger.Logger
}

// NewRedirectHandler creates a RedirectHandler by dialing the URL gRPC service.
// The producer is used to publish click events to Kafka, and urlCache provides
// the multi-level cache for short code resolution. Both may be nil in
// degraded-mode configurations, though analytics and caching will be skipped.
func NewRedirectHandler(urlServiceAddr string, producer *events.ClickProducer, urlCache *cache.Cache) (*RedirectHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &RedirectHandler{
		grpcClient:    client,
		clickProducer: producer,
		cache:         urlCache,
		log:           logger.New("redirect"),
	}, nil
}

// HandleRedirect is the hot path of the entire application. It extracts the
// short code from the URL path, resolves the destination via a cache-then-gRPC
// cascade, publishes a click event for analytics, and issues a 302 redirect.
//
// Resolution order:
//  1. Check the multi-level cache (in-process LRU backed by Redis).
//  2. On cache miss, fall through to the URL gRPC service (backed by PostgreSQL).
//  3. On gRPC success, populate the cache so subsequent hits are fast.
//
// The click event is published asynchronously; a Kafka failure does not block
// the redirect, because user-perceived latency is more important than perfect
// analytics delivery (events can be recovered from access logs if needed).
func (h *RedirectHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	// Strip the leading "/" to get the raw short code.
	shortCode := r.URL.Path[1:]
	if shortCode == "" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	var longURL string

	// --- Cache lookup (L1 in-process + L2 Redis) ---
	cacheKey := "url:" + shortCode
	cachedURL, found := h.cache.Get(ctx, cacheKey)

	if found {
		longURL = cachedURL
		h.log.Debug("Cache hit for %s", shortCode)
	} else {
		// --- gRPC fallback (authoritative store) ---
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

		// Back-fill the cache so subsequent redirects for this code are fast.
		if err := h.cache.Set(ctx, cacheKey, longURL); err != nil {
			h.log.Warn("Failed to cache URL: %v", err)
		}
	}

	// --- Publish click event for analytics ---
	// This is fire-and-forget: we log a warning on failure but never block
	// the redirect response, prioritizing end-user latency.
	clickEvent := &events.ClickEvent{
		ShortCode:   shortCode,
		Timestamp:   time.Now().Unix(),
		IP:          getClientIP(r),
		UserAgent:   r.UserAgent(),
		OriginalURL: longURL,
		Referer:     r.Header.Get("Referer"),
		QueryParams: r.URL.RawQuery,
	}
	if err := h.clickProducer.Publish(ctx, clickEvent); err != nil {
		h.log.Warn("Failed to publish click event: %v", err)
	}

	http.Redirect(w, r, longURL, http.StatusFound)
}

// getClientIP extracts the real client IP address from the request, respecting
// reverse-proxy headers in priority order: X-Forwarded-For (first entry),
// X-Real-IP, then RemoteAddr as a last resort. IPv6 loopback (::1) is
// normalized to 127.0.0.1 for consistent analytics storage.
func getClientIP(r *http.Request) string {
	// X-Forwarded-For may contain a comma-separated chain of proxies;
	// the first entry is the original client IP.
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

	// Normalize IPv6 loopback to IPv4 so downstream GeoIP lookups and
	// analytics grouping treat local requests consistently.
	if ip == "::1" {
		return "127.0.0.1"
	}

	return ip
}
