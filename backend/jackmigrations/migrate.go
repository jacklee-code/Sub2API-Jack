// Package jackmigrations keeps downstream migrations independent of upstream numbering.
package jackmigrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7241060301)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS jack_schema_migrations (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		body, readErr := files.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(body))
		var old string
		err = tx.QueryRowContext(ctx, `SELECT checksum FROM jack_schema_migrations WHERE name=$1`, name).Scan(&old)
		if err == nil {
			if old != sum {
				return fmt.Errorf("Jack migration %s checksum mismatch", name)
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("Jack migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO jack_schema_migrations(name,checksum) VALUES($1,$2)`, name, sum); err != nil {
			return err
		}
	}
	return tx.Commit()
}
