package enrichment

import (
	"net"
)

type GeoInfo struct {
	Country     string
	CountryCode string
	Region      string
	City        string
	Latitude    float64
	Longitude   float64
	Timezone    string
}

type GeoIPEnricher struct {
}

func NewGeoIPEnricher() *GeoIPEnricher {
	return &GeoIPEnricher{}
}

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

func (g *GeoIPEnricher) Close() error {
	return nil
}
