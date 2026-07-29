package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func (d *DB) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := d.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check: %s", result)
	}
	return nil
}

func (d *DB) Backup(ctx context.Context, destination string) error {
	absolute, err := safeNewDestination(destination)
	if err != nil {
		return err
	}
	temporary := absolute + ".partial"
	_ = os.Remove(temporary)
	defer os.Remove(temporary)
	if _, err = d.db.ExecContext(ctx, `PRAGMA wal_checkpoint(FULL)`); err != nil {
		return err
	}
	if _, err = d.db.ExecContext(ctx, `VACUUM INTO ?`, temporary); err != nil {
		return fmt.Errorf("database backup: %w", err)
	}
	if err = os.Chmod(temporary, 0o600); err != nil {
		return err
	}
	if err = VerifyBackup(ctx, temporary); err != nil {
		return err
	}
	file, err := os.Open(temporary)
	if err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	_ = file.Close()
	if err = os.Rename(temporary, absolute); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(absolute))
}

func VerifyBackup(ctx context.Context, path string) error {
	absolute, err := regularFile(path)
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: absolute}).String() + "?mode=ro&immutable=1"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("backup integrity check failed")
	}
	for _, version := range []int{1, 2, 3, 4, 5} {
		var count int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=?`, version).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("backup migration %d missing", version)
		}
	}
	return nil
}

func RestoreBackup(ctx context.Context, backup, destination string) error {
	if err := VerifyBackup(ctx, backup); err != nil {
		return err
	}
	source, err := regularFile(backup)
	if err != nil {
		return err
	}
	destination, err = safeNewDestination(destination)
	if err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary := destination + ".partial"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if _, err = io.Copy(output, io.LimitReader(input, 2*1024*1024*1024+1)); err != nil {
		return err
	}
	info, err := output.Stat()
	if err != nil || info.Size() > 2*1024*1024*1024 {
		return errors.New("backup exceeds restore size limit")
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	if err = VerifyBackup(ctx, temporary); err != nil {
		return err
	}
	if err = os.Rename(temporary, destination); err != nil {
		return err
	}
	committed = true
	return syncDirectory(filepath.Dir(destination))
}

func safeNewDestination(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("destination must be absolute")
	}
	absolute := filepath.Clean(path)
	if info, err := os.Lstat(absolute); err == nil {
		return "", fmt.Errorf("destination already exists with mode %s", info.Mode())
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", errors.New("destination parent is not a directory")
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func regularFile(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("path must be a regular non-symlink file")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(resolved, 0) {
		return "", errors.New("invalid path")
	}
	return resolved, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
