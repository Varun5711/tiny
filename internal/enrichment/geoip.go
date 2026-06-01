// Package enrichment provides click-event enrichment utilities that transform
// raw request metadata (IP addresses, User-Agent strings) into structured
// analytics dimensions such as geographic location and device information.
// These enriched fields are stored alongside each click event in the analytics
// pipeline so dashboards can aggregate by country, browser, device type, etc.
package enrichment

import (
	"net"
)

// GeoInfo holds the geographic attributes resolved from an IP address.
// Every field defaults to a safe sentinel ("Unknown" / "XX" / 0.0) so
// downstream consumers never have to handle nil or missing values.
type GeoInfo struct {
	Country     string
	CountryCode string
	Region      string
	City        string
	Latitude    float64
	Longitude   float64
	Timezone    string
}

// GeoIPEnricher resolves IP addresses to geographic locations.
//
// This is currently a stub implementation that returns placeholder values.
// The struct is intentionally left empty so that a future integration with
// MaxMind GeoLite2 (or a similar MMDB provider) only needs to add a
// *maxminddb.Reader field and swap the Lookup body -- the public API
// surface stays the same, so no callers need to change.
type GeoIPEnricher struct {
}

// NewGeoIPEnricher constructs a ready-to-use GeoIPEnricher.
// When a real GeoLite2 database is integrated, this constructor will accept
// a file path to the .mmdb file and return an error if it cannot be opened.
func NewGeoIPEnricher() *GeoIPEnricher {
	return &GeoIPEnricher{}
}

// Lookup resolves an IP address string into geographic information.
// It handles three cases defensively:
//  1. Unparseable IPs -- returns "Unknown" to avoid crashing on bad input.
//  2. Loopback / private IPs -- returns "Local" because geo-lookup is
//     meaningless for RFC-1918 addresses or 127.0.0.1.
//  3. All other IPs -- returns "Unknown" until a real GeoIP database
//     is wired in.
func (g *GeoIPEnricher) Lookup(ipAddress string) *GeoInfo {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return &GeoInfo{
			Country:     "Unknown",
			CountryCode: "XX",
			Region:      "Unknown",
			City:        "Unknown",
			Latitude:    0.0,
			Longitude:   0.0,
			Timezone:    "UTC",
		}
	}

	if ip.IsLoopback() || ip.IsPrivate() {
		return &GeoInfo{
			Country:     "Local",
			CountryCode: "XX",
			Region:      "Local",
			City:        "Local",
			Latitude:    0.0,
			Longitude:   0.0,
			Timezone:    "UTC",
		}
	}

	return &GeoInfo{
		Country:     "Unknown",
		CountryCode: "XX",
		Region:      "Unknown",
		City:        "Unknown",
		Latitude:    0.0,
		Longitude:   0.0,
		Timezone:    "UTC",
	}
}

// Close releases any resources held by the enricher (e.g., an open MMDB file
// handle). The stub implementation is a no-op but the method is defined now
// so callers can defer Close() from day one without future refactoring.
func (g *GeoIPEnricher) Close() error {
	return nil
}
