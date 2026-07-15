package cli

import (
	"fmt"
        "os"
	"strings"

	"github.com/spf13/cobra"
)

type peerInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	Reputation float64 `json:"reputation"`
}

func RegisterPeerCommands(root *cobra.Command) {
	var visibility string

	createCmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a new peer",
		Aliases: []string{"c"},
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("peer")
			var resp peerInfo
			err := apiPost("/api/peers", map[string]string{
				"name":       name,
				"visibility": visibility,
			}, &resp)
			if err != nil {
				return err
			}
			fmt.Printf("created peer '%s' (id: %s, visibility: %s)\n", resp.Name, resp.ID, resp.Visibility)
			return nil
		},
	}
	createCmd.Flags().String("peer", "", "peer name")
	createCmd.Flags().StringVar(&visibility, "visible", "private", "visibility: public|private|internal")
	createCmd.MarkFlagRequired("peer")

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List all peers",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var peers []peerInfo
			if err := apiGet("/api/peers", &peers); err != nil {
				return err
			}
			if len(peers) == 0 {
				fmt.Println("no peers found")
				return nil
			}
			for _, p := range peers {
				fmt.Printf("%-20s %-10s %-10s rep:%.1f\n", p.Name, p.ID, p.Visibility, p.Reputation)
			}
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:     "show <peer>",
		Short:   "Show peer details",
		Aliases: []string{"sh"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var p peerInfo
			if err := apiGet("/api/peers/"+args[0], &p); err != nil {
				return err
			}
			fmt.Printf("Name: %s\nID: %s\nVisibility: %s\nReputation: %.2f\n", p.Name, p.ID, p.Visibility, p.Reputation)
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:     "delete <peer>",
		Short:   "Delete a peer",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			if !force {
				fmt.Print("are you sure? use --force to confirm: ")
				return nil
			}
			if err := apiPost("/api/peers/"+args[0]+"/delete", nil, nil); err != nil {
				return err
			}
			fmt.Printf("deleted peer %s\n", args[0])
			return nil
		},
	}
	deleteCmd.Flags().Bool("force", false, "confirm deletion")

	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a peer",
		RunE: func(cmd *cobra.Command, args []string) error {
			oldName, _ := cmd.Flags().GetString("old")
			newName, _ := cmd.Flags().GetString("new")
			if err := apiPost("/api/peers/"+oldName+"/rename", map[string]string{"new_name": newName}, nil); err != nil {
				return err
			}
			fmt.Printf("renamed %s -> %s\n", oldName, newName)
			return nil
		},
	}
	renameCmd.Flags().String("old", "", "current peer name")
	renameCmd.Flags().String("new", "", "new peer name")
	renameCmd.MarkFlagRequired("old")
	renameCmd.MarkFlagRequired("new")

	visibilityCmd := &cobra.Command{
		Use:   "visibility <peer>",
		Short: "Change peer visibility",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, _ := cmd.Flags().GetString("set")
			if err := apiPost("/api/peers/"+args[0]+"/visibility", map[string]string{"visibility": set}, nil); err != nil {
				return err
			}
			fmt.Printf("peer %s visibility set to %s\n", args[0], set)
			return nil
		},
	}
	visibilityCmd.Flags().String("set", "", "public|private|internal")
	visibilityCmd.MarkFlagRequired("set")

	lockCmd := &cobra.Command{
		Use:   "lock <peer>",
		Short: "Lock a peer",
		Aliases: []string{"lk"},
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, _ := cmd.Flags().GetString("message")
			if err := apiPost("/api/peers/"+args[0]+"/lock", map[string]string{"message": msg}, nil); err != nil {
				return err
			}
			fmt.Printf("locked peer %s\n", args[0])
			return nil
		},
	}
	lockCmd.Flags().String("message", "", "lock message")

	unlockCmd := &cobra.Command{
		Use:   "unlock <peer>",
		Short: "Unlock a peer",
		Aliases: []string{"ul"},
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apiPost("/api/peers/"+args[0]+"/unlock", nil, nil); err != nil {
				return err
			}
			fmt.Printf("unlocked peer %s\n", args[0])
			return nil
		},
	}

        readmeCmd := &cobra.Command{
		Use:   "readme <peer>",
		Short: "Set or view a peer's README (supports .md and .mdx)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, _ := cmd.Flags().GetString("file")
			if file == "" {
				var resp map[string]string
				if err := apiGet("/api/peers/"+args[0]+"/readme", &resp); err != nil {
					return err
				}
				if resp["readme"] == "" {
					fmt.Println("no README set for this peer")
					return nil
				}
				fmt.Println(resp["readme"])
				return nil
			}

			content, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", file, err)
			}

			format := "md"
			if strings.HasSuffix(strings.ToLower(file), ".mdx") {
				format = "mdx"
			}

			if err := apiPost("/api/peers/"+args[0]+"/readme", map[string]string{
				"content": string(content), "format": format,
			}, nil); err != nil {
				return err
			}
			fmt.Printf("README set for peer %s (%s)\n", args[0], format)
			return nil
		},
	}
	readmeCmd.Flags().String("file", "", "path to README.md or README.mdx to upload")

	root.AddCommand(createCmd, listCmd, showCmd, deleteCmd, renameCmd, visibilityCmd, lockCmd, unlockCmd, readmeCmd)

}
