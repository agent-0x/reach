package client

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSaveRoundTrip tests that saving and loading a config preserves data
func TestLoadSaveRoundTrip(t *testing.T) {
	// Override HOME to use a temp directory
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := &Config{
		Servers: map[string]*ServerConfig{
			"prod": {
				Host:        "10.0.0.1",
				Port:        7100,
				Token:       "secret-token-abc",
				Fingerprint: "abc123def456",
			},
		},
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(dir, ".reach", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Verify file permissions
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected file perm 0600, got %04o", info.Mode().Perm())
	}

	// Verify directory permissions
	dirInfo, err := os.Stat(filepath.Join(dir, ".reach"))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("expected dir perm 0700, got %04o", dirInfo.Mode().Perm())
	}

	// Load it back
	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if len(loaded.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(loaded.Servers))
	}

	srv, ok := loaded.Servers["prod"]
	if !ok {
		t.Fatal("server 'prod' not found")
	}
	if srv.Host != "10.0.0.1" {
		t.Fatalf("expected host 10.0.0.1, got %q", srv.Host)
	}
	if srv.Port != 7100 {
		t.Fatalf("expected port 7100, got %d", srv.Port)
	}
	if srv.Token != "secret-token-abc" {
		t.Fatalf("expected token 'secret-token-abc', got %q", srv.Token)
	}
	if srv.Fingerprint != "abc123def456" {
		t.Fatalf("expected fingerprint 'abc123def456', got %q", srv.Fingerprint)
	}
}

// TestLoadConfig_NonExistent tests that loading a non-existent config returns empty config
func TestLoadConfig_NonExistent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Servers == nil {
		t.Fatal("expected non-nil Servers map")
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(cfg.Servers))
	}
}

// TestLoadConfig_MultipleServers tests config with multiple server entries
func TestLoadConfig_MultipleServers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := &Config{
		Servers: map[string]*ServerConfig{
			"prod": {
				Host:        "10.0.0.1",
				Port:        7100,
				Token:       "token-prod",
				Fingerprint: "fp-prod",
			},
			"staging": {
				Host:        "10.0.0.2",
				Port:        7101,
				Token:       "token-staging",
				Fingerprint: "fp-staging",
			},
			"dev": {
				Host:        "127.0.0.1",
				Port:        7102,
				Token:       "token-dev",
				Fingerprint: "fp-dev",
			},
		},
	}

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Servers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(loaded.Servers))
	}

	for name, expected := range cfg.Servers {
		got, ok := loaded.Servers[name]
		if !ok {
			t.Fatalf("server %q not found", name)
		}
		if got.Host != expected.Host {
			t.Errorf("server %q: expected host %q, got %q", name, expected.Host, got.Host)
		}
		if got.Port != expected.Port {
			t.Errorf("server %q: expected port %d, got %d", name, expected.Port, got.Port)
		}
		if got.Token != expected.Token {
			t.Errorf("server %q: expected token %q, got %q", name, expected.Token, got.Token)
		}
		if got.Fingerprint != expected.Fingerprint {
			t.Errorf("server %q: expected fingerprint %q, got %q", name, expected.Fingerprint, got.Fingerprint)
		}
	}
}

// TestLoadConfig_EmptyServersMap tests that nil servers map is initialized
func TestLoadConfig_EmptyServersMap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write a config with no servers key
	configDir := filepath.Join(dir, ".reach")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers == nil {
		t.Fatal("Servers map should be initialized even when absent from YAML")
	}
}

// TestConfigPath tests that ConfigPath returns the expected path
func TestConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	path := ConfigPath()
	expected := filepath.Join(dir, ".reach", "config.yaml")
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

// TestSaveConfig_Overwrite tests that saving overwrites existing config
func TestSaveConfig_Overwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg1 := &Config{
		Servers: map[string]*ServerConfig{
			"old": {Host: "1.1.1.1", Port: 7100, Token: "old-token"},
		},
	}
	if err := cfg1.Save(); err != nil {
		t.Fatal(err)
	}

	cfg2 := &Config{
		Servers: map[string]*ServerConfig{
			"new": {Host: "2.2.2.2", Port: 7200, Token: "new-token"},
		},
	}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := loaded.Servers["old"]; ok {
		t.Fatal("old server should be gone after overwrite")
	}
	if _, ok := loaded.Servers["new"]; !ok {
		t.Fatal("new server should exist after overwrite")
	}
}
