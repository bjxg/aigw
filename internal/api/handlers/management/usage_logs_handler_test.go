package management

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestGetUsageLogsResolvesLegacySourceChannelName(t *testing.T) {
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

	store := &memoryAuthStore{}
	manager := coreauth.NewManager(store, nil, nil)
	auth, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "oauth-auth-logs",
		FileName: "codex-test.json",
		Provider: "codex",
		Label:    "GPT1",
		Metadata: map[string]any{
			"label": "GPT1",
			"email": "pcamtu927@gmail.com",
		},
	})
	if err != nil {
		t.Fatalf("register auth: %v", err)
	}

	usage.InsertLog(
		"", "", "gpt-5.4", "pcamtu927@gmail.com", "pcamtu927@gmail.com", auth.Index,
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
			ChannelName  string `json:"channel_name"`
			AuthIndex    string `json:"auth_index"`
			FirstTokenMs int64  `json:"first_token_ms"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
	if payload.Items[0].AuthIndex != auth.Index {
		t.Fatalf("auth_index = %q, want %q", payload.Items[0].AuthIndex, auth.Index)
	}
	if payload.Items[0].ChannelName != "GPT1" {
		t.Fatalf("channel_name = %q, want %q", payload.Items[0].ChannelName, "GPT1")
	}
	if payload.Items[0].FirstTokenMs != 45 {
		t.Fatalf("first_token_ms = %d, want %d", payload.Items[0].FirstTokenMs, 45)
	}
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

	store := &memoryAuthStore{}
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
		"", "", "gpt-5.4", "tabcode-plus", "tabcode-plus", auth.Index,
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
			APIKeys  []any   `json:"api_keys"`
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
		"sk-test", "Primary", "gpt-test", "codex", "Codex", "auth-1",
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

func TestGetPublicLogContent_RejectsRequestDetailsPart(t *testing.T) {
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

	h := &Handler{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v0/management/public/usage/logs/1/content",
		bytes.NewReader([]byte(`{"api_key":"sk-test","part":"details","format":"json"}`)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	h.GetPublicLogContent(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestGetPublicUsageLogs_EmptyDB_DoesNotReturnNullModels(t *testing.T) {
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
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v0/management/public/usage/logs",
		bytes.NewReader([]byte(`{"api_key":"sk-test","days":7,"page":1,"size":50}`)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	h.GetPublicUsageLogs(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Filters struct {
			Models []string `json:"models"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Filters.Models == nil {
		t.Fatalf("filters.models is null; expected []")
	}
}

func TestGetPublicUsageLogs_AcceptsPOSTBody(t *testing.T) {
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

	body := []byte(`{"api_key":"sk-test","days":7,"page":1,"size":50}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v0/management/public/usage/logs",
		bytes.NewReader(body),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	h.GetPublicUsageLogs(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Filters struct {
			Models []string `json:"models"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Filters.Models == nil {
		t.Fatalf("filters.models is null; expected []")
	}
}

func TestGetPublicUsageLogs_DoesNotReadAPIKeyFromQuery(t *testing.T) {
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
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/v0/management/public/usage/logs?api_key=sk-test&days=7&page=1&size=50",
		nil,
	)

	h.GetPublicUsageLogs(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "api_key parameter is required") {
		t.Fatalf("expected query api_key to be ignored, body=%s", rec.Body.String())
	}
}

func TestGetPublicUsageLogs_RejectsOversizedPOSTBody(t *testing.T) {
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

	body := bytes.Repeat([]byte("a"), int(publicLookupBodyLimit)+1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v0/management/public/usage/logs",
		bytes.NewReader(body),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	h.GetPublicUsageLogs(c)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request body too large") {
		t.Fatalf("expected oversized body rejection, body=%s", rec.Body.String())
	}
}
