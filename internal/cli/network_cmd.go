package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func RegisterNetworkCommands(root *cobra.Command) {
	whoisCmd := &cobra.Command{
		Use:   "whois <peer>",
		Short: "Show detailed info about a peer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]interface{}
			if err := apiGet("/api/whois?peer="+args[0], &resp); err != nil {
				return err
			}
			fmt.Printf("Peer: %v\n", args[0])
			if tags, ok := resp["tags"]; ok {
				fmt.Printf("Tags: %v\n", tags)
			}
			if rank, ok := resp["leaderboard_rank"]; ok {
				fmt.Printf("Leaderboard rank: %v\n", rank)
			}
			if bs, ok := resp["bytes_served"]; ok {
				fmt.Printf("Bytes served: %v\n", bs)
			}
			return nil
		},
	}

	var lanFlag, orgFlag string
	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover peers on LAN or within an organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			useLAN, _ := cmd.Flags().GetBool("lan")
			org, _ := cmd.Flags().GetString("org")

			var path string
			if useLAN {
				path = "/api/discover/lan"
			} else if org != "" {
				path = "/api/discover/org?org=" + org
			} else {
				return fmt.Errorf("specify --lan or --org <name>")
			}

			var peers []map[string]interface{}
			if err := apiGet(path, &peers); err != nil {
				return err
			}
			if len(peers) == 0 {
				fmt.Println("no peers found")
				return nil
			}
			for _, p := range peers {
				fmt.Printf("%v  %v\n", p["Name"], p["Address"])
			}
			return nil
		},
	}
	discoverCmd.Flags().Bool("lan", false, "discover peers on local network")
	discoverCmd.Flags().StringVar(&orgFlag, "org", "", "discover peers in organization")
	_ = lanFlag

	msgCmd := &cobra.Command{
		Use:   "msg <peer> <message>",
		Short: "Send a direct message to a peer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apiPost("/api/peers/"+args[0]+"/message", map[string]string{"message": args[1]}, nil); err != nil {
				return err
			}
			fmt.Printf("message sent to %s\n", args[0])
			return nil
		},
	}

	bandwidthCmd := &cobra.Command{
		Use:   "bandwidth",
		Short: "Show bandwidth usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			today, _ := cmd.Flags().GetBool("today")
			month, _ := cmd.Flags().GetBool("month")

			path := "/api/bandwidth/today"
			if month {
				path = "/api/bandwidth/month"
			} else if !today {
				path = "/api/bandwidth/today"
			}

			var resp map[string]interface{}
			if err := apiGet(path, &resp); err != nil {
				return err
			}
			fmt.Printf("Date: %v\n", resp["Date"])
			fmt.Printf("Uploaded: %v bytes\n", resp["BytesUp"])
			fmt.Printf("Downloaded: %v bytes\n", resp["BytesDown"])
			return nil
		},
	}
	bandwidthCmd.Flags().Bool("today", true, "show today's bandwidth")
	bandwidthCmd.Flags().Bool("month", false, "show this month's bandwidth")

	tagCmd := &cobra.Command{Use: "tag", Short: "Manage peer tags"}
	tagAddCmd := &cobra.Command{
		Use:   "add <peer>",
		Short: "Add a tag to a peer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			if err := apiPost("/api/tags/add", map[string]string{"peer_name": args[0], "tag": tag}, nil); err != nil {
				return err
			}
			fmt.Printf("tagged %s with '%s'\n", args[0], tag)
			return nil
		},
	}
	tagAddCmd.Flags().String("tag", "", "tag to add")
	tagAddCmd.MarkFlagRequired("tag")

	tagRemoveCmd := &cobra.Command{
		Use:   "remove <peer>",
		Short: "Remove a tag from a peer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			if err := apiPost("/api/tags/remove", map[string]string{"peer_name": args[0], "tag": tag}, nil); err != nil {
				return err
			}
			fmt.Printf("removed tag '%s' from %s\n", tag, args[0])
			return nil
		},
	}
	tagRemoveCmd.Flags().String("tag", "", "tag to remove")
	tagRemoveCmd.MarkFlagRequired("tag")

	tagListCmd := &cobra.Command{
		Use:   "list",
		Short: "List peers with a given tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			var peers []map[string]interface{}
			if err := apiGet("/api/tags/list?tag="+tag, &peers); err != nil {
				return err
			}
			for _, p := range peers {
				fmt.Printf("%v\n", p["Name"])
			}
			return nil
		},
	}
	tagListCmd.Flags().String("tag", "", "tag to filter by")
	tagListCmd.MarkFlagRequired("tag")

	tagCmd.AddCommand(tagAddCmd, tagRemoveCmd, tagListCmd)

	orgCmd := &cobra.Command{Use: "org", Short: "Manage organizations"}
	orgCreateCmd := &cobra.Command{
		Use:  "create \"<name>\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]interface{}
			if err := apiPost("/api/org/create", map[string]string{"name": args[0]}, &resp); err != nil {
				return err
			}
			fmt.Printf("created org '%s' — invite code: %v\n", args[0], resp["InviteCode"])
			return nil
		},
	}
	orgJoinCmd := &cobra.Command{
		Use:  "join \"<name>\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code, _ := cmd.Flags().GetString("invite-code")
			var resp map[string]interface{}
			if err := apiPost("/api/org/join", map[string]string{"invite_code": code}, &resp); err != nil {
				return err
			}
			fmt.Printf("joined org '%v'\n", resp["Name"])
			return nil
		},
	}
	orgJoinCmd.Flags().String("invite-code", "", "invite code")
	orgJoinCmd.MarkFlagRequired("invite-code")

	orgListCmd := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			var orgs []map[string]interface{}
			if err := apiGet("/api/org/list", &orgs); err != nil {
				return err
			}
			for _, o := range orgs {
				fmt.Printf("%v  (%v members)\n", o["Name"], len(o["MemberIDs"].([]interface{})))
			}
			return nil
		},
	}

	orgCmd.AddCommand(orgCreateCmd, orgJoinCmd, orgListCmd)

	root.AddCommand(whoisCmd, discoverCmd, msgCmd, bandwidthCmd, tagCmd, orgCmd)
}
