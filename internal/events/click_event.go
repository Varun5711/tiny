package events

type ClickEvent struct {
	ShortCode   string
	Timestamp   int64
	IP          string
	UserAgent   string
	OriginalURL string
	Referer     string
	QueryParams string
}
