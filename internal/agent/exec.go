package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// 内置危险命令黑名单 — 正则匹配
var builtinDangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/\s*$`),          // rm -rf /
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/\*`),            // rm -rf /*
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/(bin|boot|dev|etc|lib|proc|sbin|sys|usr|var)\b`), // rm system dirs
	regexp.MustCompile(`\bmkfs\b`),                                          // format filesystem
	regexp.MustCompile(`\bdd\b.*\bof=/dev/[sh]d`),                          // dd to disk
	regexp.MustCompile(`>\s*/dev/[sh]d`),                                    // write to disk device
	regexp.MustCompile(`:(){ :\|:& };:`),                                    // fork bomb
	regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*\s+)?777\s+/\s*$`),           // chmod 777 /
	regexp.MustCompile(`\bchown\s+(-[a-zA-Z]*\s+)?.*\s+/\s*$`),            // chown /
	regexp.MustCompile(`>\s*/etc/passwd`),                                   // overwrite passwd
	regexp.MustCompile(`>\s*/etc/shadow`),                                   // overwrite shadow
}

// BuildBlacklist 根据配置构建最终黑名单
func BuildBlacklist(cfg *SecurityConfig) []*regexp.Regexp {
	if cfg != nil && cfg.CommandBlacklist != nil && !*cfg.CommandBlacklist {
		return nil // 黑名单被显式关闭
	}
	patterns := make([]*regexp.Regexp, len(builtinDangerousPatterns))
	copy(patterns, builtinDangerousPatterns)
	if cfg != nil {
		for _, p := range cfg.CustomBlacklist {
			if r, err := regexp.Compile(p); err == nil {
				patterns = append(patterns, r)
			}
		}
	}
	return patterns
}

// CheckDangerous 检查命令是否在黑名单中
func CheckDangerous(command string, patterns []*regexp.Regexp) error {
	cmd := strings.TrimSpace(command)
	for _, p := range patterns {
		if p.MatchString(cmd) {
			return fmt.Errorf("blocked: command matches dangerous pattern (%s)", p.String())
		}
	}
	return nil
}

// ExecResult 命令执行结果
type ExecResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

// Execute 在独立进程组中执行 shell 命令，支持超时与输出截断
func Execute(command string, timeout int, maxOutput int, blacklist []*regexp.Regexp) *ExecResult {
	if err := CheckDangerous(command, blacklist); err != nil {
		return &ExecResult{Stderr: err.Error(), ExitCode: 1}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	result := &ExecResult{}

	// 处理 stdout
	stdoutBytes := stdoutBuf.Bytes()
	if len(stdoutBytes) > maxOutput {
		stdoutBytes = stdoutBytes[:maxOutput]
		result.Truncated = true
	}
	result.Stdout = string(stdoutBytes)

	// 处理 stderr
	stderrBytes := stderrBuf.Bytes()
	if len(stderrBytes) > maxOutput {
		stderrBytes = stderrBytes[:maxOutput]
		result.Truncated = true
	}
	result.Stderr = string(stderrBytes)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// 超时：杀整个进程组
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			result.ExitCode = 124
			result.Stderr += fmt.Sprintf("\nreach: command timed out")
			return result
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Stderr += "\n" + err.Error()
		}
	}

	return result
}
