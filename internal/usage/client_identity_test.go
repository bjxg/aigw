package usage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestClientIdentityLogRoundTripWithoutContent(t *testing.T) {
	database := setupGormTestDB(t)
	if err := database.AutoMigrate(&RequestLog{}, &RequestLogContent{}); err != nil {
		t.Fatal(err)
	}
	repo := &gormLogRepo{db: database}
	identity := coreusage.ParseClientIdentity("codex-tui/0.118.0 (Mac OS)")
	entry := &RequestLog{ClientIdentity: identity, Timestamp: time.Now(), Model: "test"}
	if err := repo.Insert(context.Background(), entry, "", "", ""); err != nil {
		t.Fatal(err)
	}
	logs, err := repo.Query(context.Background(), LogQueryParams{Page: 1, Size: 10, Days: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs.Items) != 1 || logs.Items[0].ClientName != "codex" || logs.Items[0].ClientVersion != "0.118.0" || logs.Items[0].HasContent {
		t.Fatalf("列表错误: %#v", logs)
	}
	data, _ := json.Marshal(logs)
	if strings.Contains(string(data), "client_user_agent") {
		t.Fatal("列表不应返回原始 UA")
	}
	managementLogs, err := repo.Query(context.Background(), LogQueryParams{Page: 1, Size: 10, Days: 1, IncludeClientUserAgent: true})
	if err != nil || len(managementLogs.Items) != 1 || managementLogs.Items[0].ClientUserAgent != identity.ClientUserAgent {
		t.Fatalf("管理列表 UA 丢失: %#v %v", managementLogs, err)
	}
	details, err := repo.QueryContentPart(context.Background(), entry.ID, "details")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Client coreusage.ClientIdentity `json:"client"`
	}
	if err = json.Unmarshal([]byte(details.Content), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Client != identity {
		t.Fatalf("详情不匹配: %#v", parsed)
	}
	// 有压缩详情时保留原内容，并使用主表中的真实入站信息。
	second := &RequestLog{ClientIdentity: identity, Timestamp: time.Now(), Model: "test"}
	if err = repo.Insert(context.Background(), second, "", "", `{"client":{"method":"POST"},"upstream":{"request_log":"sample"}}`); err != nil {
		t.Fatal(err)
	}
	details, err = repo.QueryContentPart(context.Background(), second.ID, "details")
	if err != nil || !strings.Contains(details.Content, `"method":"POST"`) || !strings.Contains(details.Content, `"client_name":"codex"`) {
		t.Fatalf("合并详情失败: %s %v", details.Content, err)
	}
}

func TestClientIdentityMigrationPreservesOldRows(t *testing.T) {
	database := setupGormTestDB(t)
	if err := database.Exec("CREATE TABLE request_logs (id INTEGER PRIMARY KEY, model TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO request_logs (id, model) VALUES (1, 'old-model')").Error; err != nil {
		t.Fatal(err)
	}
	// 与 runGORMAutoMigrate 相同的只增加缺失列策略。
	for _, column := range []string{"ClientName", "ClientVariant", "ClientVersion", "ClientUserAgent", "ClientUserAgentTruncated"} {
		if err := database.Migrator().AddColumn(&RequestLog{}, column); err != nil {
			t.Fatal(err)
		}
	}
	var got RequestLog
	if err := database.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.Model != "old-model" || got.ClientIdentity != (coreusage.ClientIdentity{}) {
		t.Fatalf("历史数据错误: %#v", got)
	}
}
