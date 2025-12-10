package enrichment

import (
	"github.com/mssola/user_agent"
)

type UAInfo struct {
	Browser    string
	OS         string
	DeviceType string
}

func ParseUserAgent(uaString string) *UAInfo {
	ua := user_agent.New(uaString)

	browser, _ := ua.Browser()
	deviceType := "desktop"

	if ua.Mobile() {
		deviceType = "mobile"
	} else if ua.Bot() {
		deviceType = "bot"
	}

	return &UAInfo{
		Browser:    browser,
		OS:         ua.OS(),
		DeviceType: deviceType,
	}
}
