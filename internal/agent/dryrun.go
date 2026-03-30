package agent

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// dryrunRequest POST /dryrun 请求体
type dryrunRequest struct {
	Command string `json:"command"`
}

// affectedInfo 受影响路径信息（仅 rm/mv 时填充）
type affectedInfo struct {
	Type   string  `json:"type"`   // "file" | "directory" | "unknown"
	Path   string  `json:"path"`
	Files  int     `json:"files"`
	SizeMB float64 `json:"size_mb"`
}

// dryrunResponse POST /dryrun 响应体
type dryrunResponse struct {
	Risk          string        `json:"risk"`           // "low" | "medium" | "high" | "blocked"
	Score         int           `json:"score"`          // 0-100
	Reasons       []string      `json:"reasons"`
	Affected      *affectedInfo `json:"affected,omitempty"`
	Suggestion    string        `json:"suggestion,omitempty"`
	WouldExecute  bool          `json:"would_execute"`
	Confidence    string        `json:"confidence"` // "high" | "medium" | "low"
}

// scoreResult 单条命令的评分结果
type scoreResult struct {
	score      int
	reasons    []string
	confidence string // "high" | "medium" | "low"
	affected   *affectedInfo
	suggestion string
}

// shellSplitRe 匹配 shell 操作符，用于拆分命令链
var shellSplitRe = regexp.MustCompile(`&&|\|\||[;|]`)

// shCWrapperRe 匹配 sh -c '...' 或 bash -c "..."
var shCWrapperRe = regexp.MustCompile(`(?i)^(?:sh|bash|dash|zsh)\s+-c\s+['"](.+)['"]$`)

// varExpansionRe 匹配变量展开 $VAR 或反引号
var varExpansionRe = regexp.MustCompile("(\\$[{(]?[A-Za-z_][A-Za-z0-9_]*[})]?|`[^`]*`)")

// globRe 匹配 glob 字符（排除引号内不作处理的情况，简单检测）
var globRe = regexp.MustCompile(`[*?]`)

// rmRfRe 匹配 rm -rf 或 rm -fr（任意 -[字母含 r 和 f] 的组合）
var rmRfRe = regexp.MustCompile(`\brm\s+-[a-zA-Z]*[rR][a-zA-Z]*[fF][a-zA-Z]*\b|\brm\s+-[a-zA-Z]*[fF][a-zA-Z]*[rR][a-zA-Z]*\b`)

// rmOnlyRe 匹配非递归 rm（有 -? 标志但不含 r/R）
var rmOnlyRe = regexp.MustCompile(`\brm\b`)

// 读操作命令集合（score=5）
var readOnlyCmds = map[string]bool{
	"ls": true, "cat": true, "less": true, "more": true, "head": true,
	"tail": true, "ps": true, "top": true, "htop": true, "df": true,
	"du": true, "find": true, "grep": true, "awk": true, "sed": true,
	"echo": true, "printf": true, "pwd": true, "whoami": true, "id": true,
	"uname": true, "date": true, "uptime": true, "free": true, "stat": true,
	"file": true, "which": true, "whereis": true, "type": true, "env": true,
	"printenv": true, "hostname": true, "ip": true, "ifconfig": true,
	"netstat": true, "ss": true, "lsof": true, "lsblk": true, "mount": true,
	"journalctl": true, "dmesg": true, "history": true, "wc": true,
	"sort": true, "uniq": true, "diff": true, "cmp": true, "md5sum": true,
	"sha256sum": true, "base64": true, "xxd": true, "strings": true,
	"ldd": true, "readelf": true, "objdump": true, "nm": true,
}

// firstToken 提取命令的第一个词（忽略 sudo、env 等前缀包装器）
func firstToken(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	// 跳过 sudo/env/nice/time 等包装器
	wrappers := []string{"sudo", "env", "nice", "ionice", "time", "nohup", "strace"}
	for {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			return ""
		}
		found := false
		for _, w := range wrappers {
			if parts[0] == w {
				// 跳过该词
				cmd = strings.TrimSpace(cmd[len(parts[0]):])
				found = true
				break
			}
		}
		if !found {
			return parts[0]
		}
	}
}

// statAffected 对 rm/mv 目标路径做 stat，填充 affectedInfo
// 最多遍历 maxFiles 个文件
func statAffected(path string) *affectedInfo {
	if path == "" {
		return nil
	}

	// 解析符号链接
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}

	info, err := os.Stat(resolved)
	if err != nil {
		// 路径不存在，仍返回基本信息
		return &affectedInfo{Type: "unknown", Path: path}
	}

	af := &affectedInfo{Path: path}

	if !info.IsDir() {
		af.Type = "file"
		af.Files = 1
		af.SizeMB = float64(info.Size()) / 1024 / 1024
		return af
	}

	// 目录：walk 统计文件数和总大小
	af.Type = "directory"
	const maxFiles = 10000
	var totalSize int64
	var fileCount int

	_ = filepath.Walk(resolved, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无权限目录
		}
		if !fi.IsDir() {
			fileCount++
			totalSize += fi.Size()
		}
		if fileCount >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})

	af.Files = fileCount
	af.SizeMB = float64(totalSize) / 1024 / 1024
	return af
}

// extractTargetPath 从 rm/mv 命令中提取目标路径（简单解析，取最后一个非 flag 参数）
func extractTargetPath(parts []string) string {
	if len(parts) < 2 {
		return ""
	}
	// 跳过 flags（以 - 开头的参数）
	for i := 1; i < len(parts); i++ {
		if !strings.HasPrefix(parts[i], "-") {
			return parts[i]
		}
	}
	return ""
}

// scoreCommand 对单条（已拆分的）命令评分，不检查黑名单
func scoreCommand(cmd string) scoreResult {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return scoreResult{score: 0, confidence: "high", reasons: []string{}}
	}

	res := scoreResult{confidence: "high", reasons: []string{}}

	// 检测变量展开 → confidence=low, min score=50
	if varExpansionRe.MatchString(cmd) {
		res.confidence = "low"
	}

	// 检测 glob → confidence=medium（仅在未被 low 降级时）
	if res.confidence != "low" && globRe.MatchString(cmd) {
		res.confidence = "medium"
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return res
	}

	verb := firstToken(cmd)

	// --- 打分规则 ---
	switch verb {
	case "rm":
		if rmRfRe.MatchString(cmd) {
			res.score = 85
			res.reasons = append(res.reasons, "Recursive force delete (rm -rf)")
			// 检测顶层目录
			targetParts := strings.Fields(cmd)
			target := extractTargetPath(targetParts)
			if target != "" {
				// 顶层目录判断：路径深度 <= 2（如 /opt/old）
				clean := filepath.Clean(target)
				depth := len(strings.Split(strings.TrimPrefix(clean, "/"), "/"))
				if clean != "/" && depth <= 2 {
					res.score = 88
					res.reasons = append(res.reasons, "Targets a top-level directory")
				}
				res.affected = statAffected(target)
				res.suggestion = "Consider listing contents first: ls -la " + target
			}
		} else if rmOnlyRe.MatchString(cmd) {
			res.score = 45
			res.reasons = append(res.reasons, "File deletion (rm)")
			target := extractTargetPath(parts)
			if target != "" {
				res.affected = statAffected(target)
			}
		}

	case "mv":
		res.score = 35
		res.reasons = append(res.reasons, "File move (mv)")
		// mv 的源路径是第一个非-flag参数
		target := extractTargetPath(parts)
		if target != "" {
			res.affected = statAffected(target)
		}

	case "systemctl":
		if len(parts) >= 2 {
			sub := parts[1]
			switch sub {
			case "restart", "stop":
				res.score = 55
				res.reasons = append(res.reasons, "Service "+sub+" (systemctl "+sub+")")
			case "start":
				res.score = 30
				res.reasons = append(res.reasons, "Service start (systemctl start)")
			default:
				res.score = 20
				res.reasons = append(res.reasons, "Systemctl command")
			}
		}

	case "chmod":
		res.score = 45
		res.reasons = append(res.reasons, "Permission change (chmod)")

	case "chown":
		res.score = 45
		res.reasons = append(res.reasons, "Ownership change (chown)")

	case "apt", "apt-get", "yum", "dnf", "pacman", "zypper":
		if len(parts) >= 2 {
			sub := parts[1]
			switch sub {
			case "install", "remove", "purge", "autoremove", "erase":
				res.score = 50
				res.reasons = append(res.reasons, "Package management ("+verb+" "+sub+")")
			default:
				res.score = 20
				res.reasons = append(res.reasons, "Package manager command")
			}
		}

	case "kill":
		res.score = 55
		res.reasons = append(res.reasons, "Process kill (kill)")

	case "pkill":
		res.score = 58
		res.reasons = append(res.reasons, "Process kill by name (pkill)")

	case "killall":
		res.score = 60
		res.reasons = append(res.reasons, "Kill all matching processes (killall)")

	case "dd":
		res.score = 60
		res.reasons = append(res.reasons, "Direct disk/device write (dd)")

	case "reboot":
		res.score = 70
		res.reasons = append(res.reasons, "System reboot (reboot)")

	case "shutdown", "poweroff", "halt":
		res.score = 70
		res.reasons = append(res.reasons, "System shutdown ("+verb+")")

	default:
		// 检测 > /etc/ 写操作
		etcWriteRe := regexp.MustCompile(`>\s*/etc/`)
		if etcWriteRe.MatchString(cmd) {
			res.score = 45
			res.reasons = append(res.reasons, "Write to /etc/ configuration directory")
			break
		}

		// 只读命令
		if readOnlyCmds[verb] {
			res.score = 5
			res.reasons = append(res.reasons, "Read-only command ("+verb+")")
		} else {
			res.score = 30
			res.reasons = append(res.reasons, "Unknown command ("+verb+")")
		}
	}

	// confidence=low 时强制 score >= 50
	if res.confidence == "low" && res.score < 50 {
		res.score = 50
	}

	return res
}

// splitShellChain 将命令按 shell 操作符拆分成多个子命令
func splitShellChain(cmd string) []string {
	parts := shellSplitRe.Split(cmd, -1)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{cmd}
	}
	return result
}

// unwrapShC 尝试从 sh -c '...' 中提取内部命令，失败则返回原命令
func unwrapShC(cmd string) string {
	m := shCWrapperRe.FindStringSubmatch(strings.TrimSpace(cmd))
	if len(m) >= 2 {
		return m[1]
	}
	return cmd
}

// scoreOverall 对整条命令（含链式）综合评分，返回最高分结果
func scoreOverall(cmd string) scoreResult {
	// 先尝试解包 sh -c / bash -c 包装
	unwrapped := unwrapShC(cmd)

	// 拆分命令链
	parts := splitShellChain(unwrapped)

	var best scoreResult
	for _, part := range parts {
		r := scoreCommand(part)
		if r.score > best.score {
			best = r
		} else {
			// 合并 reasons（去重）
			seen := map[string]bool{}
			for _, reason := range best.reasons {
				seen[reason] = true
			}
			for _, reason := range r.reasons {
				if !seen[reason] {
					best.reasons = append(best.reasons, reason)
					seen[reason] = true
				}
			}
		}
	}

	// 如果整条命令含有变量展开，整体 confidence 降级
	if varExpansionRe.MatchString(cmd) {
		best.confidence = "low"
		if best.score < 50 {
			best.score = 50
		}
	} else if globRe.MatchString(cmd) && best.confidence == "high" {
		best.confidence = "medium"
	}

	if best.confidence == "" {
		best.confidence = "high"
	}
	if best.reasons == nil {
		best.reasons = []string{}
	}

	return best
}

// scoreToRisk 将数值分数转换为风险等级
func scoreToRisk(score int) string {
	switch {
	case score >= 70:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

// handleDryRun 处理 POST /dryrun 请求
func handleDryRun(cfg *AgentConfig, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req dryrunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		jsonError(w, "command is required", http.StatusBadRequest)
		return
	}

	// 1. 先检查黑名单
	blacklist := BuildBlacklist(&cfg.Security)
	if err := CheckDangerous(req.Command, blacklist); err != nil {
		resp := dryrunResponse{
			Risk:         "blocked",
			Score:        100,
			Reasons:      []string{err.Error()},
			WouldExecute: false,
			Confidence:   "high",
		}
		jsonResponse(w, resp)
		return
	}

	// 2. 综合评分
	result := scoreOverall(req.Command)

	resp := dryrunResponse{
		Risk:         scoreToRisk(result.score),
		Score:        result.score,
		Reasons:      result.reasons,
		Affected:     result.affected,
		Suggestion:   result.suggestion,
		WouldExecute: true,
		Confidence:   result.confidence,
	}

	jsonResponse(w, resp)
}
