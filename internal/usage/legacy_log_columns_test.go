package usage

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDropEmptyLegacyLogColumns(t *testing.T) {
	for _, columns := range [][]string{nil, {"input_content"}, {"output_content"}, {"input_content", "output_content"}} {
		t.Run(strings.Join(columns, "+"), func(t *testing.T) {
			database := setupGormTestDB(t)
			if err := database.AutoMigrate(&RequestLog{}, &RequestLogContent{}); err != nil {
				t.Fatal(err)
			}
			for _, column := range columns {
				if err := database.Exec("ALTER TABLE request_logs ADD COLUMN " + column + " TEXT DEFAULT ''").Error; err != nil {
					t.Fatal(err)
				}
			}
			entry := &RequestLog{Timestamp: time.Now(), Model: "test"}
			repo := &gormLogRepo{db: database}
			if err := repo.Insert(context.Background(), entry, "request", "response", ""); err != nil {
				t.Fatal(err)
			}
			before, err := repo.QueryLogStorageBytes(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := dropEmptyLegacyLogContentColumns(database); err != nil {
					t.Fatal(err)
				}
			}
			for _, column := range []string{"input_content", "output_content"} {
				if database.Migrator().HasColumn("request_logs", column) {
					t.Fatalf("旧列仍存在: %s", column)
				}
			}
			details, err := repo.QueryContent(context.Background(), entry.ID)
			if err != nil || details.InputContent != "request" || details.OutputContent != "response" {
				t.Fatalf("正文受影响: %#v %v", details, err)
			}
			logs, err := repo.Query(context.Background(), LogQueryParams{Page: 1, Size: 10, Days: 1})
			if err != nil || len(logs.Items) != 1 || !logs.Items[0].HasContent {
				t.Fatalf("列表受影响: %#v %v", logs, err)
			}
			after, err := repo.QueryLogStorageBytes(context.Background())
			if err != nil || after != before {
				t.Fatalf("存储统计受影响: %d %d %v", before, after, err)
			}
			if !database.Migrator().HasIndex(&RequestLog{}, "idx_logs_timestamp") {
				t.Fatal("索引丢失")
			}
		})
	}
}

func TestDropLegacyLogColumnsRefusesNonemptyData(t *testing.T) {
	for _, column := range []string{"input_content", "output_content"} {
		t.Run(column, func(t *testing.T) {
			database := setupGormTestDB(t)
			if err := database.Exec("CREATE TABLE request_logs (id INTEGER PRIMARY KEY, input_content TEXT DEFAULT '', output_content TEXT DEFAULT '')").Error; err != nil {
				t.Fatal(err)
			}
			if err := database.Exec("INSERT INTO request_logs (id," + column + ") VALUES (1, ' ')").Error; err != nil {
				t.Fatal(err)
			}
			if err := dropEmptyLegacyLogContentColumns(database); err == nil {
				t.Fatal("必须拒绝删除非空历史正文")
			}
			for _, col := range []string{"input_content", "output_content"} {
				if !database.Migrator().HasColumn("request_logs", col) {
					t.Fatalf("旧列被部分删除: %s", col)
				}
			}
		})
	}
}

func TestDropLegacyLogColumnsRollsBackOnDependency(t *testing.T) {
	database := setupGormTestDB(t)
	for _, sql := range []string{
		"CREATE TABLE request_logs (id INTEGER PRIMARY KEY, input_content TEXT DEFAULT '', output_content TEXT DEFAULT '')",
		"CREATE INDEX legacy_output_index ON request_logs(output_content)",
	} {
		if err := database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := dropEmptyLegacyLogContentColumns(database); err == nil {
		t.Fatal("依赖旧列的索引应阻止删除")
	}
	if !database.Migrator().HasColumn("request_logs", "input_content") || !database.Migrator().HasColumn("request_logs", "output_content") {
		t.Fatal("失败时没有回滚两个列")
	}
}

func TestDropLegacyLogColumnsPreservesForeignKeys(t *testing.T) {
	database := setupGormTestDB(t)
	for _, sql := range []string{
		"PRAGMA foreign_keys=ON",
		"CREATE TABLE request_logs (id INTEGER PRIMARY KEY, input_content TEXT, output_content TEXT)",
		"CREATE TABLE request_log_content (log_id INTEGER PRIMARY KEY REFERENCES request_logs(id) ON DELETE CASCADE, input_content BLOB)",
		"INSERT INTO request_logs VALUES (1,NULL,NULL)",
		"INSERT INTO request_log_content VALUES (1,X'010203')",
	} {
		if err := database.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := dropEmptyLegacyLogContentColumns(database); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Table("request_log_content").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("子表数据丢失: %d %v", count, err)
	}
	if err := database.Exec("DELETE FROM request_logs WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Table("request_log_content").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("外键级联丢失: %d %v", count, err)
	}
}
