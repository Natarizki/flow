package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Natarizki/flow/internal/cli"
)

var rootCmd = &cobra.Command{
	Use:   "flc",
	Short: "flc — FLOW CLI, peer-to-peer web caching client",
	Long:  "FLC (Flow CLI) is the command-line interface for FLOW, a P2P web caching and distributed content system.",
}

func main() {
	cli.RegisterAuthCommands(rootCmd)
	cli.RegisterPeerCommands(rootCmd)
	cli.RegisterCompressCommands(rootCmd)
	cli.RegisterCacheCommands(rootCmd)
	cli.RegisterPrefetchCommands(rootCmd)
	cli.RegisterSocialCommands(rootCmd)
	cli.RegisterNetworkCommands(rootCmd)
        cli.RegisterMediaCommands(rootCmd)

	// Cobra sudah nyediain generator completion built-in via
	// `completionCmd` internal, cukup panggil GenBashCompletionV2 dkk
	// lewat command "completion" bawaan — otomatis ada begitu ada
	// subcommand terdaftar, gak perlu didaftarin manual.
	rootCmd.CompletionOptions.DisableDefaultCmd = false

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
