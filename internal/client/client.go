package client

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// ReachClient 封装对 reach-agent 的 HTTPS 调用
type ReachClient struct {
	Name   string
	Config *ServerConfig
	http   *http.Client
}

// NewClient 创建一个新的 ReachClient
// - 若 cfg.Fingerprint 不为空，每次建立 TLS 连接时验证证书 sha256 指纹
// - 若为空（TOFU 首次连接），不验证（由 add 命令获取后保存）
func NewClient(name string, cfg *ServerConfig) *ReachClient {
	fp := cfg.Fingerprint // 闭包捕获

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if fp == "" {
				return nil // 首次连接，TOFU，不验证
			}
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificate received")
			}
			der := cs.PeerCertificates[0].Raw
			sum := sha256.Sum256(der)
			got := hex.EncodeToString(sum[:])
			if got != fp {
				return fmt.Errorf("TLS fingerprint mismatch: expected %s, got %s", fp, got)
			}
			return nil
		},
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	return &ReachClient{
		Name:   name,
		Config: cfg,
		http:   &http.Client{Transport: transport},
	}
}

// baseURL 返回服务器 base URL
func (c *ReachClient) baseURL() string {
	port := c.Config.Port
	if port == 0 {
		port = 7100
	}
	return fmt.Sprintf("https://%s:%d", c.Config.Host, port)
}

// do 发送 JSON 请求并将响应解析为 map；处理 HTTP 错误状态
func (c *ReachClient) do(method, path string, body interface{}) (map[string]interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL()+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.Config.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if errMsg, ok := result["error"].(string); ok {
			return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
		}
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return result, nil
}

// Exec 执行远程命令，POST /exec
func (c *ReachClient) Exec(command string, timeout int) (map[string]interface{}, error) {
	return c.do("POST", "/exec", map[string]interface{}{
		"command": command,
		"timeout": timeout,
	})
}

// ReadFile 读取远程文件，POST /read
func (c *ReachClient) ReadFile(path string) (map[string]interface{}, error) {
	return c.do("POST", "/read", map[string]interface{}{
		"path": path,
	})
}

// WriteFile 写入远程文件，POST /write
func (c *ReachClient) WriteFile(path, content string) error {
	_, err := c.do("POST", "/write", map[string]interface{}{
		"path":    path,
		"content": content,
	})
	return err
}

// Upload 上传本地文件到远程路径，POST /upload (multipart)
func (c *ReachClient) Upload(localPath, remotePath string) (int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("open local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if err := mw.WriteField("path", remotePath); err != nil {
		return 0, fmt.Errorf("write path field: %w", err)
	}

	fw, err := mw.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return 0, fmt.Errorf("create form file: %w", err)
	}
	written, err := io.Copy(fw, f)
	if err != nil {
		return 0, fmt.Errorf("copy file data: %w", err)
	}
	if err := mw.Close(); err != nil {
		return 0, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL()+"/upload", &buf)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.Config.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respData, &result); err != nil {
		return 0, fmt.Errorf("parse response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if errMsg, ok := result["error"].(string); ok {
			return 0, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
		}
		return 0, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return written, nil
}

// Download 从远程路径下载文件到本地，GET /download
func (c *ReachClient) Download(remotePath, localPath string) error {
	url := fmt.Sprintf("%s/download?path=%s", c.baseURL(), remotePath)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Config.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if json.Unmarshal(data, &result) == nil {
			if errMsg, ok := result["error"].(string); ok {
				return fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
			}
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// 确保本地父目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create local dir: %w", err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write local file: %w", err)
	}
	return nil
}

// Info 获取远程服务器系统信息，GET /info
func (c *ReachClient) Info() (map[string]interface{}, error) {
	return c.do("GET", "/info", nil)
}

// Health 检查远程服务器健康状态，GET /health
func (c *ReachClient) Health() (bool, error) {
	result, err := c.do("GET", "/health", nil)
	if err != nil {
		return false, err
	}
	ok, _ := result["ok"].(bool)
	return ok, nil
}

// Stats returns enhanced system monitoring data, POST /stats
func (c *ReachClient) Stats(topN int) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if topN > 0 {
		body["top_n"] = topN
	}
	return c.do("POST", "/stats", body)
}

// DryRun checks a command's risk without executing it, POST /dryrun
func (c *ReachClient) DryRun(command string) (map[string]interface{}, error) {
	return c.do("POST", "/dryrun", map[string]interface{}{
		"command": command,
	})
}

// GetFingerprint 建立 TLS 连接获取服务器证书的 sha256 指纹（用于 TOFU）
func (c *ReachClient) GetFingerprint() (string, error) {
	port := c.Config.Port
	if port == 0 {
		port = 7100
	}
	addr := fmt.Sprintf("%s:%d", c.Config.Host, port)

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return "", fmt.Errorf("TLS dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("no certificate received from server")
	}

	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}
