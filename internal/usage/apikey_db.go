package usage

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// APIKeyRow mirrors config.APIKeyEntry and is used for persistence.
type APIKeyRow struct {
	ID                   int64    `json:"id"`
	Key                  string   `json:"key"`
	Name                 string   `json:"name,omitempty"`
	UserID               *int64   `json:"user-id,omitempty"`
	Disabled             bool     `json:"disabled,omitempty"`
	DailyLimit           int      `json:"daily-limit,omitempty"`
	TotalQuota           int      `json:"total-quota,omitempty"`
	SpendingLimit        float64  `json:"spending-limit,omitempty"`
	ConcurrencyLimit     int      `json:"concurrency-limit,omitempty"`
	RPMLimit             int      `json:"rpm-limit,omitempty"`
	TPMLimit             int      `json:"tpm-limit,omitempty"`
	AllowedModels        []string `json:"allowed-models,omitempty"`
	AllowedChannels      []string `json:"allowed-channels,omitempty"`
	AllowedChannelGroups []string `json:"allowed-channel-groups,omitempty"`
	SystemPrompt         string   `json:"system-prompt,omitempty"`
	CreatedAt            string   `json:"created-at,omitempty"`
	UpdatedAt            string   `json:"updated-at,omitempty"`
}

// ToConfigEntry converts an APIKeyRow to a config.APIKeyEntry.
func (r *APIKeyRow) ToConfigEntry() config.APIKeyEntry {
	return config.APIKeyEntry{
		ID:                   r.ID,
		Key:                  r.Key,
		Name:                 r.Name,
		UserID:               r.UserID,
		Disabled:             r.Disabled,
		DailyLimit:           r.DailyLimit,
		TotalQuota:           r.TotalQuota,
		SpendingLimit:        r.SpendingLimit,
		ConcurrencyLimit:     r.ConcurrencyLimit,
		RPMLimit:             r.RPMLimit,
		TPMLimit:             r.TPMLimit,
		AllowedModels:        r.AllowedModels,
		AllowedChannels:      r.AllowedChannels,
		AllowedChannelGroups: r.AllowedChannelGroups,
		SystemPrompt:         r.SystemPrompt,
		CreatedAt:            r.CreatedAt,
	}
}

// APIKeyRowFromConfig converts a config.APIKeyEntry to an APIKeyRow.
func APIKeyRowFromConfig(entry config.APIKeyEntry) APIKeyRow {
	return APIKeyRow{
		ID:                   entry.ID,
		Key:                  entry.Key,
		Name:                 entry.Name,
		UserID:               entry.UserID,
		Disabled:             entry.Disabled,
		DailyLimit:           entry.DailyLimit,
		TotalQuota:           entry.TotalQuota,
		SpendingLimit:        entry.SpendingLimit,
		ConcurrencyLimit:     entry.ConcurrencyLimit,
		RPMLimit:             entry.RPMLimit,
		TPMLimit:             entry.TPMLimit,
		AllowedModels:        entry.AllowedModels,
		AllowedChannels:      entry.AllowedChannels,
		AllowedChannelGroups: entry.AllowedChannelGroups,
		SystemPrompt:         entry.SystemPrompt,
		CreatedAt:            entry.CreatedAt,
	}
}

func defaultAPIKeyName(index int) string {
	if index < 0 {
		index = 0
	}
	return fmt.Sprintf("api-key-%d", index+1)
}

// MigrateAPIKeysFromConfig moves API key entries from YAML config into the database.
func MigrateAPIKeysFromConfig(cfg *config.Config, configFilePath string) int {
	return GormMigrateAPIKeysFromConfig(cfg, configFilePath)
}

// ListAPIKeys retrieves all API key entries.
func ListAPIKeys() []APIKeyRow {
	return GormListAPIKeys()
}

// ListAPIKeysByUserID retrieves API key entries for a specific user.
func ListAPIKeysByUserID(userID int64) []APIKeyRow {
	return GormListAPIKeysByUserID(userID)
}

// GetAPIKey retrieves a single API key entry by key string.
func GetAPIKey(key string) *APIKeyRow {
	return GormGetAPIKey(key)
}

// GetAPIKeyByID retrieves a single API key entry by numeric ID.
func GetAPIKeyByID(id int64) *APIKeyRow {
	return GormGetAPIKeyByID(id)
}

// UpsertAPIKey inserts or updates an API key entry.
func UpsertAPIKey(entry APIKeyRow) error {
	return GormUpsertAPIKey(entry)
}

// DeleteAPIKey removes an API key entry by key string.
func DeleteAPIKey(key string) error {
	return GormDeleteAPIKey(key)
}

// UpdateAPIKeyDisabledByIDAndUserID updates the disabled status of an API key
// only if it belongs to the given user.
func UpdateAPIKeyDisabledByIDAndUserID(id int64, userID int64, disabled bool) error {
	return GormUpdateAPIKeyDisabledByIDAndUserID(id, userID, disabled)
}

// DeleteAPIKeyByID removes an API key entry by numeric ID.
func DeleteAPIKeyByID(id int64) error {
	return GormDeleteAPIKeyByID(id)
}

// ReplaceAllAPIKeys atomically replaces all API keys with the given list.
func ReplaceAllAPIKeys(entries []APIKeyRow) error {
	return GormReplaceAllAPIKeys(entries)
}
