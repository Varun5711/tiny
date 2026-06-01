// Package handlers implements HTTP request handlers for the tiny URL shortener
// API gateway. Each handler translates incoming HTTP requests into gRPC calls
// to the backend URL and user services, and returns JSON responses. The package
// follows a thin-handler pattern: validation and serialization happen here,
// while business logic lives in the gRPC services.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	es "github.com/Varun5711/shorternit/internal/elasticsearch"
	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/models"
	pb "github.com/Varun5711/shorternit/proto/url"
)

// HTTPHandler serves the core URL CRUD endpoints (create, list, search).
// It delegates all persistence to the URL gRPC service and uses Elasticsearch
// for full-text search when available.
type HTTPHandler struct {
	grpcClient pb.URLServiceClient
	esClient   *es.Client
	baseURL    string // baseURL is the public-facing prefix used to construct short URLs (e.g. "https://tiny.io").
}

// NewHTTPHandler creates an HTTPHandler by dialing the URL gRPC service at
// urlServiceAddr. The baseURL is prepended to short codes when building the
// full short URL returned to clients. esClient may be nil if Elasticsearch
// is not configured, in which case the search endpoint returns 503.
func NewHTTPHandler(urlServiceAddr string, baseURL string, esClient *es.Client) (*HTTPHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &HTTPHandler{
		grpcClient: client,
		esClient:   esClient,
		baseURL:    baseURL,
	}, nil
}

// CreateURL handles POST requests to shorten a new URL. It validates the
// request body, extracts the authenticated user ID from the context (set by
// the auth middleware), and delegates to the URL gRPC service. The response
// includes the generated short code, the fully qualified short URL, and an
// optional QR code image.
func (h *HTTPHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	// Cap the body at 1 MiB to prevent oversized payloads from consuming memory.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req models.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.LongURL == "" {
		respondError(w, http.StatusBadRequest, "long_url is required")
		return
	}

	// Reject non-HTTP(S) URLs early to avoid storing unusable destinations.
	if !isValidURL(req.LongURL) {
		respondError(w, http.StatusBadRequest, "invalid URL format")
		return
	}

	// The user ID is injected into the context by the auth middleware; an
	// empty string here means the request is unauthenticated (anonymous shortening).
	userID := middleware.GetUserID(r.Context())

	grpcReq := &pb.CreateURLRequest{
		LongUrl: req.LongURL,
		UserId:  userID,
	}

	if req.ExpiresAt != nil {
		grpcReq.ExpiresAt = req.ExpiresAt.Unix()
	}

	ctx := r.Context()
	grpcResp, err := h.grpcClient.CreateURL(ctx, grpcReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create URL")
		return
	}

	// Convert the Unix timestamp back to a time pointer; zero means no expiry.
	var expiresAt *time.Time
	if grpcResp.ExpiresAt > 0 {
		t := time.Unix(grpcResp.ExpiresAt, 0)
		expiresAt = &t
	}

	// Build the public-facing short URL by combining the configured base URL
	// with the generated short code (e.g. "https://tiny.io/abc123").
	shortURL := h.baseURL + "/" + grpcResp.ShortCode

	res := models.CreateURLResponse{
		ShortCode: grpcResp.ShortCode,
		ShortURL:  shortURL,
		LongURL:   grpcResp.LongUrl,
		CreatedAt: time.Unix(grpcResp.CreatedAt, 0),
		ExpiresAt: expiresAt,
		QRCode:    grpcResp.QrCode,
	}

	respondJSON(w, http.StatusCreated, res)
}

// ListURLs handles GET requests to retrieve all URLs owned by the authenticated
// user. Results are currently hard-capped at 100 items with no client-side
// pagination; the HasMore flag indicates whether additional records exist.
func (h *HTTPHandler) ListURLs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	grpcReq := &pb.ListURLsRequest{
		Limit:  100,
		Offset: 0,
		UserId: userID,
	}

	ctx := r.Context()
	grpcResp, err := h.grpcClient.ListURLs(ctx, grpcReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list URLs")
		return
	}

	// Map protobuf URL messages to the JSON response model, converting
	// Unix timestamps to Go time values for consistent JSON serialization.
	urlsList := make([]models.URL, len(grpcResp.Urls))
	for i, pbURL := range grpcResp.Urls {
		var expiresAt *time.Time
		if pbURL.ExpiresAt > 0 {
			t := time.Unix(pbURL.ExpiresAt, 0)
			expiresAt = &t
		}

		urlsList[i] = models.URL{
			ShortCode: pbURL.ShortCode,
			ShortURL:  pbURL.ShortUrl,
			LongURL:   pbURL.LongUrl,
			Clicks:    pbURL.Clicks,
			CreatedAt: time.Unix(pbURL.CreatedAt, 0),
			ExpiresAt: expiresAt,
		}
	}

	res := models.ListURLsResponse{
		URLs:    urlsList,
		Total:   grpcResp.Total,
		HasMore: grpcResp.HasMore,
	}

	respondJSON(w, http.StatusOK, res)
}

// respondJSON serializes data as JSON and writes it to the response with the
// given HTTP status code. It is the single exit point for all successful
// handler responses, ensuring a consistent Content-Type header.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// respondError writes a standardized JSON error envelope. All handler error
// paths use this so that API consumers see a uniform { "error", "message" }
// shape regardless of which endpoint failed.
func respondError(w http.ResponseWriter, status int, message string) {
	errResp := models.ErrorResponse{
		Error:   "error",
		Message: message,
	}
	respondJSON(w, status, errResp)
}

// CreateCustomURL handles POST requests to create a URL with a user-chosen
// vanity alias (e.g. "my-brand"). It performs the same validation as CreateURL
// and additionally requires a non-empty alias. The gRPC service enforces alias
// uniqueness, returning InvalidArgument or AlreadyExists errors that this
// handler maps to the appropriate HTTP status codes.
func (h *HTTPHandler) CreateCustomURL(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req models.CreateCustomURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Alias == "" {
		respondError(w, http.StatusBadRequest, "alias is required")
		return
	}

	if req.LongURL == "" {
		respondError(w, http.StatusBadRequest, "long_url is required")
		return
	}

	if !isValidURL(req.LongURL) {
		respondError(w, http.StatusBadRequest, "invalid URL format")
		return
	}

	userID := middleware.GetUserID(r.Context())

	grpcReq := &pb.CreateCustomURLRequest{
		Alias:   req.Alias,
		LongUrl: req.LongURL,
		UserId:  userID,
	}

	if req.ExpiresAt != nil {
		grpcReq.ExpiresAt = req.ExpiresAt.Unix()
	}

	ctx := r.Context()
	grpcResp, err := h.grpcClient.CreateCustomURL(ctx, grpcReq)
	if err != nil {
		// Map gRPC error codes to HTTP status codes. The gRPC service returns
		// structured errors, but we match on message substrings because the
		// Go gRPC client wraps them into generic status errors.
		if strings.Contains(err.Error(), "invalid alias") || strings.Contains(err.Error(), "InvalidArgument") {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "already taken") || strings.Contains(err.Error(), "AlreadyExists") {
			respondError(w, http.StatusConflict, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create custom URL")
		return
	}

	var expiresAt *time.Time
	if grpcResp.ExpiresAt > 0 {
		t := time.Unix(grpcResp.ExpiresAt, 0)
		expiresAt = &t
	}

	res := models.CreateCustomURLResponse{
		ShortCode: grpcResp.ShortCode,
		ShortURL:  grpcResp.ShortUrl,
		LongURL:   grpcResp.LongUrl,
		CreatedAt: time.Unix(grpcResp.CreatedAt, 0),
		ExpiresAt: expiresAt,
		QRCode:    grpcResp.QrCode,
	}

	respondJSON(w, http.StatusCreated, res)
}

// SearchURLs handles GET requests to perform full-text search across stored
// URLs via Elasticsearch. Query parameters:
//   - q      (required) - the search query string
//   - limit  (optional) - max results per page, default 20, max 100
//   - offset (optional) - pagination offset, default 0
//
// Returns 503 Service Unavailable if Elasticsearch was not configured at startup.
func (h *HTTPHandler) SearchURLs(w http.ResponseWriter, r *http.Request) {
	// Gracefully degrade when Elasticsearch is not wired up, rather than panicking.
	if h.esClient == nil {
		respondError(w, http.StatusServiceUnavailable, "search is not available")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		respondError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	result, err := h.esClient.SearchURLs(r.Context(), query, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "search failed")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// isValidURL checks that str is a well-formed HTTP or HTTPS URL with a host.
// It intentionally rejects other schemes (ftp, javascript, data, etc.) to
// prevent abuse of the shortener for non-web destinations.
func isValidURL(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
