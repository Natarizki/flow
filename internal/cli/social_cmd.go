package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type leaderboardEntry struct {
	PeerID       string `json:"peer_id"`
	Username     string `json:"username"`
	BytesServed  int64  `json:"bytes_served"`
	ChunksServed int64  `json:"chunks_served"`
	Score        int64  `json:"score"`
}

func RegisterSocialCommands(root *cobra.Command) {
	var limit int
	leaderboardCmd := &cobra.Command{
		Use:     "leaderboard",
		Short:   "Show the global contribution leaderboard",
		Aliases: []string{"lb"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var entries []leaderboardEntry
			path := fmt.Sprintf("/api/leaderboard?limit=%d", limit)
			if err := apiGet(path, &entries); err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("no contributions recorded yet")
				return nil
			}
			for i, e := range entries {
				name := e.Username
				if name == "" {
					name = e.PeerID[:min(len(e.PeerID), 16)]
				}
				fmt.Printf("%2d. %-20s score:%-8d chunks:%-6d bytes:%d\n", i+1, name, e.Score, e.ChunksServed, e.BytesServed)
			}
			return nil
		},
	}
	leaderboardCmd.Flags().IntVar(&limit, "limit", 10, "number of entries to show")

	var badgeFilter string
	achievementsCmd := &cobra.Command{
		Use:   "achievements",
		Short: "Show your unlocked achievement badges",
		RunE: func(cmd *cobra.Command, args []string) error {
			peerID, _ := cmd.Flags().GetString("peer")
			if peerID == "" {
				return fmt.Errorf("--peer is required (your peer ID)")
			}

			var resp struct {
				Unlocked []map[string]string `json:"unlocked"`
				Catalog  []map[string]string  `json:"catalog"`
			}
			if err := apiGet("/api/achievements?peer_id="+peerID, &resp); err != nil {
				return err
			}

			if badgeFilter != "" {
				for _, b := range resp.Unlocked {
					if b["name"] == badgeFilter {
						fmt.Printf("✓ %s [%s] — %s\n", b["name"], b["tier"], b["description"])
						return nil
					}
				}
				fmt.Printf("badge '%s' not unlocked yet\n", badgeFilter)
				return nil
			}

			fmt.Printf("Unlocked: %d / %d\n\n", len(resp.Unlocked), len(resp.Catalog))
			for _, b := range resp.Unlocked {
				fmt.Printf("✓ %-24s [%-8s] %s\n", b["name"], b["tier"], b["description"])
			}
			return nil
		},
	}
	achievementsCmd.Flags().String("peer", "", "your peer ID")
	achievementsCmd.Flags().StringVar(&badgeFilter, "badge", "", "show a specific badge by name")

	root.AddCommand(leaderboardCmd, achievementsCmd)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
