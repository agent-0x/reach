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

// boundedWriter 有界写入器，超出 limit 的数据被丢弃
type boundedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // 丢弃但不报错，让进程继续
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *boundedWriter) Truncated() bool {
	return w.buf.Len() >= w.limit
}

// Execute 在独立进程组中执行 shell 命令，支持超时与输出截断
func Execute(command string, timeout int, maxOutput int, blacklist []*regexp.Regexp) *ExecResult {
	if err := CheckDangerous(command, blacklist); err != nil {
		return &ExecResult{Stderr: err.Error(), ExitCode: 1}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.Command("sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 有界缓冲: 运行时就限制内存，不是事后截断
	stdoutW := &boundedWriter{limit: maxOutput}
	stderrW := &boundedWriter{limit: maxOutput}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		return &ExecResult{Stderr: err.Error(), ExitCode: 1}
	}

	// 在 goroutine 里 Wait，主线程监控超时
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
		// 正常结束
	case <-ctx.Done():
		// 超时: 杀整个进程组
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done // 等 Wait 返回
		return &ExecResult{
			Stdout:    stdoutW.buf.String(),
			Stderr:    stderrW.buf.String() + "\nreach: command timed out",
			ExitCode:  124,
			Truncated: stdoutW.Truncated() || stderrW.Truncated(),
		}
	}

	result := &ExecResult{
		Stdout:    stdoutW.buf.String(),
		Stderr:    stderrW.buf.String(),
		Truncated: stdoutW.Truncated() || stderrW.Truncated(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Stderr += "\n" + err.Error()
		}
	}

	return result
}
