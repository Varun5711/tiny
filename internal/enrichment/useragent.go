package enrichment

import (
	"strings"

	"github.com/mssola/user_agent"
)

type UAInfo struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string
	DeviceBrand    string
	DeviceModel    string
	IsTablet       bool
}

func ParseUserAgent(uaString string) *UAInfo {
	ua := user_agent.New(uaString)

	browser, browserVersion := ua.Browser()

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

	if strings.Contains(strings.ToLower(platform), "iphone") ||
		strings.Contains(strings.ToLower(platform), "ipad") {
		return "Apple"
	}

	return ""
}

func extractVersion(osString string) string {
	parts := strings.Split(osString, " ")
	for _, part := range parts {
		if strings.Contains(part, "_") || strings.Contains(part, ".") {
			return strings.ReplaceAll(part, "_", ".")
		}
	}
	return ""
}
