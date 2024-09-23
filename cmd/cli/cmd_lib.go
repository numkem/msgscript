package main

import (
	"github.com/spf13/cobra"
)

var libCmd = &cobra.Command{
	Use:   "lib",
	Short: "library related commands",
}

func init() {
	rootCmd.AddCommand(libCmd)
}
