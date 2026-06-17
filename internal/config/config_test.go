package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNormalizesNegativeTags(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	configContent := []byte("negative_tags:\n  - Sports\n  - esports\n  -  sports  \n  - \n")
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	expected := []string{"sports", "esports"}
	if len(cfg.NegativeTags) != len(expected) {
		t.Fatalf("expected %d negative tags, got %d (%v)", len(expected), len(cfg.NegativeTags), cfg.NegativeTags)
	}

	for i := range expected {
		if cfg.NegativeTags[i] != expected[i] {
			t.Fatalf("expected cfg.NegativeTags[%d] to be %q, got %q", i, expected[i], cfg.NegativeTags[i])
		}
	}
}
