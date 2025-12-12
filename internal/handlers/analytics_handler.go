package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Varun5711/shorternit/internal/analytics"
	"github.com/Varun5711/shorternit/internal/clickhouse"
	"github.com/Varun5711/shorternit/internal/logger"
)

type AnalyticsHandler struct {
	analyticsService *analytics.Service
	clickhouse       *clickhouse.Client
	log              *logger.Logger
}

func NewAnalyticsHandler(service *analytics.Service, ch *clickhouse.Client) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: service,
		clickhouse:       ch,
		log:              logger.New("analytics-handler"),
	}
}

func (h *AnalyticsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	shortCode := extractShortCode(r.URL.Path)
	if shortCode == "" {
		http.Error(w, "short_code required", http.StatusBadRequest)
		return
	}

	stats, err := h.analyticsService.GetURLStats(r.Context(), shortCode)
	if err != nil {
		h.log.Error("Failed to get stats: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondAnalyticsJSON(w, stats)
}

func (h *AnalyticsHandler) GetTimeline(w http.ResponseWriter, r *http.Request) {
	shortCode := extractShortCode(r.URL.Path)
	if shortCode == "" {
		http.Error(w, "short_code required", http.StatusBadRequest)
		return
	}

	days := 7
	if daysParam := r.URL.Query().Get("days"); daysParam != "" {
		if d, err := strconv.Atoi(daysParam); err == nil && d > 0 {
			days = d
		}
	}

	timeline, err := h.analyticsService.GetClickTimeline(r.Context(), shortCode, days)
	if err != nil {
		h.log.Error("Failed to get timeline: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondAnalyticsJSON(w, timeline)
}

func (h *AnalyticsHandler) GetGeoStats(w http.ResponseWriter, r *http.Request) {
	shortCode := extractShortCode(r.URL.Path)
	if shortCode == "" {
		http.Error(w, "short_code required", http.StatusBadRequest)
		return
	}

	geoStats, err := h.analyticsService.GetGeoStats(r.Context(), shortCode)
	if err != nil {
		h.log.Error("Failed to get geo stats: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondAnalyticsJSON(w, geoStats)
}

func (h *AnalyticsHandler) GetDeviceStats(w http.ResponseWriter, r *http.Request) {
	shortCode := extractShortCode(r.URL.Path)
	if shortCode == "" {
		http.Error(w, "short_code required", http.StatusBadRequest)
		return
	}

	deviceStats, err := h.analyticsService.GetDeviceStats(r.Context(), shortCode)
	if err != nil {
		h.log.Error("Failed to get device stats: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondAnalyticsJSON(w, deviceStats)
}

func (h *AnalyticsHandler) GetReferrers(w http.ResponseWriter, r *http.Request) {
	shortCode := extractShortCode(r.URL.Path)
	if shortCode == "" {
		http.Error(w, "short_code required", http.StatusBadRequest)
		return
	}

	limit := 10
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 {
			limit = l
		}
	}

	referrers, err := h.analyticsService.GetTopReferrers(r.Context(), shortCode, limit)
	if err != nil {
		h.log.Error("Failed to get referrers: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondAnalyticsJSON(w, referrers)
}

func extractShortCode(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func respondAnalyticsJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *AnalyticsHandler) GetClickEvents(w http.ResponseWriter, r *http.Request) {
	shortCode := r.URL.Query().Get("short_code")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	ctx := r.Context()
	var events []clickhouse.ClickEvent
	var err error

	if shortCode != "" {
		events, err = h.clickhouse.GetClickEvents(ctx, shortCode, limit)
	} else {
		events, err = h.clickhouse.GetAllClickEvents(ctx, limit)
	}

	if err != nil {
		h.log.Error("Failed to fetch click events: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	type ClickEventResponse struct {
		EventID        string `json:"event_id"`
		ShortCode      string `json:"short_code"`
		OriginalURL    string `json:"original_url"`
		ClickedAt      string `json:"clicked_at"`
		IPAddress      string `json:"ip_address"`
		Country        string `json:"country"`
		Region         string `json:"region"`
		City           string `json:"city"`
		Browser        string `json:"browser"`
		BrowserVersion string `json:"browser_version"`
		OS             string `json:"os"`
		OSVersion      string `json:"os_version"`
		DeviceType     string `json:"device_type"`
		Referer        string `json:"referer"`
	}

	response := make([]ClickEventResponse, len(events))
	for i, event := range events {
		response[i] = ClickEventResponse{
			EventID:        event.EventID,
			ShortCode:      event.ShortCode,
			OriginalURL:    event.OriginalURL,
			ClickedAt:      event.ClickedAt.Format("2006-01-02 15:04:05"),
			IPAddress:      event.IPAddress,
			Country:        event.Country,
			Region:         event.Region,
			City:           event.City,
			Browser:        event.Browser,
			BrowserVersion: event.BrowserVersion,
			OS:             event.OS,
			OSVersion:      event.OSVersion,
			DeviceType:     event.DeviceType,
			Referer:        event.Referer,
		}
	}

	respondAnalyticsJSON(w, map[string]interface{}{
		"clicks": response,
		"total":  len(response),
	})
}
