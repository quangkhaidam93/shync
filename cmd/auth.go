package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Re-authenticate with the storage backend",
	Long:  "Clear saved credentials and re-run the authentication flow for the active backend.",
	RunE:  runAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	switch cfg.ActiveBackend {
	case "google_drive":
		return reauthGoogleDrive()
	case "synology":
		return reauthSynology()
	case "gist":
		return reauthGist()
	default:
		return fmt.Errorf("unknown backend: %s", cfg.ActiveBackend)
	}
}

func reauthGoogleDrive() error {
	tokenFile := cfg.GoogleDrive.TokenFile
	if tokenFile == "" {
		return fmt.Errorf("no token file configured")
	}

	if err := os.Remove(tokenFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing token file: %w", err)
	}

	// Clear cached folder ID so it gets re-discovered with new credentials
	cfg.GoogleDrive.FolderID = ""
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Re-authenticating with Google Drive...")
	if _, err := newBackend(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Authentication successful.")
	return nil
}

func reauthSynology() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Username [%s]: ", cfg.Synology.Username)
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)
	if user != "" {
		cfg.Synology.Username = user
	}

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	if len(passBytes) > 0 {
		cfg.Synology.Password = string(passBytes)
	}

	cfg.Synology.DeviceID = ""
	cfg.Synology.SharePath = ""
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Re-authenticating with Synology NAS...")
	if _, err := newBackend(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Authentication successful.")
	return nil
}

func reauthGist() error {
	fmt.Print("New GitHub PAT (input hidden): ")
	tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	cfg.Gist.Token = token
	cfg.Gist.GistID = ""
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Re-authenticating with GitHub Gist...")
	if _, err := newBackend(); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Authentication successful.")
	return nil
}
