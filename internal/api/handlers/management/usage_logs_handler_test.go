package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

type testMemoryStore struct {
	auths []*coreauth.Auth
}

func (s *testMemoryStore) List(context.Context) ([]*coreauth.Auth, error) { return s.auths, nil }
func (s *testMemoryStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	s.auths = append(s.auths, auth)
	return auth.ID, nil
}
func (s *testMemoryStore) Delete(_ context.Context, id string) error {
	for i, a := range s.auths {
		if a.ID == id {
			s.auths = append(s.auths[:i], s.auths[i+1:]...)
			break
		}
	}
	return nil
}

func TestGetUsageLogsKeepsStoredChannelNameWhenCurrentAuthNameDiffers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "usage.db")
	if err := usage.InitDB("sqlite", dbPath, config.RequestLogStorageConfig{}, time.UTC); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		usage.CloseDB()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})

	store := &testMemoryStore{}
	manager := coreauth.NewManager(store, nil, nil)
	auth, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "tabcode-auth",
		FileName: "tabcode.json",
		Provider: "codex",
		Label:    "tabcode-pro",
		Metadata: map[string]any{"label": "tabcode-pro"},
	})
	if err != nil {
		t.Fatalf("register auth: %v", err)
	}

	usage.InsertLog(
		0, "", "gpt-5.4", "tabcode-plus", "tabcode-plus", auth.Index,
		false, time.Now().UTC(), 123, 45,
		usage.TokenStats{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		"", "",
	)

	h := &Handler{
		cfg:         &config.Config{},
		authManager: manager,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/logs?days=7&page=1&size=50", nil)

	h.GetUsageLogs(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			ChannelName string `json:"channel_name"`
		} `json:"items"`
		Filters struct {
			Channels []string `json:"channels"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
	if payload.Items[0].ChannelName != "tabcode-plus" {
		t.Fatalf("channel_name = %q, want %q", payload.Items[0].ChannelName, "tabcode-plus")
	}
	if len(payload.Filters.Channels) != 1 || payload.Filters.Channels[0] != "tabcode-plus" {
		t.Fatalf("filters.channels = %#v, want [tabcode-plus]", payload.Filters.Channels)
	}

	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/logs?days=7&page=1&size=50&channel=tabcode-plus", nil)
	h.GetUsageLogs(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal filtered response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ChannelName != "tabcode-plus" {
		t.Fatalf("filtered items = %#v, want one tabcode-plus item", payload.Items)
	}

	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/logs?days=7&page=1&size=50&channel=tabcode-pro", nil)
	h.GetUsageLogs(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("mismatched filtered expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal mismatched filtered response: %v", err)
	}
	if len(payload.Items) != 0 {
		t.Fatalf("mismatched filtered items = %#v, want none", payload.Items)
	}
}

func TestGetUsageLogs_EmptyDB_DoesNotReturnNullSlices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "usage.db")
	if err := usage.InitDB("sqlite", dbPath, config.RequestLogStorageConfig{}, time.UTC); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		usage.CloseDB()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})

	h := &Handler{
		cfg: &config.Config{},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/logs?days=7&page=1&size=50", nil)

	h.GetUsageLogs(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Items   []any `json:"items"`
		Filters struct {
			APIKeys  []any    `json:"api_keys"`
			Models   []string `json:"models"`
			Channels []string `json:"channels"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if payload.Items == nil {
		t.Fatalf("items is null; expected []")
	}
	if payload.Filters.APIKeys == nil {
		t.Fatalf("filters.api_keys is null; expected []")
	}
	if payload.Filters.Models == nil {
		t.Fatalf("filters.models is null; expected []")
	}
	if payload.Filters.Channels == nil {
		t.Fatalf("filters.channels is null; expected []")
	}
}

func TestGetLogContent_ReturnsRequestDetailsPart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "usage.db")
	if err := usage.InitDB("sqlite", dbPath, config.RequestLogStorageConfig{
		StoreContent:           true,
		ContentRetentionDays:   30,
		CleanupIntervalMinutes: 1440,
	}, time.UTC); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		usage.CloseDB()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})

	details := `{"client":{"headers":{"Authorization":"Bearer sk-client-plaintext"}},"upstream":{"headers":{"Authorization":"Bearer sk-upstream-plaintext"}},"response":{"headers":{"X-Request-Id":"req-plaintext"}}}`
	usage.InsertLogWithDetails(
		0, "Primary", "gpt-test", "codex", "Codex", "auth-1",
		false, time.Now().UTC(), 100, 10,
		usage.TokenStats{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		`{"messages":[]}`, `{"choices":[]}`, details,
	)
	result, err := usage.QueryLogs(usage.LogQueryParams{Page: 1, Size: 10, Days: 1})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one log row, got %d", len(result.Items))
	}

	h := &Handler{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(result.Items[0].ID, 10)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/logs/1/content?part=details&format=json", nil)

	h.GetLogContent(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload struct {
		Part    string `json:"part"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Part != "details" || payload.Content != details {
		t.Fatalf("unexpected details payload: %+v", payload)
	}
}
