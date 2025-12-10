package enrichment

import (
	"net"
)

type GeoInfo struct {
	Country string
	City    string
}

type GeoIPEnricher struct {
}

func NewGeoIPEnricher() *GeoIPEnricher {
	return &GeoIPEnricher{}
}

func (g *GeoIPEnricher) Lookup(ipAddress string) *GeoInfo {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return &GeoInfo{Country: "XX", City: "Unknown"}
	}

	if ip.IsLoopback() || ip.IsPrivate() {
		return &GeoInfo{Country: "XX", City: "Local"}
	}

	return &GeoInfo{
		Country: "US",
		City:    "Unknown",
	}
}

func (g *GeoIPEnricher) Close() error {
	return nil
}
