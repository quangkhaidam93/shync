package storage

import (
	"context"
	"io"
	"time"
)

type FileMetadata struct {
	Name     string
	Size     int64
	Modified time.Time
}

type Backend interface {
	Upload(ctx context.Context, remotePath string, src io.Reader, filename string) error
	Download(ctx context.Context, remotePath string, dst io.Writer) error
	List(ctx context.Context, remoteDir string) ([]FileMetadata, error)
	Exists(ctx context.Context, remotePath string) (bool, error)
	Delete(ctx context.Context, remotePath string) error
	Name() string
}
