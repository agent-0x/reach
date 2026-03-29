package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentConfig 对应 config.yaml
type AgentConfig struct {
	Port       int    `yaml:"port"`
	Token      string `yaml:"token"`
	TLSCert    string `yaml:"tls_cert"`
	TLSKey     string `yaml:"tls_key"`
	MaxOutput  int    `yaml:"max_output"`
	MaxTimeout int    `yaml:"max_timeout"`
}

// LoadConfig 从 yaml 文件加载配置
func LoadConfig(path string) (*AgentConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg AgentConfig
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Serve 启动 HTTPS server
func Serve(cfg *AgentConfig) error {
	mux := http.NewServeMux()

	// /health — 无需认证
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handleHealth(cfg, w, r)
	})

	// 需要认证的路由
	for _, route := range []struct {
		path    string
		handler func(*AgentConfig, http.ResponseWriter, *http.Request)
	}{
		{"/info", handleInfo},
		{"/exec", handleExec},
		{"/read", handleRead},
		{"/write", handleWrite},
		{"/upload", handleUpload},
		{"/download", handleDownload},
	} {
		route := route // capture
		mux.HandleFunc(route.path, authMiddleware(cfg, route.handler))
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Printf("reach agent listening on https://0.0.0.0%s\n", addr)
	return srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
}

// authMiddleware 检查 Authorization: Bearer <token>
func authMiddleware(cfg *AgentConfig, next func(*AgentConfig, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(remoteIP); err == nil {
			remoteIP = host
		}

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			log.Printf("AUTH_FAIL from %s: missing token on %s", remoteIP, r.URL.Path)
			jsonError(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != cfg.Token {
			log.Printf("AUTH_FAIL from %s: invalid token on %s", remoteIP, r.URL.Path)
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next(cfg, w, r)
	}
}

// --- Handlers ---

func handleHealth(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"version": "0.1.0",
	})
}

// --- Helpers ---

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
