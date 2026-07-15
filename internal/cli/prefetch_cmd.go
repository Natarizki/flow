package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func RegisterPrefetchCommands(root *cobra.Command) {
	prefetchCmd := &cobra.Command{
		Use:   "prefetch",
		Short: "Manage predictive prefetching",
	}

	var history string
	trainCmd := &cobra.Command{
		Use:   "train",
		Short: "Train the Markov chain predictor from history",
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]int
			if err := apiPost("/api/prefetch/train", map[string]string{"history_file": history}, &resp); err != nil {
				return err
			}
			fmt.Printf("trained on %d sessions\n", resp["sessions"])
			return nil
		},
	}
	trainCmd.Flags().StringVar(&history, "history", "", "path to history JSON file")
	trainCmd.MarkFlagRequired("history")

	predictCmd := &cobra.Command{
		Use:   "predict <url>",
		Short: "Show predicted next pages for a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp []map[string]interface{}
			if err := apiGet("/api/prefetch/predict?url="+args[0], &resp); err != nil {
				return err
			}
			for _, p := range resp {
				fmt.Printf("%.1f%%  %s\n", p["probability"].(float64)*100, p["url"])
			}
			return nil
		},
	}

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable predictive prefetch",
		RunE: func(cmd *cobra.Command, args []string) error {
			return apiPost("/api/prefetch/enable", nil, nil)
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable predictive prefetch",
		RunE: func(cmd *cobra.Command, args []string) error {
			return apiPost("/api/prefetch/disable", nil, nil)
		},
	}

	var depth int
	nowCmd := &cobra.Command{
		Use:   "now <url>",
		Short: "Trigger immediate prefetch from a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]int
			if err := apiPost("/api/prefetch/now", map[string]interface{}{"url": args[0], "depth": depth}, &resp); err != nil {
				return err
			}
			fmt.Printf("prefetched %d pages\n", resp["count"])
			return nil
		},
	}
	nowCmd.Flags().IntVar(&depth, "depth", 3, "prefetch depth")

	prefetchCmd.AddCommand(trainCmd, predictCmd, enableCmd, disableCmd, nowCmd)
	root.AddCommand(prefetchCmd)
}
