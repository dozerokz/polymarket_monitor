package activitycache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	filesreaders "polymarket_monitor/internal/files_readers"
	"time"

	_ "modernc.org/sqlite"
)

const maxSeenEventsPerWallet = 500

type SeenEvent struct {
	Key             string
	TransactionHash string
	SeenAt          time.Time
}

type Store struct {
	db *sql.DB
}

func MigrateLegacyDatabase(legacyPath string, targetPath string) error {
	resolvedLegacyPath, err := filesreaders.ResolvePath(legacyPath)
	if err != nil {
		return fmt.Errorf("failed to resolve legacy activity cache path: %w", err)
	}

	resolvedTargetPath, err := filesreaders.ResolvePath(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target activity cache path: %w", err)
	}

	if _, err = os.Stat(resolvedTargetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect target activity cache: %w", err)
	}

	if _, err = os.Stat(resolvedLegacyPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to inspect legacy activity cache: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(resolvedTargetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create target activity cache directory: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err = moveFileIfExists(resolvedLegacyPath+suffix, resolvedTargetPath+suffix); err != nil {
			return err
		}
	}

	if err = os.Rename(resolvedLegacyPath, resolvedTargetPath); err != nil {
		return fmt.Errorf("failed to move legacy activity cache: %w", err)
	}

	return nil
}

func moveFileIfExists(sourcePath string, targetPath string) error {
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to inspect legacy SQLite file %s: %w", sourcePath, err)
	}

	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect target SQLite file %s: %w", targetPath, err)
	}

	if err := os.Rename(sourcePath, targetPath); err != nil {
		return fmt.Errorf("failed to move legacy SQLite file %s: %w", sourcePath, err)
	}

	return nil
}

func Open(filePath string) (*Store, error) {
	resolvedPath, err := filesreaders.ResolvePath(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve activity cache path: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create activity cache directory: %w", err)
	}

	db, err := sql.Open("sqlite", resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open activity cache database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

func (s *Store) IsWalletInitialized(wallet string) (bool, error) {
	var initialized bool
	err := s.db.QueryRow(`
		SELECT initialized
		FROM wallet_state
		WHERE wallet = ?
	`, wallet).Scan(&initialized)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read wallet state for %s: %w", wallet, err)
	}

	return initialized, nil
}

func (s *Store) SeedWallet(wallet string, events []SeenEvent, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start activity cache seed transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = s.upsertWalletStateTx(tx, wallet, true, now); err != nil {
		return err
	}

	if err = s.rememberEventsTx(tx, wallet, events); err != nil {
		return err
	}

	if err = s.pruneWalletEventsTx(tx, wallet); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit activity cache seed transaction: %w", err)
	}

	return nil
}

func (s *Store) HasSeenEvent(wallet string, eventKey string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM seen_activity
			WHERE wallet = ? AND event_key = ?
		)
	`, wallet, eventKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check cached activity for wallet %s: %w", wallet, err)
	}

	return exists, nil
}

func (s *Store) ActivityWatermark(wallet string) (time.Time, error) {
	var initializedAt string
	var latestActivityAt sql.NullString
	err := s.db.QueryRow(`
		SELECT ws.updated_at, MAX(sa.seen_at)
		FROM wallet_state ws
		LEFT JOIN seen_activity sa ON sa.wallet = ws.wallet
		WHERE ws.wallet = ?
		GROUP BY ws.wallet, ws.updated_at
	`, wallet).Scan(&initializedAt, &latestActivityAt)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read activity watermark for wallet %s: %w", wallet, err)
	}

	watermark, err := time.Parse(time.RFC3339Nano, initializedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse initialization time for wallet %s: %w", wallet, err)
	}

	if latestActivityAt.Valid {
		latestSeenAt, parseErr := time.Parse(time.RFC3339Nano, latestActivityAt.String)
		if parseErr != nil {
			return time.Time{}, fmt.Errorf("failed to parse latest activity time for wallet %s: %w", wallet, parseErr)
		}
		if latestSeenAt.After(watermark) {
			watermark = latestSeenAt
		}
	}

	return watermark.UTC(), nil
}

func (s *Store) RememberEvent(wallet string, event SeenEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start remember activity transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = s.rememberEventsTx(tx, wallet, []SeenEvent{event}); err != nil {
		return err
	}

	if err = s.pruneWalletEventsTx(tx, wallet); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit remember activity transaction: %w", err)
	}

	return nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS wallet_state (
			wallet TEXT PRIMARY KEY,
			initialized INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS seen_activity (
			wallet TEXT NOT NULL,
			event_key TEXT NOT NULL,
			transaction_hash TEXT NOT NULL,
			seen_at TEXT NOT NULL,
			PRIMARY KEY (wallet, event_key)
		);

		CREATE INDEX IF NOT EXISTS idx_seen_activity_wallet_seen_at
		ON seen_activity(wallet, seen_at DESC);
	`); err != nil {
		return fmt.Errorf("failed to migrate activity cache database: %w", err)
	}

	return nil
}

func (s *Store) upsertWalletStateTx(tx *sql.Tx, wallet string, initialized bool, now time.Time) error {
	if _, err := tx.Exec(`
		INSERT INTO wallet_state(wallet, initialized, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(wallet) DO UPDATE SET
			initialized = excluded.initialized,
			updated_at = excluded.updated_at
	`, wallet, initialized, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("failed to upsert wallet state for %s: %w", wallet, err)
	}

	return nil
}

func (s *Store) rememberEventsTx(tx *sql.Tx, wallet string, events []SeenEvent) error {
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO seen_activity(wallet, event_key, transaction_hash, seen_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare remember activity statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		if event.Key == "" {
			continue
		}

		seenAt := event.SeenAt
		if seenAt.IsZero() {
			seenAt = time.Now().UTC()
		}

		if _, err = stmt.Exec(wallet, event.Key, event.TransactionHash, seenAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("failed to remember activity for wallet %s: %w", wallet, err)
		}
	}

	return nil
}

func (s *Store) pruneWalletEventsTx(tx *sql.Tx, wallet string) error {
	if _, err := tx.Exec(`
		DELETE FROM seen_activity
		WHERE wallet = ?
		  AND event_key NOT IN (
			SELECT event_key
			FROM seen_activity
			WHERE wallet = ?
			ORDER BY seen_at DESC
			LIMIT ?
		  )
	`, wallet, wallet, maxSeenEventsPerWallet); err != nil {
		return fmt.Errorf("failed to prune activity cache for wallet %s: %w", wallet, err)
	}

	return nil
}
