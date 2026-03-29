package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agent-0x/reach/internal/mcp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mcpCmd())
}

// mcpCmd — reach mcp
func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server management (Claude Code integration)",
	}
	cmd.AddCommand(mcpServeCmd())
	cmd.AddCommand(mcpInstallCmd())
	return cmd
}

// mcpServeCmd — reach mcp serve
func mcpServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP stdio server (used by Claude Code)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcp.Serve()
		},
	}
}

// mcpInstallCmd — reach mcp install [--global]
func mcpInstallCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install reach as an MCP server in Claude Code config",
		Long: `Write MCP server config so Claude Code can invoke 'reach mcp serve'.

By default writes .mcp.json in the current directory.
Use --global to write to ~/.claude/settings.json instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 找到 reach 二进制的绝对路径
			reachBin, err := reachBinaryPath()
			if err != nil {
				return fmt.Errorf("resolve reach binary: %w", err)
			}

			entry := map[string]any{
				"command": reachBin,
				"args":    []string{"mcp", "serve"},
			}

			if global {
				return installGlobal(entry)
			}
			return installLocal(entry)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Write to ~/.claude/settings.json instead of .mcp.json")
	return cmd
}

// reachBinaryPath 返回当前进程可执行文件的绝对路径。
func reachBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// 解析符号链接，确保是真实路径
	return filepath.EvalSymlinks(exe)
}

// installLocal 写入 .mcp.json（当前目录）
func installLocal(entry map[string]any) error {
	path := ".mcp.json"
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := writeMCPConfig(abs, entry); err != nil {
		return err
	}
	fmt.Printf("Reach MCP server installed to %s\n", abs)
	return nil
}

// installGlobal 写入 ~/.claude/settings.json
func installGlobal(entry map[string]any) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create ~/.claude: %w", err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := writeMCPConfig(path, entry); err != nil {
		return err
	}
	fmt.Printf("Reach MCP server installed to %s\n", path)
	return nil
}

// writeMCPConfig 读取已有的 JSON 配置（若存在），合并 mcpServers.reach，然后写回。
func writeMCPConfig(path string, entry map[string]any) error {
	// 读取已有内容
	var root map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse existing config %s: %w", path, err)
		}
	}
	if root == nil {
		root = make(map[string]any)
	}

	// 确保 mcpServers 字段存在
	mcpServers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	mcpServers["reach"] = entry
	root["mcpServers"] = mcpServers

	// 序列化（缩进格式，易读）
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
