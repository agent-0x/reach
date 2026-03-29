package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- jsonResponse tests ---

func TestJsonResponse(t *testing.T) {
	w := httptest.NewRecorder()
	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"version": "1.0",
	})

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type 'application/json', got %q", ct)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body["ok"])
	}
	if body["version"] != "1.0" {
		t.Fatalf("expected version=1.0, got %v", body["version"])
	}
}

// --- jsonError tests ---

func TestJsonError(t *testing.T) {
	tests := []struct {
		name       string
		msg        string
		code       int
		expectCode int
	}{
		{"bad request", "missing field", http.StatusBadRequest, 400},
		{"unauthorized", "invalid token", http.StatusUnauthorized, 401},
		{"internal error", "something broke", http.StatusInternalServerError, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			jsonError(w, tt.msg, tt.code)

			resp := w.Result()
			if resp.StatusCode != tt.expectCode {
				t.Fatalf("expected status %d, got %d", tt.expectCode, resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("expected Content-Type 'application/json', got %q", ct)
			}

			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["error"] != tt.msg {
				t.Fatalf("expected error %q, got %q", tt.msg, body["error"])
			}
		})
	}
}

// --- authMiddleware tests ---

func TestAuthMiddleware_ValidToken(t *testing.T) {
	cfg := &AgentConfig{Token: "test-secret-token"}

	handlerCalled := false
	handler := func(c *AgentConfig, w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		jsonResponse(w, map[string]string{"status": "ok"})
	}

	mw := authMiddleware(cfg, handler)

	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	mw(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called with valid token")
	}
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
}

func TestAuthMiddleware_MissingAuth(t *testing.T) {
	falseVal := false
	cfg := &AgentConfig{
		Token: "test-secret-token",
		Security: SecurityConfig{
			AuthFailLog: &falseVal, // suppress log output in tests
		},
	}

	handlerCalled := false
	handler := func(c *AgentConfig, w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}

	mw := authMiddleware(cfg, handler)

	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	// No Authorization header
	w := httptest.NewRecorder()

	mw(w, req)

	if handlerCalled {
		t.Fatal("handler should NOT have been called without auth")
	}
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}

	var body map[string]string
	_ = json.NewDecoder(w.Result().Body).Decode(&body)
	if body["error"] == "" {
		t.Fatal("expected error message in response")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	falseVal := false
	cfg := &AgentConfig{
		Token: "correct-token",
		Security: SecurityConfig{
			AuthFailLog: &falseVal,
		},
	}

	handlerCalled := false
	handler := func(c *AgentConfig, w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}

	mw := authMiddleware(cfg, handler)

	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	mw(w, req)

	if handlerCalled {
		t.Fatal("handler should NOT have been called with wrong token")
	}
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}

	var body map[string]string
	_ = json.NewDecoder(w.Result().Body).Decode(&body)
	if body["error"] != "invalid token" {
		t.Fatalf("expected 'invalid token' error, got %q", body["error"])
	}
}

func TestAuthMiddleware_BasicAuthScheme(t *testing.T) {
	falseVal := false
	cfg := &AgentConfig{
		Token: "test-token",
		Security: SecurityConfig{
			AuthFailLog: &falseVal,
		},
	}

	handlerCalled := false
	handler := func(c *AgentConfig, w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}

	mw := authMiddleware(cfg, handler)

	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth, not Bearer
	w := httptest.NewRecorder()

	mw(w, req)

	if handlerCalled {
		t.Fatal("handler should NOT have been called with Basic auth")
	}
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}
}

func TestAuthMiddleware_EmptyBearer(t *testing.T) {
	falseVal := false
	cfg := &AgentConfig{
		Token: "test-token",
		Security: SecurityConfig{
			AuthFailLog: &falseVal,
		},
	}

	handlerCalled := false
	handler := func(c *AgentConfig, w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}

	mw := authMiddleware(cfg, handler)

	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()

	mw(w, req)

	if handlerCalled {
		t.Fatal("handler should NOT have been called with empty bearer token")
	}
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}
}

// --- boolDefault tests ---

func TestBoolDefault(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		p        *bool
		def      bool
		expected bool
	}{
		{"nil with default true", nil, true, true},
		{"nil with default false", nil, false, false},
		{"true pointer", &trueVal, false, true},
		{"false pointer", &falseVal, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boolDefault(tt.p, tt.def)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

// --- handleHealth tests ---

func TestHandleHealth(t *testing.T) {
	cfg := &AgentConfig{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(cfg, w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body["ok"])
	}
	if body["version"] != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %v", body["version"])
	}
}

// --- LoadConfig tests ---

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"

	content := `port: 7100
token: my-secret
tls_cert: /path/to/cert.pem
tls_key: /path/to/key.pem
max_output: 10485760
max_timeout: 600
security:
  command_blacklist: true
  auth_fail_log: true
`
	if err := WriteFile(path, content, ""); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 7100 {
		t.Fatalf("expected port 7100, got %d", cfg.Port)
	}
	if cfg.Token != "my-secret" {
		t.Fatalf("expected token 'my-secret', got %q", cfg.Token)
	}
	if cfg.MaxOutput != 10485760 {
		t.Fatalf("expected max_output 10485760, got %d", cfg.MaxOutput)
	}
	if cfg.MaxTimeout != 600 {
		t.Fatalf("expected max_timeout 600, got %d", cfg.MaxTimeout)
	}
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	_, err := LoadConfig("/tmp/nonexistent_reach_config_12345.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent config file")
	}
}
