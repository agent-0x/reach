package cli

import (
	"fmt"
	"os"

	"github.com/agent-0x/reach/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(removeCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(infoCmd())
	rootCmd.AddCommand(healthCmd())
}

// getClient 从本地配置加载指定名称的服务器客户端
func getClient(name string) (*client.ReachClient, error) {
	cfg, err := client.LoadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	srv, ok := cfg.Servers[name]
	if !ok {
		return nil, fmt.Errorf("server %q not found; use 'reach add' to register it", name)
	}
	return client.NewClient(name, srv), nil
}

// addCmd — reach add <name> --host <ip> --token <token> [--port 7100]
func addCmd() *cobra.Command {
	var host, token string
	var port int

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a server (TOFU: fetch and pin certificate fingerprint)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if host == "" {
				return fmt.Errorf("--host is required")
			}
			if token == "" {
				return fmt.Errorf("--token is required")
			}

			// 临时配置（无指纹），用于获取指纹
			tmpCfg := &client.ServerConfig{
				Host:  host,
				Port:  port,
				Token: token,
			}
			tmpClient := client.NewClient(name, tmpCfg)

			// TOFU：获取证书指纹
			fp, err := tmpClient.GetFingerprint()
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Fingerprint: sha256:%s\n", fp)

			// 验证 health（带指纹）
			verifiedCfg := &client.ServerConfig{
				Host:        host,
				Port:        port,
				Token:       token,
				Fingerprint: fp,
			}
			verifiedClient := client.NewClient(name, verifiedCfg)
			ok, err := verifiedClient.Health()
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}
			if !ok {
				return fmt.Errorf("server health check returned not-OK")
			}

			// 保存到配置
			cfg, err := client.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Servers[name] = verifiedCfg
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Server %q added successfully.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "server IP or hostname (required)")
	cmd.Flags().StringVar(&token, "token", "", "agent token (required)")
	cmd.Flags().IntVar(&port, "port", 7100, "agent port")
	return cmd
}

// removeCmd — reach remove <name>
func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a server from local config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := client.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if _, ok := cfg.Servers[name]; !ok {
				return fmt.Errorf("server %q not found", name)
			}
			delete(cfg.Servers, name)
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Server %q removed.\n", name)
			return nil
		},
	}
}

// listCmd — reach list
func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := client.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.Servers) == 0 {
				fmt.Fprintln(os.Stdout, "No servers configured. Use 'reach add' to add one.")
				return nil
			}
			fmt.Fprintf(os.Stdout, "%-20s  %s\n", "NAME", "ADDRESS")
			fmt.Fprintf(os.Stdout, "%-20s  %s\n", "----", "-------")
			for name, srv := range cfg.Servers {
				port := srv.Port
				if port == 0 {
					port = 7100
				}
				fmt.Fprintf(os.Stdout, "%-20s  %s:%d\n", name, srv.Host, port)
			}
			return nil
		},
	}
}

// infoCmd — reach info <name>
func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show system information for a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(args[0])
			if err != nil {
				return err
			}
			info, err := c.Info()
			if err != nil {
				return err
			}
			printMap(info)
			return nil
		},
	}
}

// healthCmd — reach health <name>
func healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health <name>",
		Short: "Check health of a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(args[0])
			if err != nil {
				return err
			}
			ok, err := c.Health()
			if err != nil {
				fmt.Fprintf(os.Stdout, "FAIL: %v\n", err)
				return nil
			}
			if ok {
				fmt.Fprintln(os.Stdout, "OK")
			} else {
				fmt.Fprintln(os.Stdout, "FAIL")
			}
			return nil
		},
	}
}

// printMap 格式化打印 map 内容（key: value）
func printMap(m map[string]interface{}) {
	keys := []string{
		"hostname", "os", "arch", "cpu_count",
		"mem_total_mb", "mem_free_mb",
		"disk_total_gb", "disk_free_gb",
		"uptime_secs", "agent_version",
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			fmt.Fprintf(os.Stdout, "%-16s %v\n", k+":", v)
		}
	}
	// 打印任何不在预定义列表中的额外字段
	known := make(map[string]bool)
	for _, k := range keys {
		known[k] = true
	}
	for k, v := range m {
		if !known[k] {
			fmt.Fprintf(os.Stdout, "%-16s %v\n", k+":", v)
		}
	}
}
