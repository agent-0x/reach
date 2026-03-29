package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(readCmd())
	rootCmd.AddCommand(writeCmd())
	rootCmd.AddCommand(uploadCmd())
	rootCmd.AddCommand(downloadCmd())
}

// readCmd — reach read <server> <path>
func readCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <server> <path>",
		Short: "Read a file from a remote server and print to stdout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]
			remotePath := args[1]

			c, err := getClient(serverName)
			if err != nil {
				return err
			}

			result, err := c.ReadFile(remotePath)
			if err != nil {
				return err
			}

			content, ok := result["content"].(string)
			if !ok {
				return fmt.Errorf("unexpected response format")
			}
			fmt.Fprint(os.Stdout, content)
			return nil
		},
	}
}

// writeCmd — reach write <server> <path>  (从 stdin 读内容)
func writeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "write <server> <path>",
		Short: "Write stdin content to a file on a remote server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]
			remotePath := args[1]

			c, err := getClient(serverName)
			if err != nil {
				return err
			}

			content, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}

			if err := c.WriteFile(remotePath, string(content)); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Written to %s\n", remotePath)
			return nil
		},
	}
}

// uploadCmd — reach upload <server> <local> <remote>
func uploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload <server> <local> <remote>",
		Short: "Upload a local file to a remote server",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]
			localPath := args[1]
			remotePath := args[2]

			c, err := getClient(serverName)
			if err != nil {
				return err
			}

			n, err := c.Upload(localPath, remotePath)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Uploaded %s → %s (%d bytes)\n", localPath, remotePath, n)
			return nil
		},
	}
}

// downloadCmd — reach download <server> <remote> <local>
func downloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download <server> <remote> <local>",
		Short: "Download a file from a remote server to local",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverName := args[0]
			remotePath := args[1]
			localPath := args[2]

			c, err := getClient(serverName)
			if err != nil {
				return err
			}

			if err := c.Download(remotePath, localPath); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Downloaded %s → %s\n", remotePath, localPath)
			return nil
		},
	}
}
