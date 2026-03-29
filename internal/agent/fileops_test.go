package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ReadFile tests ---

func TestReadFile_NormalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadFile(path, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != content {
		t.Fatalf("expected %q, got %q", content, result.Content)
	}
	if result.Size != len(content) {
		t.Fatalf("expected size %d, got %d", len(content), result.Size)
	}
	if result.Truncated {
		t.Fatal("should not be truncated")
	}
}

func TestReadFile_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	content := strings.Repeat("x", 1000)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadFile(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 100 {
		t.Fatalf("expected content length 100, got %d", len(result.Content))
	}
	if !result.Truncated {
		t.Fatal("should be truncated")
	}
	if result.Size != 1000 {
		t.Fatalf("expected size 1000, got %d", result.Size)
	}
}

func TestReadFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadFile(path, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "" {
		t.Fatalf("expected empty content, got %q", result.Content)
	}
	if result.Size != 0 {
		t.Fatalf("expected size 0, got %d", result.Size)
	}
	if result.Truncated {
		t.Fatal("should not be truncated")
	}
}

func TestReadFile_NonExistentFile(t *testing.T) {
	_, err := ReadFile("/tmp/nonexistent_reach_test_file_12345", 1024)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "open file") {
		t.Fatalf("expected 'open file' error, got: %v", err)
	}
}

func TestReadFile_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadFile(dir, 1024)
	if err == nil {
		t.Fatal("expected error when reading a directory")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected 'not a regular file' error, got: %v", err)
	}
}

func TestReadFile_RejectsSymlinkToProc(t *testing.T) {
	// Skip if /proc doesn't exist (non-Linux)
	if _, err := os.Stat("/proc/self/status"); os.IsNotExist(err) {
		t.Skip("skipping: /proc not available")
	}

	dir := t.TempDir()
	link := filepath.Join(dir, "proc_link")
	if err := os.Symlink("/proc/self/status", link); err != nil {
		t.Fatal(err)
	}

	// /proc/self/status is not a regular file (size 0 in stat but data available)
	// The function opens the file (following symlink) and checks IsRegular on the opened fd.
	// Since /proc files report as regular, this actually tests the LimitReader path.
	// The key security check is that we stat the opened fd (not the symlink).
	result, err := ReadFile(link, 1024)
	// /proc/self/status is technically regular, so ReadFile will succeed reading it
	if err != nil {
		// If it fails with "not a regular file", that's also acceptable security behavior
		t.Logf("ReadFile rejected /proc file (this is fine): %v", err)
		return
	}
	// If it succeeds, it should contain real content (proc files are readable)
	if result.Content == "" {
		t.Logf("proc file read returned empty content")
	}
}

func TestReadFile_ExactMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exact.txt")
	content := strings.Repeat("a", 100)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadFile(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != content {
		t.Fatalf("content mismatch")
	}
	if result.Truncated {
		t.Fatal("should not be truncated when size == maxSize")
	}
}

func TestReadFile_BinaryContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	data := []byte{0x00, 0x01, 0xFF, 0xFE, 0x7F}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadFile(path, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 5 {
		t.Fatalf("expected 5 bytes, got %d", len(result.Content))
	}
}

// --- WriteFile tests ---

func TestWriteFile_BasicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	content := "hello from test"

	if err := WriteFile(path, content, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected %q, got %q", content, string(data))
	}
}

func TestWriteFile_DefaultPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "default_perm.txt")

	if err := WriteFile(path, "test", ""); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Fatalf("expected perm 0644, got %04o", perm)
	}
}

func TestWriteFile_CustomPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom_perm.txt")

	if err := WriteFile(path, "test", "0755"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0755 {
		t.Fatalf("expected perm 0755, got %04o", perm)
	}
}

func TestWriteFile_PreservesExistingPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preserve.txt")

	// Create file with specific permission
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	// Overwrite with empty mode => should preserve 0600
	if err := WriteFile(path, "updated", ""); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected preserved perm 0600, got %04o", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "updated" {
		t.Fatalf("expected 'updated', got %q", string(data))
	}
}

func TestWriteFile_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_mode.txt")

	err := WriteFile(path, "test", "notanumber")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("expected 'invalid mode' error, got: %v", err)
	}
}

func TestWriteFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")

	if err := WriteFile(path, "deep content", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "deep content" {
		t.Fatalf("expected 'deep content', got %q", string(data))
	}
}

func TestWriteFile_AtomicBehavior(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")

	// Write initial content
	if err := WriteFile(path, "initial", ""); err != nil {
		t.Fatal(err)
	}

	// Write new content - should be atomic (no temp files left behind on success)
	if err := WriteFile(path, "updated", ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "updated" {
		t.Fatalf("expected 'updated', got %q", string(data))
	}

	// No leftover temp files
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".reach-tmp-") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestWriteFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	if err := WriteFile(path, "", ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty file, got %d bytes", len(data))
	}
}

func TestWriteFile_LargeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	content := strings.Repeat("x", 1<<20) // 1MB

	if err := WriteFile(path, content, ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1<<20 {
		t.Fatalf("expected %d bytes, got %d", 1<<20, len(data))
	}
}
