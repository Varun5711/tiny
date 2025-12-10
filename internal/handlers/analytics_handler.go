package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Varun5711/shorternit/internal/analytics"
	"github.com/Varun5711/shorternit/internal/logger"
)

type AnalyticsHandler struct {
	analyticsService *analytics.Service
	log              *logger.Logger
}

func NewAnalyticsHandler(service *analytics.Service) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: service,
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
