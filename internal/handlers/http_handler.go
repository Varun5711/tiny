package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/models"
	pb "github.com/Varun5711/shorternit/proto/url"
)

type HTTPHandler struct {
	grpcClient pb.URLServiceClient
	baseURL    string
}

func NewHTTPHandler(urlServiceAddr string, baseURL string) (*HTTPHandler, error) {
	client, err := grpcClient.NewURLServiceClient(urlServiceAddr)
	if err != nil {
		return nil, err
	}

	return &HTTPHandler{
		grpcClient: client,
		baseURL:    baseURL,
	}, nil
}

func (h *HTTPHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	var req models.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON")
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

	var expiresAt *time.Time
	if grpcResp.ExpiresAt > 0 {
		t := time.Unix(grpcResp.ExpiresAt, 0)
		expiresAt = &t
	}

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

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	errResp := models.ErrorResponse{
		Error:   "error",
		Message: message,
	}
	respondJSON(w, status, errResp)
}

func (h *HTTPHandler) CreateCustomURL(w http.ResponseWriter, r *http.Request) {
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

func isValidURL(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
