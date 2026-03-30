package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-0x/reach/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const instructions = `You have access to remote servers via Reach — an AI-native alternative to SSH.

## Golden Rules

1. **reach_dryrun BEFORE destructive commands.** Always call reach_dryrun before reach_bash when the command modifies state (rm, mv, systemctl restart, apt install, kill, etc.). If risk is "high" or "blocked", warn the user and suggest alternatives.

2. **reach_stats instead of shell parsing.** Never run "top", "free -m", "df -h" via reach_bash and parse text. Use reach_stats — it returns structured JSON with CPU %, memory, disk, network, top processes. AI-native, not human-native.

3. **reach_read/reach_write instead of cat/echo.** For file operations, prefer reach_read and reach_write (atomic writes, no shell escaping issues) over reach_bash with cat/echo/sed.

4. **reach_list to discover servers.** If you're unsure which servers are available, call reach_list first.

5. **One server at a time.** Each tool call targets one server by name. If you need to run the same command on multiple servers, call reach_bash for each.

## Tool Selection Guide

| Need | Tool | NOT this |
|------|------|----------|
| System stats (CPU, memory, disk) | reach_stats | reach_bash + "top" / "free" |
| Check if command is safe | reach_dryrun | Just running it and hoping |
| Read a file | reach_read | reach_bash + "cat" |
| Write a file | reach_write | reach_bash + "echo > " |
| Upload a file | reach_upload | reach_bash + "base64 decode" |
| Run a command | reach_bash | — |
| System info (hostname, OS) | reach_info | reach_bash + "uname" |

## Behavioral Notes

- reach_write is atomic (temp file → fsync → rename). It never leaves partial writes.
- reach_bash commands run in isolated process groups with timeouts. Stuck commands are killed.
- All connections are HTTPS with certificate pinning. No plaintext.
- The server has a command blacklist (rm -rf /, mkfs, dd to disk, fork bombs). Blocked commands return an error, not a silent failure.`

// Serve 启动 MCP stdio server，阻塞直到 stdin 关闭。
func Serve() error {
	s := server.NewMCPServer("reach", "0.2.0",
		server.WithInstructions(instructions),
	)

	s.AddTool(toolReachBash(), handleReachBash)
	s.AddTool(toolReachRead(), handleReachRead)
	s.AddTool(toolReachWrite(), handleReachWrite)
	s.AddTool(toolReachInfo(), handleReachInfo)
	s.AddTool(toolReachList(), handleReachList)
	s.AddTool(toolReachUpload(), handleReachUpload)
	s.AddTool(toolReachStats(), handleReachStats)
	s.AddTool(toolReachDryRun(), handleReachDryRun)

	return server.ServeStdio(s)
}

// ── tool 定义 ───────────────────────────────────────────────────────────────

func toolReachBash() mcp.Tool {
	return mcp.NewTool("reach_bash",
		mcp.WithDescription("Execute a shell command on a remote server via reach"),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server (as configured in ~/.reach/config.yaml)"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Shell command to execute"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds (default 30)"),
		),
	)
}

func toolReachRead() mcp.Tool {
	return mcp.NewTool("reach_read",
		mcp.WithDescription("Read a file from a remote server"),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Absolute path of the file to read"),
		),
	)
}

func toolReachWrite() mcp.Tool {
	return mcp.NewTool("reach_write",
		mcp.WithDescription("Write content to a file on a remote server (atomic)"),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Absolute path of the file to write"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write"),
		),
	)
}

func toolReachInfo() mcp.Tool {
	return mcp.NewTool("reach_info",
		mcp.WithDescription("Get system information from a remote server"),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
	)
}

func toolReachList() mcp.Tool {
	return mcp.NewTool("reach_list",
		mcp.WithDescription("List all servers configured in ~/.reach/config.yaml"),
	)
}

func toolReachUpload() mcp.Tool {
	return mcp.NewTool("reach_upload",
		mcp.WithDescription("Upload a local file to a remote server"),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
		mcp.WithString("local_path",
			mcp.Required(),
			mcp.Description("Absolute local path of the file to upload"),
		),
		mcp.WithString("remote_path",
			mcp.Required(),
			mcp.Description("Absolute remote path where the file should be placed"),
		),
	)
}

func toolReachStats() mcp.Tool {
	return mcp.NewTool("reach_stats",
		mcp.WithDescription("Get detailed system stats from a remote server: CPU usage %, memory, disk, network IO, top processes. Returns structured JSON — no shell parsing needed."),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
		mcp.WithNumber("top_n",
			mcp.Description("Number of top processes to return (default 5, max 20)"),
		),
	)
}

func toolReachDryRun() mcp.Tool {
	return mcp.NewTool("reach_dryrun",
		mcp.WithDescription("Check if a command is dangerous before executing it. Returns risk level (low/medium/high/blocked), score 0-100, and reasons. Use this before reach_bash for destructive commands."),
		mcp.WithString("server",
			mcp.Required(),
			mcp.Description("Name of the remote server"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Shell command to analyze (not executed)"),
		),
	)
}

// ── 辅助 ─────────────────────────────────────────────────────────────────────

// getClientFromArgs 从 MCP 请求参数中提取 server 并创建 ReachClient。
func getClientFromArgs(args map[string]any) (*client.ReachClient, error) {
	serverName, ok := args["server"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("parameter 'server' is required and must be a non-empty string")
	}

	cfg, err := client.LoadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	srv, ok := cfg.Servers[serverName]
	if !ok {
		return nil, fmt.Errorf("server %q not found; use 'reach add' to register it", serverName)
	}

	return client.NewClient(serverName, srv), nil
}

// toJSON 将 map 序列化为 JSON 字符串，失败时返回 fallback。
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// ── handlers ─────────────────────────────────────────────────────────────────

func handleReachBash(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	command, _ := args["command"].(string)
	if command == "" {
		return mcp.NewToolResultError("parameter 'command' is required"), nil
	}

	timeout := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}

	result, err := c.Exec(command, timeout)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("exec failed: %v", err)), nil
	}

	return mcp.NewToolResultText(toJSON(result)), nil
}

func handleReachRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return mcp.NewToolResultError("parameter 'path' is required"), nil
	}

	result, err := c.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read failed: %v", err)), nil
	}

	content, _ := result["content"].(string)
	return mcp.NewToolResultText(content), nil
}

func handleReachWrite(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return mcp.NewToolResultError("parameter 'path' is required"), nil
	}

	content, _ := args["content"].(string)

	if err := c.WriteFile(path, content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write failed: %v", err)), nil
	}

	return mcp.NewToolResultText("ok"), nil
}

func handleReachInfo(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	info, err := c.Info()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("info failed: %v", err)), nil
	}

	return mcp.NewToolResultText(toJSON(info)), nil
}

func handleReachList(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := client.LoadClientConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load config: %v", err)), nil
	}

	type serverEntry struct {
		Name string `json:"name"`
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	entries := make([]serverEntry, 0, len(cfg.Servers))
	for name, srv := range cfg.Servers {
		port := srv.Port
		if port == 0 {
			port = 7100
		}
		entries = append(entries, serverEntry{Name: name, Host: srv.Host, Port: port})
	}

	return mcp.NewToolResultText(toJSON(entries)), nil
}

func handleReachUpload(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	localPath, _ := args["local_path"].(string)
	if localPath == "" {
		return mcp.NewToolResultError("parameter 'local_path' is required"), nil
	}

	remotePath, _ := args["remote_path"].(string)
	if remotePath == "" {
		return mcp.NewToolResultError("parameter 'remote_path' is required"), nil
	}

	written, err := c.Upload(localPath, remotePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("upload failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("uploaded %d bytes", written)), nil
}

func handleReachStats(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	topN := 5
	if t, ok := args["top_n"].(float64); ok && t > 0 {
		topN = int(t)
	}
	result, err := c.Stats(topN)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("stats failed: %v", err)), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}

func handleReachDryRun(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	c, err := getClientFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command, _ := args["command"].(string)
	if command == "" {
		return mcp.NewToolResultError("parameter 'command' is required"), nil
	}
	result, err := c.DryRun(command)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dryrun failed: %v", err)), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}
