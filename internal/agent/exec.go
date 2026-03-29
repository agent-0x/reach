package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// ExecResult 命令执行结果
type ExecResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

// Execute 在独立进程组中执行 shell 命令，支持超时与输出截断
func Execute(command string, timeout int, maxOutput int) *ExecResult {
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
