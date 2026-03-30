package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// callDryRun 辅助函数：构造 POST /dryrun 请求并返回解析后的响应
func callDryRun(t *testing.T, cfg *AgentConfig, command string) dryrunResponse {
	t.Helper()
	body, _ := json.Marshal(dryrunRequest{Command: command})
	req := httptest.NewRequest(http.MethodPost, "/dryrun", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleDryRun(cfg, w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d for command %q; body: %s", w.Code, command, w.Body.String())
	}

	var resp dryrunResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// defaultCfg 返回用于测试的默认配置（启用黑名单）
func defaultCfg() *AgentConfig {
	return &AgentConfig{}
}

// --- 必需测试用例 ---

func TestDryRun_BlockedCommand(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm -rf /")
	if resp.Risk != "blocked" {
		t.Errorf("expected risk=blocked, got %q", resp.Risk)
	}
	if resp.Score != 100 {
		t.Errorf("expected score=100, got %d", resp.Score)
	}
	if resp.WouldExecute {
		t.Error("expected would_execute=false for blocked command")
	}
}

func TestDryRun_HighRiskRm(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm -rf /opt/old")
	if resp.Risk != "high" {
		t.Errorf("expected risk=high, got %q", resp.Risk)
	}
	if resp.Score < 70 {
		t.Errorf("expected score>=70, got %d", resp.Score)
	}
	if !resp.WouldExecute {
		t.Error("expected would_execute=true")
	}
}

func TestDryRun_LowRiskReadOnly(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "ls -la /opt")
	if resp.Risk != "low" {
		t.Errorf("expected risk=low, got %q", resp.Risk)
	}
	if resp.Score > 10 {
		t.Errorf("expected score<=10, got %d", resp.Score)
	}
}

func TestDryRun_MediumRiskRestart(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "systemctl restart nginx")
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
}

func TestDryRun_ChainedCommand(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "echo hello && rm -rf /opt/old")
	if resp.Risk != "high" {
		t.Errorf("expected risk=high for chained command, got %q", resp.Risk)
	}
	if resp.Score < 70 {
		t.Errorf("expected score>=70 (highest wins), got %d", resp.Score)
	}
}

func TestDryRun_ShCWrapper(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "sh -c 'rm -rf /opt/old'")
	if resp.Risk != "high" {
		t.Errorf("expected risk=high for sh -c wrapper, got %q", resp.Risk)
	}
}

func TestDryRun_VariableExpansion(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm -rf $DIR")
	if resp.Confidence != "low" {
		t.Errorf("expected confidence=low for variable expansion, got %q", resp.Confidence)
	}
	if resp.Score < 50 {
		t.Errorf("expected score>=50 for variable expansion, got %d", resp.Score)
	}
}

func TestDryRun_MethodNotAllowed(t *testing.T) {
	cfg := defaultCfg()
	req := httptest.NewRequest(http.MethodGet, "/dryrun", nil)
	w := httptest.NewRecorder()
	handleDryRun(cfg, w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestDryRun_EmptyCommand(t *testing.T) {
	cfg := defaultCfg()
	body, _ := json.Marshal(dryrunRequest{Command: ""})
	req := httptest.NewRequest(http.MethodPost, "/dryrun", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleDryRun(cfg, w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- 额外覆盖测试 ---

func TestDryRun_SystemctlStop(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "systemctl stop mysql")
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
	if resp.Score != 55 {
		t.Errorf("expected score=55, got %d", resp.Score)
	}
}

func TestDryRun_SystemctlStart(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "systemctl start redis")
	// score=30 → medium
	if resp.Score != 30 {
		t.Errorf("expected score=30, got %d", resp.Score)
	}
}

func TestDryRun_Chmod(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "chmod 755 /opt/app")
	if resp.Score != 45 {
		t.Errorf("expected score=45, got %d", resp.Score)
	}
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
}

func TestDryRun_Chown(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "chown www-data /opt/app")
	if resp.Score != 45 {
		t.Errorf("expected score=45, got %d", resp.Score)
	}
}

func TestDryRun_AptInstall(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "apt-get install nginx")
	if resp.Score != 50 {
		t.Errorf("expected score=50, got %d", resp.Score)
	}
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
}

func TestDryRun_Kill(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "kill 12345")
	if resp.Score != 55 {
		t.Errorf("expected score=55, got %d", resp.Score)
	}
}

func TestDryRun_Reboot(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "reboot")
	if resp.Score != 70 {
		t.Errorf("expected score=70, got %d", resp.Score)
	}
	if resp.Risk != "high" {
		t.Errorf("expected risk=high, got %q", resp.Risk)
	}
}

func TestDryRun_MvCommand(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "mv /opt/old /opt/backup")
	if resp.Score != 35 {
		t.Errorf("expected score=35, got %d", resp.Score)
	}
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
}

func TestDryRun_RmNonRecursive(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm /tmp/oldfile.txt")
	if resp.Score != 45 {
		t.Errorf("expected score=45, got %d", resp.Score)
	}
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium, got %q", resp.Risk)
	}
}

func TestDryRun_WouldExecuteTrue(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "cat /etc/hostname")
	if !resp.WouldExecute {
		t.Error("expected would_execute=true for safe command")
	}
}

func TestDryRun_ConfidenceHigh(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "ls /opt")
	if resp.Confidence != "high" {
		t.Errorf("expected confidence=high, got %q", resp.Confidence)
	}
}

func TestDryRun_GlobConfidenceMedium(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "ls /opt/*.log")
	if resp.Confidence != "medium" {
		t.Errorf("expected confidence=medium for glob, got %q", resp.Confidence)
	}
}

func TestDryRun_BlockedRmRfRoot(t *testing.T) {
	// rm -rf /* 也应该被黑名单拦截
	cfg := defaultCfg()
	body, _ := json.Marshal(dryrunRequest{Command: "rm -rf /*"})
	req := httptest.NewRequest(http.MethodPost, "/dryrun", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleDryRun(cfg, w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", w.Code)
	}
	var resp dryrunResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Risk != "blocked" {
		t.Errorf("expected risk=blocked for rm -rf /*, got %q", resp.Risk)
	}
}

func TestDryRun_InvalidJSON(t *testing.T) {
	cfg := defaultCfg()
	req := httptest.NewRequest(http.MethodPost, "/dryrun", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleDryRun(cfg, w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDryRun_SuggestionForRmRf(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm -rf /opt/old")
	if resp.Suggestion == "" {
		t.Error("expected non-empty suggestion for rm -rf")
	}
}

func TestDryRun_ReasonsNotEmpty(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "rm -rf /opt/old")
	if len(resp.Reasons) == 0 {
		t.Error("expected at least one reason")
	}
}

func TestDryRun_BashCWrapper(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), `bash -c 'systemctl restart nginx'`)
	if resp.Risk != "medium" {
		t.Errorf("expected risk=medium for bash -c wrapper, got %q", resp.Risk)
	}
}

func TestDryRun_PkillScore(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "pkill nginx")
	if resp.Score != 58 {
		t.Errorf("expected score=58, got %d", resp.Score)
	}
}

func TestDryRun_KillallScore(t *testing.T) {
	resp := callDryRun(t, defaultCfg(), "killall nginx")
	if resp.Score != 60 {
		t.Errorf("expected score=60, got %d", resp.Score)
	}
}

// TestDryRun_ScoreToRisk 测试评分到风险等级的映射
func TestScoreToRisk(t *testing.T) {
	tests := []struct {
		score    int
		expected string
	}{
		{100, "high"},
		{70, "high"},
		{69, "medium"},
		{30, "medium"},
		{29, "low"},
		{0, "low"},
	}
	for _, tt := range tests {
		got := scoreToRisk(tt.score)
		if got != tt.expected {
			t.Errorf("scoreToRisk(%d) = %q, want %q", tt.score, got, tt.expected)
		}
	}
}

// TestSplitShellChain 测试 shell 链拆分
func TestSplitShellChain(t *testing.T) {
	tests := []struct {
		cmd      string
		expected int // 期望拆分后的部分数
	}{
		{"echo hello", 1},
		{"echo hello && ls /tmp", 2},
		{"echo a || echo b", 2},
		{"echo a; echo b; echo c", 3},
		{"echo a | grep a", 2},
	}
	for _, tt := range tests {
		parts := splitShellChain(tt.cmd)
		if len(parts) != tt.expected {
			t.Errorf("splitShellChain(%q): expected %d parts, got %d: %v",
				tt.cmd, tt.expected, len(parts), parts)
		}
	}
}

// TestUnwrapShC 测试 sh -c 解包
func TestUnwrapShC(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sh -c 'rm -rf /opt'", "rm -rf /opt"},
		{`bash -c "ls /tmp"`, "ls /tmp"},
		{"ls /tmp", "ls /tmp"}, // 不是包装器，原样返回
	}
	for _, tt := range tests {
		got := unwrapShC(tt.input)
		if got != tt.expected {
			t.Errorf("unwrapShC(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
