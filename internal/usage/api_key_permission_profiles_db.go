package usage

import (
	"encoding/json"
	"strings"
)

const apiKeyPermissionProfilesMigrationBackupSuffix = ".pre-api-key-permission-profiles-sqlite-migration"

type APIKeyPermissionProfileRow struct {
	ID                   string   `json:"id" yaml:"id"`
	Name                 string   `json:"name" yaml:"name"`
	DailyLimit           int      `json:"daily-limit" yaml:"daily-limit"`
	TotalQuota           int      `json:"total-quota" yaml:"total-quota"`
	ConcurrencyLimit     int      `json:"concurrency-limit" yaml:"concurrency-limit"`
	RPMLimit             int      `json:"rpm-limit" yaml:"rpm-limit"`
	TPMLimit             int      `json:"tpm-limit" yaml:"tpm-limit"`
	AllowedModels        []string `json:"allowed-models" yaml:"allowed-models"`
	AllowedChannels      []string `json:"allowed-channels" yaml:"allowed-channels"`
	AllowedChannelGroups []string `json:"allowed-channel-groups" yaml:"allowed-channel-groups"`
	SystemPrompt         string   `json:"system-prompt" yaml:"system-prompt"`
	CreatedAt            string   `json:"created-at,omitempty" yaml:"created-at,omitempty"`
	UpdatedAt            string   `json:"updated-at,omitempty" yaml:"updated-at,omitempty"`
}

func ListAPIKeyPermissionProfiles() []APIKeyPermissionProfileRow {
	return GormListAPIKeyPermissionProfiles()
}

func ReplaceAllAPIKeyPermissionProfiles(profiles []APIKeyPermissionProfileRow) error {
	return GormReplaceAllAPIKeyPermissionProfiles(profiles)
}

func MigrateAPIKeyPermissionProfilesFromYAML(configFilePath string) int {
	return GormMigrateAPIKeyPermissionProfilesFromYAML(configFilePath)
}

func cleanAPIKeyPermissionProfilesFromYAML(configFilePath string) {
	cleanConfigKeysFromYAML(configFilePath, map[string]bool{
		"api-key-permission-profiles": true,
	}, "api_key_permission_profiles")
}

func normalizeAPIKeyPermissionProfile(profile APIKeyPermissionProfileRow) APIKeyPermissionProfileRow {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.DailyLimit = normalizeNonNegativeInt(profile.DailyLimit)
	profile.TotalQuota = normalizeNonNegativeInt(profile.TotalQuota)
	profile.ConcurrencyLimit = normalizeNonNegativeInt(profile.ConcurrencyLimit)
	profile.RPMLimit = normalizeNonNegativeInt(profile.RPMLimit)
	profile.TPMLimit = normalizeNonNegativeInt(profile.TPMLimit)
	profile.AllowedModels = normalizeStringSlice(profile.AllowedModels)
	profile.AllowedChannels = normalizeStringSlice(profile.AllowedChannels)
	profile.AllowedChannelGroups = normalizeStringSlice(profile.AllowedChannelGroups)
	profile.SystemPrompt = strings.TrimSpace(profile.SystemPrompt)
	return profile
}

func normalizeNonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if result == nil {
		return []string{}
	}
	return result
}

func mustJSONStringList(values []string) string {
	normalized := normalizeStringSlice(values)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeJSONStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return normalizeStringSlice(values)
}
