package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

type ExcludedModelsSummary struct {
	hash  string
	count int
}

// SummarizeExcludedModels normalizes and hashes an excluded-model list.
func SummarizeExcludedModels(list []string) ExcludedModelsSummary {
	if len(list) == 0 {
		return ExcludedModelsSummary{}
	}
	seen := make(map[string]struct{}, len(list))
	normalized := make([]string, 0, len(list))
	for _, entry := range list {
		if trimmed := strings.ToLower(strings.TrimSpace(entry)); trimmed != "" {
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			normalized = append(normalized, trimmed)
		}
	}
	sort.Strings(normalized)
	return ExcludedModelsSummary{
		hash:  ComputeExcludedModelsHash(normalized),
		count: len(normalized),
	}
}

type AmpModelMappingsSummary struct {
	hash  string
	count int
}

// SummarizeAmpModelMappings hashes Amp model mappings for change detection.
func SummarizeAmpModelMappings(mappings []config.AmpModelMapping) AmpModelMappingsSummary {
	if len(mappings) == 0 {
		return AmpModelMappingsSummary{}
	}
	entries := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		from := strings.TrimSpace(mapping.From)
		to := strings.TrimSpace(mapping.To)
		if from == "" && to == "" {
			continue
		}
		entries = append(entries, from+"->"+to)
	}
	if len(entries) == 0 {
		return AmpModelMappingsSummary{}
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "|")))
	return AmpModelMappingsSummary{
		hash:  hex.EncodeToString(sum[:]),
		count: len(entries),
	}
}
