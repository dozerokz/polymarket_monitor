package config

import (
	"fmt"
	filesreaders "polymarket_monitor/internal/files_readers"
	"slices"
	"strings"
)

type Config struct {
	NegativeTags []string `yaml:"negative_tags"`
}

func Load(filePath string) (Config, error) {
	var cfg Config
	if err := filesreaders.ReadYAML(filePath, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	cfg.NegativeTags = normalizeTags(cfg.NegativeTags)
	return cfg, nil
}

func normalizeTags(tags []string) []string {
	normalizedTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalizedTag := strings.ToLower(strings.TrimSpace(tag))
		if normalizedTag == "" || slices.Contains(normalizedTags, normalizedTag) {
			continue
		}
		normalizedTags = append(normalizedTags, normalizedTag)
	}

	return normalizedTags
}
