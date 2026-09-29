// Package backup creates and validates portable instance backups without starting provider services.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/alerts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

const databaseName = "sublane.db"
const keyName = "credentials.key"
const manifestName = "manifest.json"
const MaxDatabaseBytes int64 = 4 << 30
const MaxArchiveBytes int64 = MaxDatabaseBytes + (64 << 20)
const Timeout = 15 * time.Minute

var ErrInvalid = errors.New("invalid_backup")
var ErrExists = errors.New("backup_destination_exists")

type Info struct {
	CreatedAt     time.Time `json:"created_at"`
	Version       string    `json:"version"`
	SchemaVersion int       `json:"schema_version"`
	DatabaseBytes int64     `json:"database_bytes"`
}

func vacant(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
func stageFor(path string) (string, error) {
	if path == "" {
		return "", ErrInvalid
	}
	if err := vacant(path); err != nil {
		return "", err
	}
	return os.MkdirTemp(filepath.Dir(path), ".sublane-backup-")
}

func Create(ctx context.Context, dataDir, output, version string) (Info, error) {
	info := Info{CreatedAt: time.Now().UTC().Truncate(time.Second), Version: version}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	stage, err := stageFor(output)
	if err != nil {
		return info, err
	}
	defer os.RemoveAll(stage)
	if err := copyKey(ctx, filepath.Join(dataDir, keyName), filepath.Join(stage, keyName)); err != nil {
		return info, err
	}
	snapshot := filepath.Join(stage, databaseName)
	if err := storage.Snapshot(ctx, filepath.Join(dataDir, databaseName), snapshot, MaxDatabaseBytes); err != nil {
		return info, err
	}
	connection, err := storage.OpenReadOnly(ctx, snapshot)
	if err != nil {
		return info, err
	}
	info.SchemaVersion, err = storage.ValidateSnapshot(ctx, connection)
	if err == nil {
		var current int
		current, err = storage.CurrentSchemaVersion()
		if err == nil && current != info.SchemaVersion {
			err = errors.New("start this SubLane version to migrate the source before backing it up")
		}
	}
	if err == nil {
		err = verifyCredentials(ctx, connection, stage)
	}
	closeErr := connection.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return info, err
	}
	stat, err := os.Stat(snapshot)
	if err != nil {
		return info, err
	}
	info.DatabaseBytes = stat.Size()
	archive := filepath.Join(stage, "bundle.tar.gz")
	if err := writeArchive(ctx, stage, archive, info); err != nil {
		return info, err
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	if err := publish(archive, output); err != nil {
		return info, err
	}
	return info, nil
}

func Verify(ctx context.Context, input string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	stage, err := os.MkdirTemp("", ".sublane-backup-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(stage)
	info, connection, err := prepare(ctx, input, stage)
	if err != nil {
		return info, err
	}
	return info, connection.Close()
}

func Restore(ctx context.Context, input, target string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	stage, err := stageFor(target)
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(stage)
	info, connection, err := prepare(ctx, input, stage)
	if err != nil {
		return info, err
	}
	// A backup must not reactivate browser sessions that were revoked after the snapshot.
	err = db.New(connection).RevokeRestoredSessions(ctx)
	if err == nil {
		_, err = connection.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	}
	closeErr := connection.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return info, err
	}
	if err := os.Remove(filepath.Join(stage, manifestName)); err != nil {
		return info, err
	}
	for _, name := range []string{databaseName, keyName} {
		if err := syncFile(filepath.Join(stage, name)); err != nil {
			return info, err
		}
	}
	if err := syncDirectory(stage); err != nil {
		return info, err
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	if err := publish(stage, target); err != nil {
		return info, err
	}
	return info, nil
}

func prepare(ctx context.Context, input, stage string) (Info, *sql.DB, error) {
	info, err := extractArchive(ctx, input, stage)
	if err != nil {
		return info, nil, err
	}
	path := filepath.Join(stage, databaseName)
	source, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		return info, nil, ErrInvalid
	}
	schema, err := storage.ValidateSnapshot(ctx, source)
	closeErr := source.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return info, nil, fmt.Errorf("%w: database validation failed: %v", ErrInvalid, err)
	}
	if schema != info.SchemaVersion {
		return info, nil, ErrInvalid
	}
	// Upgrade only the private extracted copy, never the archive or the live database.
	connection, err := storage.Open(ctx, path)
	if err != nil {
		return info, nil, err
	}
	if err = storage.LimitSnapshotValues(ctx, connection); err == nil {
		_, err = storage.ValidateSnapshot(ctx, connection)
	}
	if err == nil {
		err = verifyCredentials(ctx, connection, stage)
	}
	if err != nil {
		connection.Close()
		return info, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return info, connection, nil
}
func verifyCredentials(ctx context.Context, connection *sql.DB, directory string) error {
	oversized, err := db.New(connection).CountOversizedBackupRecords(ctx)
	if err != nil {
		return err
	}
	if oversized != 0 {
		return fmt.Errorf("%w: credential records exceed backup bounds", ErrInvalid)
	}

	cipher, err := vault.Open(filepath.Join(directory, keyName), false)
	if err != nil {
		return errors.New("backup credential key is missing or invalid")
	}
	if err := accounts.New(connection, cipher).Verify(ctx); err != nil {
		return errors.New("backup account credentials cannot be decrypted")
	}
	if err := apikey.New(connection, cipher).Verify(ctx); err != nil {
		return errors.New("backup API keys cannot be decrypted")
	}
	if err := alerts.New(connection, cipher, nil).Verify(ctx); err != nil {
		return errors.New("backup webhook credentials cannot be decrypted")
	}
	return nil
}
func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
func publish(source, target string) error {
	if err := renameExclusive(source, target); err != nil {
		if os.IsExist(err) {
			return ErrExists
		}
		return err
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf("backup was published but directory synchronization failed: %w", err)
	}
	return nil
}
