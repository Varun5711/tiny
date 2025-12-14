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

var (
	apiGatewayURL    = getEnv("API_GATEWAY_URL", "http://localhost:8080")
	redirectURL      = getEnv("REDIRECT_SERVICE_URL", "http://localhost:8081")
	testUserEmail    = fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	testUserPassword = "testPassword123"
	authToken        string
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("Skipping integration tests. Set INTEGRATION_TEST=true to run.")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHealthCheck(t *testing.T) {
	resp, err := http.Get(apiGatewayURL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResult)

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
	defer redirectResp.Body.Close()

	if redirectResp.StatusCode != http.StatusFound && redirectResp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected redirect status (301/302), got %d", redirectResp.StatusCode)
	}

	location := redirectResp.Header.Get("Location")
	if location != "https://httpbin.org/get" {
		t.Errorf("expected redirect to 'https://httpbin.org/get', got '%s'", location)
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, apiGatewayURL+"/api/urls", nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid URL, got %d", resp.StatusCode)
	}
}

func TestNotFoundShortCode(t *testing.T) {
	resp, err := http.Get(redirectURL + "/nonexistent-code-12345")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}
