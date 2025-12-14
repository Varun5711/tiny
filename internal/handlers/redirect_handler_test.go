package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected '203.0.113.195', got '%s'", ip)
	}
}

func TestGetClientIP_XForwardedFor_Single(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195")

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected '203.0.113.195', got '%s'", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")

	ip := getClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got '%s'", ip)
	}
}

func TestGetClientIP_XForwardedFor_TakesPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195")
	req.Header.Set("X-Real-IP", "192.168.1.100")

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected X-Forwarded-For to take precedence, got '%s'", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.50:12345"

	ip := getClientIP(req)
	if ip != "192.168.1.50" {
		t.Errorf("expected '192.168.1.50', got '%s'", ip)
	}
}

func TestGetClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.50"

	ip := getClientIP(req)
	if ip != "192.168.1.50" {
		t.Errorf("expected '192.168.1.50', got '%s'", ip)
	}
}

func TestGetClientIP_IPv6Localhost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "[::1]:12345"

	ip := getClientIP(req)
	if ip != "127.0.0.1" {
		t.Errorf("expected '127.0.0.1' for IPv6 localhost, got '%s'", ip)
	}
}

func TestGetClientIP_XForwardedFor_WithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "  203.0.113.195  , 70.41.3.18")

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected trimmed '203.0.113.195', got '%s'", ip)
	}
}
