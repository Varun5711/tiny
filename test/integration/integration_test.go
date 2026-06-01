// Package integration contains end-to-end tests that exercise the Tiny URL
// shortener through its public HTTP API. These tests require a fully running
// stack (API gateway, auth service, URL service, redirect service, PostgreSQL,
// Redis) and are gated behind the INTEGRATION_TEST=true environment variable
// so they never run during normal `go test ./...` invocations.
//
// The tests are designed to run in order (Go runs tests within a package
// sequentially by default). Earlier tests (register, login) populate the
// package-level authToken variable that later tests depend on. Each test
// uses t.Skip if the token is missing rather than failing, so a CI failure
// in registration surfaces clearly without cascading noise.
//
// Service URLs default to localhost but can be overridden via environment
// variables (API_GATEWAY_URL, REDIRECT_SERVICE_URL) for Docker Compose or
// Kubernetes test environments.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// Package-level test configuration. The test user email includes a nanosecond
// timestamp to avoid collisions across repeated test runs against the same
// database.
var (
	apiGatewayURL    = getEnv("API_GATEWAY_URL", "http://localhost:8080")
	redirectURL      = getEnv("REDIRECT_SERVICE_URL", "http://localhost:8081")
	testUserEmail    = fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	testUserPassword = "testPassword123"
	authToken        string // populated by TestUserRegistration / TestUserLogin
)

// getEnv returns the environment variable value or a default. Used to make
// service URLs configurable for different deployment environments.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestMain is the test entry point. It exits immediately with a skip message
// when INTEGRATION_TEST is not set, preventing these slow, infra-dependent
// tests from running during unit-test sweeps.
func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("Skipping integration tests. Set INTEGRATION_TEST=true to run.")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestHealthCheck verifies the API gateway is reachable and returns 200.
// This is the first test to run and serves as a smoke test for the stack.
func TestHealthCheck(t *testing.T) {
	resp, err := http.Get(apiGatewayURL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestUserRegistration creates a new user account and captures the auth token
// for use by subsequent tests. Accepts both 200 and 201 because the API may
// return either depending on the implementation.
func TestUserRegistration(t *testing.T) {
	payload := map[string]string{
		"email":    testUserEmail,
		"password": testUserPassword,
		"name":     "Test User",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(apiGatewayURL+"/api/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("registration request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 201 or 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if token, ok := result["token"].(string); ok {
		authToken = token
	}
}

// TestUserLogin authenticates the previously registered user and updates
// the authToken. This test also runs after registration so there is always
// a fresh token for the URL operation tests that follow.
func TestUserLogin(t *testing.T) {
	payload := map[string]string{
		"email":    testUserEmail,
		"password": testUserPassword,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(apiGatewayURL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if token, ok := result["token"].(string); ok {
		authToken = token
	}

	if authToken == "" {
		t.Error("expected auth token in response")
	}
}

// TestCreateURL verifies that an authenticated user can shorten a URL
// and receives a short_code in the response.
func TestCreateURL(t *testing.T) {
	if authToken == "" {
		t.Skip("no auth token available")
	}

	payload := map[string]string{
		"long_url": "https://example.com/test-url-" + fmt.Sprint(time.Now().UnixNano()),
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, apiGatewayURL+"/api/urls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create URL request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 201 or 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := result["short_code"].(string); !ok {
		t.Error("expected short_code in response")
	}
}

// TestCreateCustomURL verifies that an authenticated user can create a
// short URL with a custom alias and that the returned short_code matches
// the requested alias exactly.
func TestCreateCustomURL(t *testing.T) {
	if authToken == "" {
		t.Skip("no auth token available")
	}

	customAlias := fmt.Sprintf("test-alias-%d", time.Now().UnixNano())
	payload := map[string]string{
		"alias":    customAlias,
		"long_url": "https://example.com/custom-url-test",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, apiGatewayURL+"/api/urls/custom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create custom URL request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 201 or 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if shortCode, ok := result["short_code"].(string); !ok || shortCode != customAlias {
		t.Errorf("expected short_code '%s', got '%v'", customAlias, result["short_code"])
	}
}

// TestListURLs verifies that the authenticated user can retrieve their
// list of short URLs and that the response contains a "urls" array.
func TestListURLs(t *testing.T) {
	if authToken == "" {
		t.Skip("no auth token available")
	}

	req, _ := http.NewRequest(http.MethodGet, apiGatewayURL+"/api/urls", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("list URLs request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := result["urls"].([]interface{}); !ok {
		t.Error("expected urls array in response")
	}
}

// TestRedirect performs the full redirect flow: creates a short URL, then
// hits the redirect service and asserts a 301/302 with the correct Location
// header. A custom HTTP client with redirect-following disabled is used so
// we can inspect the redirect response directly.
func TestRedirect(t *testing.T) {
	if authToken == "" {
		t.Skip("no auth token available")
	}

	payload := map[string]string{
		"long_url": "https://httpbin.org/get",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, apiGatewayURL+"/api/urls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create URL request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var createResult map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&createResult)

	shortCode, ok := createResult["short_code"].(string)
	if !ok {
		t.Fatal("no short_code in create response")
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	redirectResp, err := noRedirectClient.Get(redirectURL + "/" + shortCode)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	defer func() { _ = redirectResp.Body.Close() }()

	if redirectResp.StatusCode != http.StatusFound && redirectResp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected redirect status (301/302), got %d", redirectResp.StatusCode)
	}

	location := redirectResp.Header.Get("Location")
	if location != "https://httpbin.org/get" {
		t.Errorf("expected redirect to 'https://httpbin.org/get', got '%s'", location)
	}
}

// TestUnauthorizedAccess verifies that requests without an Authorization
// header are rejected with 401, ensuring the auth middleware is active.
func TestUnauthorizedAccess(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, apiGatewayURL+"/api/urls", nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestInvalidURL verifies that submitting a malformed URL (missing scheme
// and host) returns a 400 Bad Request, confirming server-side validation.
func TestInvalidURL(t *testing.T) {
	if authToken == "" {
		t.Skip("no auth token available")
	}

	payload := map[string]string{
		"long_url": "not-a-valid-url",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, apiGatewayURL+"/api/urls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid URL, got %d", resp.StatusCode)
	}
}

// TestNotFoundShortCode verifies that the redirect service returns 404
// for a nonexistent short code rather than a redirect or server error.
func TestNotFoundShortCode(t *testing.T) {
	resp, err := http.Get(redirectURL + "/nonexistent-code-12345")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}
