package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize shync configuration",
	Long:  "Interactive setup — choose a storage backend, enter credentials, and write the config file.",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	cfg := config.Default()

	path := cfgFile
	if path == "" {
		path = config.DefaultPath()
	}

	// Check if config already exists
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Config already exists at %s\n", path)
		overwrite := promptui.Select{
			Label: "Overwrite",
			Items: []string{"No", "Yes"},
		}
		_, overwriteChoice, err := overwrite.Run()
		if err != nil || overwriteChoice == "No" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Choose backend
	sel := promptui.Select{
		Label: "Select backend",
		Items: backends,
		Size:  len(backends),
	}

	_, chosen, err := sel.Run()
	if err != nil {
		fmt.Println("Aborted.")
		return nil
	}

	cfg.ActiveBackend = chosen
	fmt.Printf("\nConfigure %s:\n", chosen)

	switch chosen {
	case "synology":
		if err := setupSynology(reader, cfg); err != nil {
			return err
		}
	default:
		if err := setupGoogleDrive(reader, cfg); err != nil {
			return err
		}
	}

	// Remote directory
	fmt.Printf("\nRemote directory [%s]: ", cfg.RemoteDir)
	dir, _ := reader.ReadString('\n')
	dir = strings.TrimSpace(dir)
	if dir != "" {
		cfg.RemoteDir = dir
	}

	// Backup expiry
	fmt.Printf("Backup expiry [%s]: ", cfg.BackupExpiry)
	expiry, _ := reader.ReadString('\n')
	expiry = strings.TrimSpace(expiry)
	if expiry != "" {
		cfg.BackupExpiry = expiry
	}

	if err := cfg.SaveTo(path); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConfig written to %s\n", path)

	// Trigger auth flow so user completes setup in one step
	fmt.Printf("\nAuthenticating with %s...\n", chosen)
	if _, err := newBackendWith(cfg); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Println("Authentication successful.")

	fmt.Println("Run 'shync up <file>' to start syncing files.")
	return nil
}

func setupGoogleDrive(reader *bufio.Reader, cfg *config.Config) error {
	// Check if credentials file already exists
	if _, err := os.Stat(cfg.GoogleDrive.CredentialsFile); err != nil {
		home, _ := os.UserHomeDir()
		displayPath := strings.Replace(cfg.GoogleDrive.CredentialsFile, home, "$HOME", 1)
		fmt.Printf(`
To use Google Drive, you need OAuth credentials from Google Cloud Console.
Follow these steps:

  1. Go to https://console.cloud.google.com/projectcreate
     - Create a new project (e.g. "shync")

  2. Go to https://console.cloud.google.com/apis/library/drive.googleapis.com
     - Click "Enable" to enable the Google Drive API

  3. Go to https://console.cloud.google.com/auth/branding
     - Set App name (e.g. "shync") and your email as support email
     - Click Save

  4. Go to https://console.cloud.google.com/auth/audience
     - Select "External" user type
     - Under "Test users", add your Google email
     - Click Save

  5. Go to https://console.cloud.google.com/auth/clients
     - Click "+ Create Client"
     - Application type: "Desktop app", name: "shync"
     - Click Create, then "Download JSON"

  6. Copy the downloaded file:
     cp ~/Downloads/client_secret_*.json %s

Note: When authorizing, you may see "Google hasn't verified this app".
      This is normal — click "Advanced" > "Go to shync (unsafe)" to continue.
`, displayPath)
		fmt.Print("Press Enter once the credentials file is in place (or type a path): ")
		creds, _ := reader.ReadString('\n')
		creds = strings.TrimSpace(creds)
		if creds != "" {
			cfg.GoogleDrive.CredentialsFile = creds
		}

		// Validate the file exists
		if _, err := os.Stat(cfg.GoogleDrive.CredentialsFile); err != nil {
			return fmt.Errorf("credentials file not found: %s", cfg.GoogleDrive.CredentialsFile)
		}
	} else {
		fmt.Printf("\nCredentials file found: %s\n", cfg.GoogleDrive.CredentialsFile)
	}

	fmt.Printf("Token file [%s]: ", cfg.GoogleDrive.TokenFile)
	tok, _ := reader.ReadString('\n')
	tok = strings.TrimSpace(tok)
	if tok != "" {
		cfg.GoogleDrive.TokenFile = tok
	}

	return nil
}

func setupSynology(reader *bufio.Reader, cfg *config.Config) error {
	fmt.Print("\nSynology host (e.g., 192.168.1.100 or nas.example.com): ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host != "" {
		cfg.Synology.Host = host
	}

	fmt.Printf("Port [%d]: ", cfg.Synology.Port)
	port, _ := reader.ReadString('\n')
	port = strings.TrimSpace(port)
	if port != "" {
		fmt.Sscanf(port, "%d", &cfg.Synology.Port)
	}

	httpsSelect := promptui.Select{
		Label: "Use HTTPS",
		Items: []string{"Yes", "No"},
	}
	_, httpsChoice, _ := httpsSelect.Run()
	cfg.Synology.HTTPS = httpsChoice == "Yes"

	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	cfg.Synology.Username = strings.TrimSpace(user)

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after hidden input
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	cfg.Synology.Password = string(passBytes)

	defaultShare := cfg.Synology.SharePath
	if defaultShare == "" {
		defaultShare = "/homes/" + cfg.Synology.Username + "/Drive"
	}
	fmt.Printf("Share path [%s]: ", defaultShare)
	share, _ := reader.ReadString('\n')
	share = strings.TrimSpace(share)
	if share != "" {
		cfg.Synology.SharePath = share
	}

	sslSelect := promptui.Select{
		Label: "Verify SSL",
		Items: []string{"No", "Yes"},
	}
	_, sslChoice, _ := sslSelect.Run()
	cfg.Synology.VerifySSL = sslChoice == "Yes"

	return nil
}
