package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// handleInfo 返回系统信息
func handleInfo(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()

	hostInfo, err := host.Info()
	osName := ""
	archName := runtime.GOARCH
	var uptimeSecs uint64
	if err == nil && hostInfo != nil {
		osName = hostInfo.OS
		uptimeSecs = hostInfo.Uptime
		if hostInfo.KernelArch != "" {
			archName = hostInfo.KernelArch
		}
	}

	cpuInfos, err := cpu.Info()
	cpuCount := 0
	if err == nil {
		cpuCount = len(cpuInfos)
	}
	// 逻辑 CPU 数量（线程数）作为备选
	if cpuCount == 0 {
		if n, err := cpu.Counts(true); err == nil {
			cpuCount = n
		}
	}

	var memTotalMB, memFreeMB uint64
	if vmStat, err := mem.VirtualMemory(); err == nil {
		memTotalMB = vmStat.Total / 1024 / 1024
		memFreeMB = vmStat.Available / 1024 / 1024
	}

	var diskTotalGB, diskFreeGB uint64
	if diskStat, err := disk.Usage("/"); err == nil {
		diskTotalGB = diskStat.Total / 1024 / 1024 / 1024
		diskFreeGB = diskStat.Free / 1024 / 1024 / 1024
	}

	jsonResponse(w, map[string]interface{}{
		"hostname":      hostname,
		"os":            osName,
		"arch":          archName,
		"cpu_count":     cpuCount,
		"mem_total_mb":  memTotalMB,
		"mem_free_mb":   memFreeMB,
		"disk_total_gb": diskTotalGB,
		"disk_free_gb":  diskFreeGB,
		"uptime_secs":   uptimeSecs,
		"agent_version": "0.1.0",
	})
}

// execRequest POST /exec 请求体
type execRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// handleExec 执行 shell 命令
func handleExec(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		jsonError(w, "command is required", http.StatusBadRequest)
		return
	}

	// 设置默认超时，并限制最大值
	if req.Timeout <= 0 {
		req.Timeout = 30
	}
	if req.Timeout > cfg.MaxTimeout {
		req.Timeout = cfg.MaxTimeout
	}

	blacklist := BuildBlacklist(&cfg.Security)
	result := Execute(req.Command, req.Timeout, cfg.MaxOutput, blacklist)
	jsonResponse(w, result)
}

// readRequest POST /read 请求体
type readRequest struct {
	Path string `json:"path"`
}

// handleRead 读取文件内容
func handleRead(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req readRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	result, err := ReadFile(req.Path, cfg.MaxOutput)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, result)
}

// writeRequest POST /write 请求体
type writeRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
}

// handleWrite 写入文件内容
func handleWrite(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req writeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	if err := WriteFile(req.Path, req.Content, req.Mode); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{"ok": true})
}

// handleUpload multipart 文件上传
func handleUpload(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		jsonError(w, "parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	path := r.FormValue("path")
	if path == "" {
		jsonError(w, "path field is required", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "read file field: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "read upload data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := WriteFile(path, string(data), ""); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ok":   true,
		"size": len(data),
	})
}

// handleDownload 下载文件（流式）
func handleDownload(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		jsonError(w, "path query parameter is required", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, info.Name(), time.Time{}, f)
}
