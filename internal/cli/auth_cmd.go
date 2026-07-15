package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type loginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

func RegisterAuthCommands(root *cobra.Command) {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	var email, password, username string

	loginCmd := &cobra.Command{
		Use:     "login",
		Short:   "Log in to FLOW",
		Aliases: []string{"l"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp loginResponse
			err := apiPost("/api/auth/login", map[string]string{
				"email":    email,
				"password": password,
			}, &resp)
			if err != nil {
				return err
			}
			if err := SaveToken(resp.Token); err != nil {
				return err
			}
			fmt.Printf("logged in as %s (%s)\n", resp.User.Username, resp.User.Email)
			return nil
		},
	}
	loginCmd.Flags().StringVar(&email, "email", "", "account email")
	loginCmd.Flags().StringVar(&password, "password", "", "account password")
	loginCmd.MarkFlagRequired("email")
	loginCmd.MarkFlagRequired("password")

	registerCmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new FLOW account",
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp loginResponse
			err := apiPost("/api/auth/register", map[string]string{
				"email":    email,
				"username": username,
				"password": password,
			}, &resp)
			if err != nil {
				return err
			}
			fmt.Printf("registered account %s (%s), run 'flc auth login' to continue\n", resp.User.Username, resp.User.Email)
			return nil
		},
	}
	registerCmd.Flags().StringVar(&email, "email", "", "account email")
	registerCmd.Flags().StringVar(&username, "username", "", "desired username")
	registerCmd.Flags().StringVar(&password, "password", "", "account password")
	registerCmd.MarkFlagRequired("email")
	registerCmd.MarkFlagRequired("username")
	registerCmd.MarkFlagRequired("password")

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of FLOW",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apiPost("/api/auth/logout", nil, nil); err != nil {
				return err
			}
			if err := ClearToken(); err != nil {
				return err
			}
			fmt.Println("logged out")
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current login status",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := LoadToken()
			if err != nil {
				fmt.Println("not logged in")
				return nil
			}
			var resp map[string]interface{}
			if err := apiGet("/api/auth/status", &resp); err != nil {
				fmt.Println("session invalid, please login again")
				return nil
			}
			fmt.Printf("logged in, token: %s...\n", token[:12])
			return nil
		},
	}

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the current auth token",
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]string
			if err := apiPost("/api/auth/refresh", nil, &resp); err != nil {
				return err
			}
			if err := SaveToken(resp["token"]); err != nil {
				return err
			}
			fmt.Println("token refreshed")
			return nil
		},
	}

	authCmd.AddCommand(loginCmd, registerCmd, logoutCmd, statusCmd, refreshCmd)
	root.AddCommand(authCmd)

	// alias: flc a l -> auth login
	aCmd := &cobra.Command{Use: "a", Short: "alias for auth", Hidden: true}
	aCmd.AddCommand(&cobra.Command{
		Use: "l", Short: "alias for auth login", Hidden: true,
		RunE: loginCmd.RunE,
	})
	root.AddCommand(aCmd)
}
