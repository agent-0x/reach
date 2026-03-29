package agent

import (
	"regexp"
	"strings"
	"testing"
)

// --- boundedWriter tests ---

func TestBoundedWriter_UnderLimit(t *testing.T) {
	w := &boundedWriter{limit: 100}
	data := []byte("hello world")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected n=%d, got %d", len(data), n)
	}
	if w.buf.String() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", w.buf.String())
	}
	if w.Truncated() {
		t.Fatal("should not be truncated")
	}
}

func TestBoundedWriter_ExactLimit(t *testing.T) {
	w := &boundedWriter{limit: 5}
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected n=5, got %d", n)
	}
	if w.buf.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", w.buf.String())
	}
	if !w.Truncated() {
		t.Fatal("should be truncated at exact limit")
	}
}

func TestBoundedWriter_OverLimit(t *testing.T) {
	w := &boundedWriter{limit: 5}
	// Write more than the limit in a single call
	n, err := w.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should report full len(p) even though data was truncated
	if n != 11 {
		t.Fatalf("expected n=11, got %d", n)
	}
	if w.buf.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", w.buf.String())
	}
	if !w.Truncated() {
		t.Fatal("should be truncated")
	}
}

func TestBoundedWriter_MultipleWrites(t *testing.T) {
	w := &boundedWriter{limit: 10}
	_, _ = w.Write([]byte("hello"))
	_, _ = w.Write([]byte(" "))
	_, _ = w.Write([]byte("world"))
	_, _ = w.Write([]byte("extra"))

	if w.buf.String() != "hello worl" {
		t.Fatalf("expected 'hello worl', got %q", w.buf.String())
	}
	if !w.Truncated() {
		t.Fatal("should be truncated after exceeding limit")
	}
}

func TestBoundedWriter_ZeroLimit(t *testing.T) {
	w := &boundedWriter{limit: 0}
	n, err := w.Write([]byte("anything"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 8 {
		t.Fatalf("expected n=8, got %d", n)
	}
	if w.buf.Len() != 0 {
		t.Fatal("nothing should be written with limit=0")
	}
}

// --- CheckDangerous tests ---

func TestCheckDangerous_SafeCommands(t *testing.T) {
	patterns := BuildBlacklist(nil)
	safe := []string{
		"echo hello",
		"ls -la /tmp",
		"cat /etc/hostname",
		"rm /tmp/myfile.txt",
		"rm -f /tmp/somefile",
		"dd if=/dev/zero of=/tmp/test bs=1M count=1",
	}
	for _, cmd := range safe {
		if err := CheckDangerous(cmd, patterns); err != nil {
			t.Errorf("command %q should be safe, got: %v", cmd, err)
		}
	}
}

func TestCheckDangerous_DangerousCommands(t *testing.T) {
	patterns := BuildBlacklist(nil)
	dangerous := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -f /etc",
		"rm -rf /usr",
		"rm -rf /var",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		"> /dev/sda",
		"chmod 777 /",
		"> /etc/passwd",
		"> /etc/shadow",
	}
	for _, cmd := range dangerous {
		if err := CheckDangerous(cmd, patterns); err == nil {
			t.Errorf("command %q should be dangerous, but was allowed", cmd)
		}
	}
}

func TestCheckDangerous_NilPatterns(t *testing.T) {
	// With nil patterns (blacklist disabled), everything passes
	if err := CheckDangerous("rm -rf /", nil); err != nil {
		t.Fatalf("expected nil patterns to allow all commands, got: %v", err)
	}
}

func TestCheckDangerous_WhitespaceHandling(t *testing.T) {
	patterns := BuildBlacklist(nil)
	// Leading/trailing whitespace should be trimmed
	if err := CheckDangerous("  mkfs.ext4 /dev/sda1  ", patterns); err == nil {
		t.Fatal("whitespace-padded dangerous command should still be caught")
	}
}

// --- BuildBlacklist tests ---

func TestBuildBlacklist_Default(t *testing.T) {
	// nil config => use builtin patterns
	patterns := BuildBlacklist(nil)
	if len(patterns) != len(builtinDangerousPatterns) {
		t.Fatalf("expected %d patterns, got %d", len(builtinDangerousPatterns), len(patterns))
	}
}

func TestBuildBlacklist_DisabledExplicitly(t *testing.T) {
	falseVal := false
	cfg := &SecurityConfig{CommandBlacklist: &falseVal}
	patterns := BuildBlacklist(cfg)
	if patterns != nil {
		t.Fatal("expected nil patterns when blacklist is disabled")
	}
}

func TestBuildBlacklist_CustomPatterns(t *testing.T) {
	trueVal := true
	cfg := &SecurityConfig{
		CommandBlacklist: &trueVal,
		CustomBlacklist:  []string{`\bcurl\b`, `\bwget\b`},
	}
	patterns := BuildBlacklist(cfg)
	expected := len(builtinDangerousPatterns) + 2
	if len(patterns) != expected {
		t.Fatalf("expected %d patterns, got %d", expected, len(patterns))
	}
	// Custom patterns should work
	if err := CheckDangerous("curl http://evil.com", patterns); err == nil {
		t.Fatal("curl should be blocked by custom blacklist")
	}
	if err := CheckDangerous("wget http://evil.com", patterns); err == nil {
		t.Fatal("wget should be blocked by custom blacklist")
	}
}

func TestBuildBlacklist_InvalidCustomRegex(t *testing.T) {
	trueVal := true
	cfg := &SecurityConfig{
		CommandBlacklist: &trueVal,
		CustomBlacklist:  []string{`[invalid`}, // bad regex
	}
	patterns := BuildBlacklist(cfg)
	// Invalid regex should be silently skipped; only builtins remain
	if len(patterns) != len(builtinDangerousPatterns) {
		t.Fatalf("expected %d patterns (invalid skipped), got %d", len(builtinDangerousPatterns), len(patterns))
	}
}

// --- Execute tests ---

func TestExecute_BasicEcho(t *testing.T) {
	result := Execute("echo hello", 10, 1024, nil)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", result.ExitCode, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("expected 'hello', got %q", result.Stdout)
	}
}

func TestExecute_StderrCapture(t *testing.T) {
	result := Execute("echo err >&2", 10, 1024, nil)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if strings.TrimSpace(result.Stderr) != "err" {
		t.Fatalf("expected stderr 'err', got %q", result.Stderr)
	}
}

func TestExecute_NonZeroExit(t *testing.T) {
	result := Execute("exit 42", 10, 1024, nil)
	if result.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestExecute_Timeout(t *testing.T) {
	result := Execute("sleep 60", 1, 1024, nil)
	if result.ExitCode != 124 {
		t.Fatalf("expected exit code 124 (timeout), got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "timed out") {
		t.Fatalf("expected 'timed out' in stderr, got %q", result.Stderr)
	}
}

func TestExecute_BlockedCommand(t *testing.T) {
	patterns := []*regexp.Regexp{regexp.MustCompile(`\bforbidden\b`)}
	result := Execute("forbidden command", 10, 1024, patterns)
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "blocked") {
		t.Fatalf("expected 'blocked' in stderr, got %q", result.Stderr)
	}
}

func TestExecute_OutputTruncation(t *testing.T) {
	// Generate output larger than maxOutput
	result := Execute("yes | head -n 10000", 10, 100, nil)
	if result.ExitCode != 0 {
		// head may cause SIGPIPE which is fine; just check truncation
		// Some systems give exit code 141 for SIGPIPE, so accept both 0 and 141
		if result.ExitCode != 141 {
			t.Logf("note: exit code %d (may be SIGPIPE)", result.ExitCode)
		}
	}
	if !result.Truncated {
		t.Fatal("expected output to be truncated")
	}
	if len(result.Stdout) > 100 {
		t.Fatalf("stdout should be at most 100 bytes, got %d", len(result.Stdout))
	}
}

func TestExecute_LsCommand(t *testing.T) {
	result := Execute("ls /", 10, 4096, nil)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", result.ExitCode, result.Stderr)
	}
	// Root directory should contain well-known dirs
	if !strings.Contains(result.Stdout, "tmp") {
		t.Fatalf("expected 'tmp' in ls / output, got %q", result.Stdout)
	}
}

func TestExecute_MultilineOutput(t *testing.T) {
	result := Execute("printf 'line1\\nline2\\nline3'", 10, 4096, nil)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}
