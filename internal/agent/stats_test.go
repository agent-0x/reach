package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleStats_ReturnsExpectedShape 验证响应中所有顶级字段和关键子字段存在
func TestHandleStats_ReturnsExpectedShape(t *testing.T) {
	cfg := &AgentConfig{}
	body := bytes.NewBufferString(`{"top_n": 3}`)
	req := httptest.NewRequest(http.MethodPost, "/stats", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleStats(cfg, w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// 检查所有顶级键
	topLevelKeys := []string{
		"cpu", "memory", "disk", "network",
		"top_processes", "uptime_secs", "hostname",
		"os", "arch", "agent_version",
	}
	for _, key := range topLevelKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("missing top-level key: %q", key)
		}
	}

	// 检查 cpu 子字段
	cpuRaw, ok := result["cpu"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected cpu to be an object, got %T", result["cpu"])
	}
	for _, key := range []string{"count", "usage_percent"} {
		if _, ok := cpuRaw[key]; !ok {
			t.Errorf("missing cpu.%s", key)
		}
	}
	if count, ok := cpuRaw["count"].(float64); !ok || count <= 0 {
		t.Errorf("expected cpu.count > 0, got %v", cpuRaw["count"])
	}

	// 检查 memory 子字段
	memRaw, ok := result["memory"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected memory to be an object, got %T", result["memory"])
	}
	for _, key := range []string{"total_mb", "used_mb", "available_mb", "usage_percent"} {
		if _, ok := memRaw[key]; !ok {
			t.Errorf("missing memory.%s", key)
		}
	}
	if totalMB, ok := memRaw["total_mb"].(float64); !ok || totalMB <= 0 {
		t.Errorf("expected memory.total_mb > 0, got %v", memRaw["total_mb"])
	}

	// 检查 agent_version 匹配常量
	if result["agent_version"] != AgentVersion {
		t.Errorf("expected agent_version=%q, got %v", AgentVersion, result["agent_version"])
	}

	// 检查 top_processes 长度 <= 3
	procs, ok := result["top_processes"].([]interface{})
	if !ok {
		t.Fatalf("expected top_processes to be an array, got %T", result["top_processes"])
	}
	if len(procs) > 3 {
		t.Errorf("expected top_processes length <= 3, got %d", len(procs))
	}
}

// TestHandleStats_DefaultTopN 默认 top_n=5 时 top_processes 长度不超过 5
func TestHandleStats_DefaultTopN(t *testing.T) {
	cfg := &AgentConfig{}
	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/stats", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleStats(cfg, w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	procs, ok := result["top_processes"].([]interface{})
	if !ok {
		t.Fatalf("expected top_processes to be an array, got %T", result["top_processes"])
	}
	if len(procs) > 5 {
		t.Errorf("expected default top_processes length <= 5, got %d", len(procs))
	}
}

// TestHandleStats_MethodNotAllowed GET 请求应返回 405
func TestHandleStats_MethodNotAllowed(t *testing.T) {
	cfg := &AgentConfig{}
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()

	handleStats(cfg, w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected error message in 405 response")
	}
}
