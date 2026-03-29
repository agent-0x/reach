package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-0x/reach/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Serve 启动 MCP stdio server，阻塞直到 stdin 关闭。
func Serve() error {
	s := server.NewMCPServer("reach", "0.1.0")

	s.AddTool(toolReachBash(), handleReachBash)
	s.AddTool(toolReachRead(), handleReachRead)
	s.AddTool(toolReachWrite(), handleReachWrite)
	s.AddTool(toolReachInfo(), handleReachInfo)
	s.AddTool(toolReachList(), handleReachList)
	s.AddTool(toolReachUpload(), handleReachUpload)

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
