package cli

import (
	"fmt"
	"net"
	"os"

	"github.com/agent-0x/reach/internal/agent"
	"github.com/spf13/cobra"
)

func init() {
	// reach agent
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the reach agent server",
	}

	// reach agent serve --config <path>
	var serveConfig string
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTPS agent server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := agent.LoadConfig(serveConfig)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			return agent.Serve(cfg)
		},
	}
	serveCmd.Flags().StringVar(&serveConfig, "config", "/etc/reach-agent/config.yaml", "path to config.yaml")

	// reach agent init --dir <path>
	var initDir string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize agent: generate TLS cert and token",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := agent.Init(initDir)
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			// 获取本机非 loopback IP 用于提示
			thisIP := detectOutboundIP()

			fmt.Fprintln(os.Stdout, "=== Reach Agent Initialized ===")
			fmt.Fprintf(os.Stdout, "Config: %s\n", result.ConfigPath)
			fmt.Fprintf(os.Stdout, "Token:  %s\n", result.Token)
			fmt.Fprintf(os.Stdout, "Fingerprint: sha256:%s\n", result.Fingerprint)
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintln(os.Stdout, "Add this server to your local machine:")
			fmt.Fprintf(os.Stdout, "  reach add <name> --host %s --token %s\n", thisIP, result.Token)
			return nil
		},
	}
	initCmd.Flags().StringVar(&initDir, "dir", "/etc/reach-agent", "directory to store cert, key, and config")

	agentCmd.AddCommand(serveCmd, initCmd)
	rootCmd.AddCommand(agentCmd)
}

// detectOutboundIP 返回本机对外可达的首个非 loopback IP，失败时返回 "<this-ip>"
func detectOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "<this-ip>"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "<this-ip>"
	}
	return addr.IP.String()
}
