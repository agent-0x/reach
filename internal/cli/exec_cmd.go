package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(execCmd())
}

// execCmd — reach exec <server> <command...>
func execCmd() *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "exec <server> <command...>",
		Short: "Execute a command on a remote server",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]
			command := strings.Join(args[1:], " ")

			c, err := getClient(serverName)
			if err != nil {
				return err
			}

			result, err := c.Exec(command, timeout)
			if err != nil {
				return err
			}

			// 打印 stdout
			if stdout, ok := result["stdout"].(string); ok && stdout != "" {
				_, _ = fmt.Fprint(os.Stdout, stdout)
			}

			// 打印 stderr
			if stderr, ok := result["stderr"].(string); ok && stderr != "" {
				_, _ = fmt.Fprint(os.Stderr, stderr)
			}

			// 如果退出码非零，返回错误（但不打印额外信息，已经在上面打印了）
			if exitCode, ok := result["exit_code"].(float64); ok && exitCode != 0 {
				os.Exit(int(exitCode))
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&timeout, "timeout", "t", 30, "command timeout in seconds")
	return cmd
}
