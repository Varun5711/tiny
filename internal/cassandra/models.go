package cassandra

import (
	"time"

	"github.com/gocql/gocql"
)

type Click struct {
	ShortCode string
	ClickedAt time.Time
	ClickID   gocql.UUID
	IPAddress string
	UserAgent string
	Referer   string
}

func (c *CassandraClient) InsertClick(click *Click) error {
	query := `
			INSERT INTO recent_clicks (
  			short_code, clicked_at, click_id, 
  			ip_address, user_agent, referer
  		) VALUES (?, ?, ?, ?, ?, ?) 
	`

	return c.session.Query(query,
		click.ShortCode,
		click.ClickedAt,
		gocql.TimeUUID(),
		click.IPAddress,
		click.UserAgent,
		click.Referer,
	).Exec()
}

func (c *CassandraClient) GetRecentClicks(shortCode string, limit int) ([]Click, error) {
	query := `
		SELECT short_code, clicked_at, click_id, ip_address, user_agent, referer
		FROM recent_clicks
		WHERE short_code = ?
		LIMIT ?
	`

	var clicks []Click
	iter := c.session.Query(query, shortCode, limit).Iter()

	var click Click
	for iter.Scan(
		&click.ShortCode,
		&click.ClickedAt,
		&click.ClickID,
		&click.IPAddress,
		&click.UserAgent,
		&click.Referer,
	) {
		clicks = append(clicks, click)
		click = Click{}
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return clicks, nil
}
