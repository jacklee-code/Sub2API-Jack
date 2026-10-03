package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CreateJackUpdateBackup fails the update before binary replacement if a complete
// database snapshot cannot be persisted. It does not roll the database back.
func (s *BackupService) CreateJackUpdateBackup(ctx context.Context, version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	root := filepath.Join(filepath.Dir(exe), "update-backups")
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(root, time.Now().UTC().Format("20060102T150405Z")+"-")
	if err != nil {
		return err
	}
	archive, _, err := s.createCompressedBackupFile(ctx)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	for _, pair := range [][2]string{{archive, filepath.Join(dir, "database.sql.gz")}, {exe, filepath.Join(dir, "sub2api")}} {
		if err = copyJackBackupFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	configPath := filepath.Join(filepath.Dir(filepath.Dir(exe)), "config.yaml")
	if _, err = os.Stat(configPath); err == nil {
		if err = copyJackBackupFile(configPath, filepath.Join(dir, "config.yaml")); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "version.txt"), []byte(version+"\n"), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "COMPLETE"), []byte("Database, executable and available configuration saved. Database restore requires a maintenance window.\n"), 0600)
}
func copyJackBackupFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if err = out.Sync(); copyErr == nil {
		copyErr = err
	}
	if err = out.Close(); copyErr == nil {
		copyErr = err
	}
	if copyErr != nil {
		return fmt.Errorf("save update backup: %w", copyErr)
	}
	return nil
}
