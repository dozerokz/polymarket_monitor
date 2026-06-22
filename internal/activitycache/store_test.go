package activitycache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateLegacyDatabase(t *testing.T) {
	tempDir := t.TempDir()
	legacyPath := filepath.Join(tempDir, "activity_cache.sqlite")
	targetPath := filepath.Join(tempDir, "data", "activity_cache.sqlite")

	if err := os.WriteFile(legacyPath, []byte("database"), 0o600); err != nil {
		t.Fatalf("failed to create legacy database: %v", err)
	}
	if err := os.WriteFile(legacyPath+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatalf("failed to create legacy WAL file: %v", err)
	}

	if err := MigrateLegacyDatabase(legacyPath, targetPath); err != nil {
		t.Fatalf("MigrateLegacyDatabase returned error: %v", err)
	}

	for _, path := range []string{targetPath, targetPath + "-wal"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected migrated file %s: %v", path, err)
		}
	}
	for _, path := range []string{legacyPath, legacyPath + "-wal"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected legacy file %s to be removed, got err=%v", path, err)
		}
	}
}

func TestSeedWalletAndRememberEvent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "data", "activity_cache.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	initialized, err := store.IsWalletInitialized("wallet-1")
	if err != nil {
		t.Fatalf("IsWalletInitialized returned error: %v", err)
	}
	if initialized {
		t.Fatal("expected wallet to be uninitialized")
	}

	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	if err = store.SeedWallet("wallet-1", []SeenEvent{
		{Key: "event-1", TransactionHash: "tx-1", SeenAt: now},
	}, now); err != nil {
		t.Fatalf("SeedWallet returned error: %v", err)
	}

	initialized, err = store.IsWalletInitialized("wallet-1")
	if err != nil {
		t.Fatalf("IsWalletInitialized returned error after seed: %v", err)
	}
	if !initialized {
		t.Fatal("expected wallet to be initialized after seed")
	}

	seen, err := store.HasSeenEvent("wallet-1", "event-1")
	if err != nil {
		t.Fatalf("HasSeenEvent returned error: %v", err)
	}
	if !seen {
		t.Fatal("expected seeded event to be marked as seen")
	}

	if err = store.RememberEvent("wallet-1", SeenEvent{
		Key:             "event-2",
		TransactionHash: "tx-2",
		SeenAt:          now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("RememberEvent returned error: %v", err)
	}

	seen, err = store.HasSeenEvent("wallet-1", "event-2")
	if err != nil {
		t.Fatalf("HasSeenEvent returned error for remembered event: %v", err)
	}
	if !seen {
		t.Fatal("expected remembered event to be marked as seen")
	}
}

func TestActivityWatermarkUsesLatestSeenEvent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "activity_cache.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	initializedAt := time.Date(2026, time.June, 22, 12, 0, 0, 0, time.UTC)
	if err = store.SeedWallet("wallet-1", []SeenEvent{
		{
			Key:             "old-event",
			TransactionHash: "old-tx",
			SeenAt:          initializedAt.Add(-time.Hour),
		},
	}, initializedAt); err != nil {
		t.Fatalf("SeedWallet returned error: %v", err)
	}

	watermark, err := store.ActivityWatermark("wallet-1")
	if err != nil {
		t.Fatalf("ActivityWatermark returned error: %v", err)
	}
	if !watermark.Equal(initializedAt) {
		t.Fatalf("expected initialization time watermark %s, got %s", initializedAt, watermark)
	}

	latestActivityAt := initializedAt.Add(time.Minute)
	if err = store.RememberEvent("wallet-1", SeenEvent{
		Key:             "new-event",
		TransactionHash: "new-tx",
		SeenAt:          latestActivityAt,
	}); err != nil {
		t.Fatalf("RememberEvent returned error: %v", err)
	}

	watermark, err = store.ActivityWatermark("wallet-1")
	if err != nil {
		t.Fatalf("ActivityWatermark returned error: %v", err)
	}
	if !watermark.Equal(latestActivityAt) {
		t.Fatalf("expected latest activity watermark %s, got %s", latestActivityAt, watermark)
	}
}
