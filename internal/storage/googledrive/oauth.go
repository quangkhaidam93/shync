package googledrive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

func loadOAuthConfig(credentialsFile string) (*oauth2.Config, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}
	cfg, err := google.ConfigFromJSON(data, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return cfg, nil
}

func getToken(oauthCfg *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	tok, err := loadToken(tokenFile)
	if err == nil {
		return tok, nil
	}

	tok, err = obtainToken(oauthCfg)
	if err != nil {
		return nil, err
	}

	if err := saveToken(tokenFile, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

func obtainToken(oauthCfg *oauth2.Config) (*oauth2.Token, error) {
	// Generate PKCE verifier
	verifier := generateCodeVerifier()
	challenge := generateCodeChallenge(verifier)

	// Start local callback server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	oauthCfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			fmt.Fprint(w, "Error: no authorization code received.")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "Authorization successful! You can close this window.")
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	authURL := oauthCfg.AuthCodeURL("state",
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)

	fmt.Printf("\nOpen this URL in your browser to authorize shync:\n\n%s\n\nWaiting for authorization...\n", authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	}

	tok, err := oauthCfg.Exchange(context.Background(), code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}
	return tok, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
