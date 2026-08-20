// Package metadata stores tenant and policy authority in PostgreSQL.
package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct{ db *sql.DB }

func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

// Tenant implements ingest.Authenticator and matches only key hashes.
func (s *Store) Tenant(ctx context.Context, key string) (string, bool, error) {
	var tenantID string
	err := s.db.QueryRowContext(ctx, `select tenant_id from api_keys where key_hash = $1 and disabled_at is null`, HashAPIKey(key)).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return tenantID, true, nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateTenantKey(ctx context.Context, tenantID, keyHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `insert into tenants (id) values ($1) on conflict do nothing`, tenantID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `insert into api_keys (tenant_id, key_hash) values ($1, $2) on conflict (key_hash) do nothing`, tenantID, keyHash); err != nil {
		return err
	}
	return tx.Commit()
}
