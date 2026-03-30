package agent

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// statsRequest POST /stats 请求体
type statsRequest struct {
	TopN int `json:"top_n"`
}

// cpuStats CPU 统计信息
type cpuStats struct {
	Count        int       `json:"count"`
	UsagePercent float64   `json:"usage_percent"`
	PerCore      []float64 `json:"per_core"`
	LoadAvg      []float64 `json:"load_avg"`
}

// memoryStats 内存统计信息
type memoryStats struct {
	TotalMB      uint64  `json:"total_mb"`
	UsedMB       uint64  `json:"used_mb"`
	AvailableMB  uint64  `json:"available_mb"`
	UsagePercent float64 `json:"usage_percent"`
	SwapTotalMB  uint64  `json:"swap_total_mb"`
	SwapUsedMB   uint64  `json:"swap_used_mb"`
}

// diskPartition 单个分区统计
type diskPartition struct {
	Mount        string  `json:"mount"`
	TotalGB      uint64  `json:"total_gb"`
	UsedGB       uint64  `json:"used_gb"`
	FreeGB       uint64  `json:"free_gb"`
	UsagePercent float64 `json:"usage_percent"`
}

// diskStats 磁盘统计信息
type diskStats struct {
	Partitions []diskPartition `json:"partitions"`
}

// networkStats 网络统计信息（全部接口汇总）
type networkStats struct {
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
}

// processInfo 进程信息（故意不包含命令行参数，防止凭据泄露）
type processInfo struct {
	PID          int32   `json:"pid"`
	Name         string  `json:"name"`
	CPUPercent   float64 `json:"cpu_percent"`
	MemoryMB     float64 `json:"memory_mb"`
}

// statsResponse /stats 响应体
type statsResponse struct {
	CPU          cpuStats      `json:"cpu"`
	Memory       memoryStats   `json:"memory"`
	Disk         diskStats     `json:"disk"`
	Network      networkStats  `json:"network"`
	TopProcesses []processInfo `json:"top_processes"`
	UptimeSecs   uint64        `json:"uptime_secs"`
	Hostname     string        `json:"hostname"`
	OS           string        `json:"os"`
	Arch         string        `json:"arch"`
	AgentVersion string        `json:"agent_version"`
}

// handleStats 返回结构化系统监控数据
func handleStats(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req statsRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// 默认 top_n = 5，最大 20
	topN := req.TopN
	if topN <= 0 {
		topN = 5
	}
	if topN > 20 {
		topN = 20
	}

	resp := statsResponse{
		AgentVersion: AgentVersion,
		Arch:         runtime.GOARCH,
	}

	// hostname
	if h, err := os.Hostname(); err == nil {
		resp.Hostname = h
	}

	// host info (OS, uptime)
	if hi, err := host.Info(); err == nil && hi != nil {
		resp.OS = hi.OS
		resp.UptimeSecs = hi.Uptime
		if hi.KernelArch != "" {
			resp.Arch = hi.KernelArch
		}
	}

	// CPU
	resp.CPU = buildCPUStats()

	// Memory
	resp.Memory = buildMemoryStats()

	// Disk
	resp.Disk = buildDiskStats()

	// Network
	resp.Network = buildNetworkStats()

	// Top processes
	resp.TopProcesses = buildTopProcesses(topN)

	jsonResponse(w, resp)
}

func buildCPUStats() cpuStats {
	s := cpuStats{
		PerCore: []float64{},
		LoadAvg: []float64{},
	}

	// CPU 核心数（逻辑线程数）
	if n, err := cpu.Counts(true); err == nil {
		s.Count = n
	}

	// 总体使用率（采样 200ms）
	if percents, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(percents) > 0 {
		s.UsagePercent = percents[0]
	}

	// 每核使用率
	if perCore, err := cpu.Percent(0, true); err == nil {
		s.PerCore = perCore
	}

	// 系统负载（仅 Linux/macOS 支持，Windows 会返回零值）
	if avg, err := load.Avg(); err == nil && avg != nil {
		s.LoadAvg = []float64{avg.Load1, avg.Load5, avg.Load15}
	}

	return s
}

func buildMemoryStats() memoryStats {
	s := memoryStats{}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.TotalMB = vm.Total / 1024 / 1024
		s.UsedMB = vm.Used / 1024 / 1024
		s.AvailableMB = vm.Available / 1024 / 1024
		s.UsagePercent = vm.UsedPercent
	}

	if sw, err := mem.SwapMemory(); err == nil {
		s.SwapTotalMB = sw.Total / 1024 / 1024
		s.SwapUsedMB = sw.Used / 1024 / 1024
	}

	return s
}

func buildDiskStats() diskStats {
	s := diskStats{Partitions: []diskPartition{}}

	parts, err := disk.Partitions(false)
	if err != nil {
		// 回退到只查 /
		if usage, err2 := disk.Usage("/"); err2 == nil {
			s.Partitions = append(s.Partitions, diskPartition{
				Mount:        "/",
				TotalGB:      usage.Total / 1024 / 1024 / 1024,
				UsedGB:       usage.Used / 1024 / 1024 / 1024,
				FreeGB:       usage.Free / 1024 / 1024 / 1024,
				UsagePercent: usage.UsedPercent,
			})
		}
		return s
	}

	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}
		s.Partitions = append(s.Partitions, diskPartition{
			Mount:        p.Mountpoint,
			TotalGB:      usage.Total / 1024 / 1024 / 1024,
			UsedGB:       usage.Used / 1024 / 1024 / 1024,
			FreeGB:       usage.Free / 1024 / 1024 / 1024,
			UsagePercent: usage.UsedPercent,
		})
	}

	return s
}

func buildNetworkStats() networkStats {
	s := networkStats{}

	counters, err := psnet.IOCounters(false) // false = 汇总所有接口
	if err != nil || len(counters) == 0 {
		return s
	}

	// false 模式下通常只有一个 "all" 条目
	for _, c := range counters {
		s.BytesSent += c.BytesSent
		s.BytesRecv += c.BytesRecv
		s.PacketsSent += c.PacketsSent
		s.PacketsRecv += c.PacketsRecv
	}

	return s
}

func buildTopProcesses(topN int) []processInfo {
	procs, err := process.Processes()
	if err != nil {
		return []processInfo{}
	}

	type entry struct {
		info processInfo
		cpu  float64
	}

	entries := make([]entry, 0, len(procs))
	for _, p := range procs {
		name, _ := p.Name()
		cpuPct, _ := p.CPUPercent()
		memInfo, _ := p.MemoryInfo()

		var memMB float64
		if memInfo != nil {
			memMB = float64(memInfo.RSS) / 1024 / 1024
		}

		entries = append(entries, entry{
			info: processInfo{
				PID:        p.Pid,
				Name:       name,
				CPUPercent: cpuPct,
				MemoryMB:   memMB,
			},
			cpu: cpuPct,
		})
	}

	// 按 CPU 使用率降序排序（简单插入排序，topN 通常 <= 20）
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].cpu > entries[j-1].cpu; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}

	if topN > len(entries) {
		topN = len(entries)
	}

	result := make([]processInfo, topN)
	for i := 0; i < topN; i++ {
		result[i] = entries[i].info
	}
	return result
}
