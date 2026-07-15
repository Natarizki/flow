package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func RegisterMediaCommands(root *cobra.Command) {
	bookmarkCmd := &cobra.Command{Use: "bookmark", Short: "Manage bookmarks"}

	addCmd := &cobra.Command{
		Use:  "add <url>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title, _ := cmd.Flags().GetString("title")
			if err := apiPost("/api/bookmarks/add", map[string]interface{}{"url": args[0], "title": title}, nil); err != nil {
				return err
			}
			fmt.Println("bookmark added")
			return nil
		},
	}
	addCmd.Flags().String("title", "", "bookmark title")

	listCmd := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			var bookmarks []map[string]interface{}
			if err := apiGet("/api/bookmarks", &bookmarks); err != nil {
				return err
			}
			for _, b := range bookmarks {
				fmt.Printf("%v — %v\n", b["Title"], b["URL"])
			}
			return nil
		},
	}

	bookmarkCmd.AddCommand(addCmd, listCmd)

	wikiCmd := &cobra.Command{
		Use:   "wikipedia-precache",
		Short: "Pre-cache today's most-read Wikipedia articles for offline reading",
		RunE: func(cmd *cobra.Command, args []string) error {
			lang, _ := cmd.Flags().GetString("lang")
			var resp map[string]int
			if err := apiPost("/api/wikipedia/precache", map[string]string{"lang": lang}, &resp); err != nil {
				return err
			}
			fmt.Printf("cached %d articles for offline reading\n", resp["cached"])
			return nil
		},
	}
	wikiCmd.Flags().String("lang", "en", "Wikipedia language edition")

	root.AddCommand(bookmarkCmd, wikiCmd)
}
