package enrichment

import (
	"strings"

	"github.com/mssola/user_agent"
)

// UAInfo holds the parsed components of a User-Agent string.
// These fields are persisted alongside click events so the analytics
// service can aggregate traffic by browser, OS, and device category.
type UAInfo struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	// DeviceType is one of "desktop", "mobile", or "bot". The analytics
	// pipeline uses this for the device-breakdown chart.
	DeviceType  string
	DeviceBrand string
	DeviceModel string
	// IsTablet is reserved for future detection. Currently always false
	// because the underlying mssola/user_agent library does not expose a
	// reliable tablet signal separate from mobile.
	IsTablet bool
}

// ParseUserAgent decomposes a raw User-Agent header into structured UAInfo.
// It relies on the mssola/user_agent library for the heavy parsing and adds
// brand detection on top via keyword matching. The function always returns a
// non-nil result; unknown fields are left as empty strings.
func ParseUserAgent(uaString string) *UAInfo {
	ua := user_agent.New(uaString)

	browser, browserVersion := ua.Browser()

	// Default to "desktop"; override for bots and mobile. Bot detection
	// runs first so that mobile-mimicking crawlers are still classified
	// as bots (many bots send a mobile UA string).
	deviceType := "desktop"
	isTablet := false

	if ua.Bot() {
		deviceType = "bot"
	} else if ua.Mobile() {
		deviceType = "mobile"
	}

	platform := ua.Platform()
	model := ua.Model()

	brand := extractBrand(uaString, platform)

	// OS version extraction is currently limited to iOS/iPadOS because
	// Apple embeds the version inside the OS token (e.g., "iPhone OS 17_0").
	// Android and desktop OS versions are not reliably exposed in UA strings.
	osVersion := ""
	if strings.Contains(ua.OS(), "iPhone OS") || strings.Contains(ua.OS(), "CPU OS") {
		osVersion = extractVersion(ua.OS())
	}

	return &UAInfo{
		Browser:        browser,
		BrowserVersion: browserVersion,
		OS:             ua.OS(),
		OSVersion:      osVersion,
		DeviceType:     deviceType,
		DeviceBrand:    brand,
		DeviceModel:    model,
		IsTablet:       isTablet,
	}
}

// extractBrand infers the hardware manufacturer from keywords in the
// User-Agent string. A map-based lookup is used so new brands can be
// added without touching control flow. The platform fallback catches
// Apple devices that the keyword scan might miss.
func extractBrand(uaString, platform string) string {
	uaLower := strings.ToLower(uaString)

	brands := map[string]string{
		"iphone":    "Apple",
		"ipad":      "Apple",
		"macintosh": "Apple",
		"samsung":   "Samsung",
		"galaxy":    "Samsung",
		"pixel":     "Google",
		"nexus":     "Google",
		"huawei":    "Huawei",
		"xiaomi":    "Xiaomi",
		"oneplus":   "OnePlus",
		"oppo":      "Oppo",
		"vivo":      "Vivo",
		"nokia":     "Nokia",
		"lg":        "LG",
		"sony":      "Sony",
		"htc":       "HTC",
		"motorola":  "Motorola",
	}

	for keyword, brand := range brands {
		if strings.Contains(uaLower, keyword) {
			return brand
		}
	}

	// Fallback: check the platform string reported by the UA parser,
	// which sometimes contains "iPhone" or "iPad" even when the raw
	// UA string uses a different casing or format.
	if strings.Contains(strings.ToLower(platform), "iphone") ||
		strings.Contains(strings.ToLower(platform), "ipad") {
		return "Apple"
	}

	return ""
}

// extractVersion pulls a version-like token from an OS string by looking
// for parts containing underscores or dots (e.g., "17_0" -> "17.0").
// Apple uses underscores in UA version tokens; this normalizes them to dots.
func extractVersion(osString string) string {
	parts := strings.Split(osString, " ")
	for _, part := range parts {
		if strings.Contains(part, "_") || strings.Contains(part, ".") {
			return strings.ReplaceAll(part, "_", ".")
		}
	}
	return ""
}
