package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	grpcClient "github.com/Varun5711/shorternit/internal/grpc"
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

	grpcReq := &pb.CreateURLRequest{
		LongUrl: req.LongURL,
	}

	ctx := context.Background()
	grpcResp, err := h.grpcClient.CreateURL(ctx, grpcReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create URL")
		return
	}

	res := models.CreateURLResponse{
		ShortCode: grpcResp.ShortCode,
		ShortURL:  h.baseURL + "/" + grpcResp.ShortCode,
		LongURL:   grpcResp.LongUrl,
		CreatedAt: time.Unix(grpcResp.CreatedAt, 0),
	}

	respondJSON(w, http.StatusCreated, res)
}

func (h *HTTPHandler) ListURLs(w http.ResponseWriter, r *http.Request) {
	grpcReq := &pb.ListURLsRequest{
		Limit:  100,
		Offset: 0,
	}

	ctx := context.Background()
	grpcResp, err := h.grpcClient.ListURLs(ctx, grpcReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list URLs")
		return
	}

	urlsList := make([]models.URL, len(grpcResp.Urls))
	for i, pbURL := range grpcResp.Urls {
		urlsList[i] = models.URL{
			ID:        pbURL.Id,
			ShortCode: pbURL.ShortCode,
			LongURL:   pbURL.LongUrl,
			Clicks:    pbURL.Clicks,
			CreatedAt: time.Unix(pbURL.CreatedAt, 0),
		}
	}

	res := models.ListURLsResponse{
		URLs: urlsList,
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

func isValidURL(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
