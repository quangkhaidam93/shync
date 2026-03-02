package gist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
)

const apiBase = "https://api.github.com"

// Gist implements storage.Backend using a single GitHub Gist as storage.
type Gist struct {
	client *http.Client
	token  string
	gistID string
	cfg    *config.Config
}

// gistResponse represents the relevant fields from the GitHub Gist API.
type gistResponse struct {
	ID        string                `json:"id"`
	UpdatedAt time.Time             `json:"updated_at"`
	Files     map[string]gistFile   `json:"files"`
}

type gistFile struct {
	Filename  string `json:"filename"`
	Size      int    `json:"size"`
	Content   string `json:"content"`
	RawURL    string `json:"raw_url"`
	Truncated bool   `json:"truncated"`
}

// New creates a new Gist backend. If no gist_id is configured, a new private
// gist is created and its ID is saved to the config.
func New(cfg *config.Config) (*Gist, error) {
	if cfg.Gist.Token == "" {
		return nil, fmt.Errorf("gist token not configured — run 'shync init' or 'shync auth'")
	}

	g := &Gist{
		client: &http.Client{Timeout: 30 * time.Second},
		token:  cfg.Gist.Token,
		gistID: cfg.Gist.GistID,
		cfg:    cfg,
	}

	ctx := context.Background()

	if g.gistID == "" {
		id, err := g.createGist(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating gist: %w", err)
		}
		g.gistID = id
		cfg.Gist.GistID = id
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("saving gist id: %w", err)
		}
	} else {
		// Validate connectivity by fetching the gist.
		if _, err := g.fetchGist(ctx); err != nil {
			return nil, fmt.Errorf("connecting to gist: %w", err)
		}
	}

	return g, nil
}

func (g *Gist) Name() string { return "gist" }

func (g *Gist) Close() { g.client.CloseIdleConnections() }

func (g *Gist) Upload(ctx context.Context, remotePath string, src io.Reader, filename string) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	key := fileKey(remotePath)
	files := map[string]any{
		key: map[string]string{"content": string(data)},
	}
	return g.patchGist(ctx, files)
}

func (g *Gist) Download(ctx context.Context, remotePath string, dst io.Writer) error {
	resp, err := g.fetchGist(ctx)
	if err != nil {
		return err
	}

	key := fileKey(remotePath)
	f, ok := resp.Files[key]
	if !ok {
		return fmt.Errorf("file %q not found in gist", key)
	}

	// If content is not truncated, use inline content directly.
	if !f.Truncated {
		_, err := io.WriteString(dst, f.Content)
		return err
	}

	// For truncated (large) files, fetch from raw_url.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.RawURL, nil)
	if err != nil {
		return fmt.Errorf("creating raw request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)

	raw, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching raw content: %w", err)
	}
	defer raw.Body.Close()

	if raw.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching raw content: HTTP %d", raw.StatusCode)
	}

	_, err = io.Copy(dst, raw.Body)
	return err
}

func (g *Gist) List(ctx context.Context, remoteDir string) ([]storage.FileMetadata, error) {
	resp, err := g.fetchGist(ctx)
	if err != nil {
		return nil, err
	}

	var files []storage.FileMetadata
	for name, f := range resp.Files {
		// Skip the placeholder file used when creating the gist.
		if name == ".shync" {
			continue
		}
		files = append(files, storage.FileMetadata{
			Name:     name,
			Size:     int64(f.Size),
			Modified: resp.UpdatedAt,
		})
	}
	return files, nil
}

func (g *Gist) Exists(ctx context.Context, remotePath string) (bool, error) {
	resp, err := g.fetchGist(ctx)
	if err != nil {
		return false, err
	}

	key := fileKey(remotePath)
	_, ok := resp.Files[key]
	return ok, nil
}

func (g *Gist) Delete(ctx context.Context, remotePath string) error {
	key := fileKey(remotePath)
	// Setting a file to null in the PATCH payload removes it from the gist.
	files := map[string]any{
		key: nil,
	}
	return g.patchGist(ctx, files)
}

// fileKey extracts the filename from a remote path (e.g. "/shync/config.toml" → "config.toml").
func fileKey(remotePath string) string {
	return path.Base(remotePath)
}

// doRequest executes an HTTP request with auth headers and checks the status code.
func (g *Gist) doRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// fetchGist retrieves the gist metadata and file contents.
func (g *Gist) fetchGist(ctx context.Context) (*gistResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/gists/"+g.gistID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var gr gistResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decoding gist response: %w", err)
	}
	return &gr, nil
}

// createGist creates a new private gist with a .shync placeholder file.
func (g *Gist) createGist(ctx context.Context) (string, error) {
	payload := map[string]any{
		"description": "shync — synced config files",
		"public":      false,
		"files": map[string]any{
			".shync": map[string]string{
				"content": "managed by shync\n",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/gists", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.doRequest(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var gr gistResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", fmt.Errorf("decoding create response: %w", err)
	}
	if gr.ID == "" {
		return "", fmt.Errorf("gist created but no ID returned")
	}
	return gr.ID, nil
}

// patchGist updates the gist with the given file changes.
func (g *Gist) patchGist(ctx context.Context, files map[string]any) error {
	payload := map[string]any{"files": files}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiBase+"/gists/"+g.gistID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.doRequest(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
