package cli

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "reach",
	Short: "AI remote server agent — control any server from any AI",
}

func Execute() error {
	return rootCmd.Execute()
}
