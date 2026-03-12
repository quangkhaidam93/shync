package googledrive

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type Drive struct {
	service  *drive.Service
	folderID string
	cfg      *config.Config
}

func New(cfg *config.Config) (*Drive, error) {
	oauthCfg, err := loadOAuthConfig(cfg.GoogleDrive.CredentialsFile)
	if err != nil {
		return nil, err
	}

	tok, err := getToken(oauthCfg, cfg.GoogleDrive.TokenFile)
	if err != nil {
		return nil, err
	}

	client := oauthCfg.Client(context.Background(), tok)
	svc, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}

	d := &Drive{
		service:  svc,
		folderID: cfg.GoogleDrive.FolderID,
		cfg:      cfg,
	}

	// Ensure shync folder exists
	if d.folderID == "" {
		folderID, err := d.ensureFolder(context.Background())
		if err != nil {
			return nil, err
		}
		d.folderID = folderID
		cfg.GoogleDrive.FolderID = folderID
	}

	return d, nil
}

func (d *Drive) Name() string { return "google_drive" }

func (d *Drive) Close() {}

func (d *Drive) Upload(ctx context.Context, remotePath string, src io.Reader, filename string) error {
	name := path.Base(remotePath)

	// Check if file already exists and update it
	existing, err := d.findFile(ctx, name)
	if err != nil {
		return err
	}

	if existing != nil {
		_, err = d.service.Files.Update(existing.Id, &drive.File{}).
			Media(src).
			Context(ctx).
			Do()
		return err
	}

	f := &drive.File{
		Name:    name,
		Parents: []string{d.folderID},
	}
	_, err = d.service.Files.Create(f).
		Media(src).
		Context(ctx).
		Do()
	return err
}

func (d *Drive) Download(ctx context.Context, remotePath string, dst io.Writer) error {
	name := path.Base(remotePath)
	file, err := d.findFile(ctx, name)
	if err != nil {
		return err
	}
	if file == nil {
		return fmt.Errorf("file not found: %s", name)
	}

	resp, err := d.service.Files.Get(file.Id).
		Context(ctx).
		Download()
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(dst, resp.Body)
	return err
}

func (d *Drive) List(ctx context.Context, remoteDir string) ([]storage.FileMetadata, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false", d.folderID)
	result, err := d.service.Files.List().
		Q(query).
		Fields("files(id, name, size, modifiedTime)").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
	}

	var files []storage.FileMetadata
	for _, f := range result.Files {
		modified, _ := time.Parse(time.RFC3339, f.ModifiedTime)
		files = append(files, storage.FileMetadata{
			Name:     f.Name,
			Size:     f.Size,
			Modified: modified,
		})
	}
	return files, nil
}

func (d *Drive) Exists(ctx context.Context, remotePath string) (bool, error) {
	name := path.Base(remotePath)
	file, err := d.findFile(ctx, name)
	if err != nil {
		return false, err
	}
	return file != nil, nil
}

func (d *Drive) Delete(ctx context.Context, remotePath string) error {
	name := path.Base(remotePath)
	file, err := d.findFile(ctx, name)
	if err != nil {
		return err
	}
	if file == nil {
		return fmt.Errorf("file not found: %s", name)
	}
	return d.service.Files.Delete(file.Id).Context(ctx).Do()
}

func (d *Drive) findFile(ctx context.Context, name string) (*drive.File, error) {
	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", name, d.folderID)
	result, err := d.service.Files.List().
		Q(query).
		Fields("files(id, name, size, modifiedTime)").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("searching for file: %w", err)
	}
	if len(result.Files) == 0 {
		return nil, nil
	}
	return result.Files[0], nil
}

func (d *Drive) ensureFolder(ctx context.Context) (string, error) {
	// Check if folder already exists
	query := "name = 'shync' and mimeType = 'application/vnd.google-apps.folder' and trashed = false"
	result, err := d.service.Files.List().
		Q(query).
		Fields("files(id)").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("searching for shync folder: %w", err)
	}
	if len(result.Files) > 0 {
		return result.Files[0].Id, nil
	}

	// Create folder
	folder := &drive.File{
		Name:     "shync",
		MimeType: "application/vnd.google-apps.folder",
	}
	created, err := d.service.Files.Create(folder).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("creating shync folder: %w", err)
	}
	return created.Id, nil
}
