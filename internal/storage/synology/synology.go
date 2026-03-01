package synology

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
)

type Synology struct {
	client *http.Client
	base   string
	sid    string
	cfg    *config.Config
}

func New(cfg *config.Config) (*Synology, error) {
	synCfg := &cfg.Synology
	client := newHTTPClient(synCfg)
	base := baseURL(synCfg)

	sid, err := login(client, base, cfg)
	if err != nil {
		return nil, err
	}

	s := &Synology{
		client: client,
		base:   base,
		sid:    sid,
		cfg:    cfg,
	}

	return s, nil
}

func (s *Synology) Name() string { return "synology" }

// Close closes idle TCP connections to the Synology server.
func (s *Synology) Close() {
	s.client.CloseIdleConnections()
}

func (s *Synology) Upload(ctx context.Context, remotePath string, src io.Reader, filename string) error {
	dir := path.Dir(s.fullPath(remotePath))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// All fields must come before the file part
	writer.WriteField("api", "SYNO.FileStation.Upload")
	writer.WriteField("version", "2")
	writer.WriteField("method", "upload")
	writer.WriteField("path", dir)
	writer.WriteField("create_parents", "true")
	writer.WriteField("overwrite", "true")
	writer.WriteField("_sid", s.sid)

	// File must be the last part
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, src); err != nil {
		return fmt.Errorf("writing file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.base+"/webapi/entry.cgi?_sid="+url.QueryEscape(s.sid), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return s.checkResponse(resp)
}

func (s *Synology) Download(ctx context.Context, remotePath string, dst io.Writer) error {
	params := url.Values{
		"api":     {"SYNO.FileStation.Download"},
		"version": {"2"},
		"method":  {"download"},
		"path":    {s.fullPath(remotePath)},
		"mode":    {"download"},
		"_sid":    {s.sid},
	}

	body := strings.NewReader(params.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST",
		s.base+"/webapi/entry.cgi", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		return s.checkResponse(resp)
	}
	if strings.HasPrefix(ct, "text/html") {
		return fmt.Errorf("download failed (status %d): endpoint returned HTML — is FileStation installed and running?",
			resp.StatusCode)
	}

	_, err = io.Copy(dst, resp.Body)
	return err
}

func (s *Synology) List(ctx context.Context, remoteDir string) ([]storage.FileMetadata, error) {
	params := url.Values{
		"api":        {"SYNO.FileStation.List"},
		"version":    {"2"},
		"method":     {"list"},
		"folder_path": {s.fullPath(remoteDir)},
		"additional": {"[\"size\",\"time\"]"},
		"_sid":       {s.sid},
	}

	resp, err := s.postForm(ctx, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Files []struct {
				Name       string `json:"name"`
				Additional struct {
					Size int64 `json:"size"`
					Time struct {
						Mtime int64 `json:"mtime"`
					} `json:"time"`
				} `json:"additional"`
			} `json:"files"`
		} `json:"data"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing list response: %w", err)
	}
	if !result.Success {
		return nil, fmt.Errorf("list failed (error code: %d)", result.Error.Code)
	}

	var files []storage.FileMetadata
	for _, f := range result.Data.Files {
		files = append(files, storage.FileMetadata{
			Name:     f.Name,
			Size:     f.Additional.Size,
			Modified: time.Unix(f.Additional.Time.Mtime, 0),
		})
	}
	return files, nil
}

func (s *Synology) Exists(ctx context.Context, remotePath string) (bool, error) {
	params := url.Values{
		"api":     {"SYNO.FileStation.List"},
		"version": {"2"},
		"method":  {"getinfo"},
		"path":    {s.fullPath(remotePath)},
		"_sid":    {s.sid},
	}

	resp, err := s.postForm(ctx, params)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Success, nil
}

func (s *Synology) Delete(ctx context.Context, remotePath string) error {
	params := url.Values{
		"api":     {"SYNO.FileStation.Delete"},
		"version": {"2"},
		"method":  {"delete"},
		"path":    {s.fullPath(remotePath)},
		"_sid":    {s.sid},
	}

	resp, err := s.postForm(ctx, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return s.checkResponse(resp)
}

func (s *Synology) fullPath(remotePath string) string {
	sharePath := s.cfg.Synology.SharePath
	if sharePath == "" {
		sharePath = "/home/Drive"
	}
	return sharePath + remotePath
}

// postForm sends a POST request with form-encoded params to /webapi/entry.cgi.
// On session timeout (error 106), it re-logins and retries once.
func (s *Synology) postForm(ctx context.Context, params url.Values) (*http.Response, error) {
	resp, err := s.doPost(ctx, params)
	if err != nil {
		return nil, err
	}

	// Peek at JSON responses to check for session timeout (error 106)
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return resp, nil
	}

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	var result struct {
		Success bool `json:"success"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(tee).Decode(&result); err == nil {
		if !result.Success && result.Error.Code == 106 {
			resp.Body.Close()
			// Re-login and retry
			sid, loginErr := login(s.client, s.base, s.cfg)
			if loginErr != nil {
				return nil, fmt.Errorf("re-login failed: %w", loginErr)
			}
			s.sid = sid
			params.Set("_sid", s.sid)
			return s.doPost(ctx, params)
		}
	}

	// Reconstruct the body from the buffer
	resp.Body.Close()
	resp.Body = io.NopCloser(&buf)
	return resp, nil
}

func (s *Synology) doPost(ctx context.Context, params url.Values) (*http.Response, error) {
	body := strings.NewReader(params.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", s.base+"/webapi/entry.cgi", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.client.Do(req)
}

// do executes a request (used only for multipart uploads).
func (s *Synology) do(req *http.Request) (*http.Response, error) {
	return s.client.Do(req)
}

func (s *Synology) checkResponse(resp *http.Response) error {
	var result struct {
		Success bool `json:"success"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("API error (code: %d)", result.Error.Code)
	}
	return nil
}
