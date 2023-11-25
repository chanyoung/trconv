package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "help")
	}

	trconvCmd.SetArgs(os.Args[1:])
	trconvCmd.Execute()
}

var trconvCmd = &cobra.Command {
	Use: "trconv [command] (flags)",
	Short: "Block trace converter",
	Long: "Block trace converter",
	Version: "v0.0.1",
}

func init() {
	cobra.EnableCommandSorting = false
}

