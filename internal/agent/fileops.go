package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// ReadResult 文件读取结果
type ReadResult struct {
	Content   string `json:"content"`
	Size      int    `json:"size"`
	Truncated bool   `json:"truncated"`
}

// ReadFile 读取文件内容，最多 maxSize 字节
// 拒绝非常规文件（设备、管道、/proc 等），防止无限阻塞或内存爆炸
func ReadFile(path string, maxSize int) (*ReadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// 只允许常规文件
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}

	// 始终通过 LimitReader 读取，不信任 Stat().Size()
	limitedReader := io.LimitReader(f, int64(maxSize)+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	result := &ReadResult{Size: int(info.Size())}
	if len(data) > maxSize {
		result.Content = string(data[:maxSize])
		result.Truncated = true
	} else {
		result.Content = string(data)
	}

	return result, nil
}

// WriteFile 原子写入文件。mode 为空时保留原文件权限（若不存在则用 0644）。
func WriteFile(path string, content string, mode string) error {
	// 确定目标文件权限
	var perm os.FileMode = 0644

	if mode != "" {
		// 解析八进制字符串，如 "0644" 或 "644"
		modeVal, err := strconv.ParseUint(mode, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid mode %q: %w", mode, err)
		}
		perm = os.FileMode(modeVal)
	} else {
		// 若目标文件已存在，保留其权限
		if info, err := os.Stat(path); err == nil {
			perm = info.Mode().Perm()
		}
	}

	// 在同目录创建临时文件，保证 Rename 是原子操作（同一文件系统）
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".reach-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// 失败时清理临时文件
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// 设置权限
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	// 原子重命名
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename to target: %w", err)
	}

	// fsync 目录确保 rename 持久化
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}

	success = true
	return nil
}
