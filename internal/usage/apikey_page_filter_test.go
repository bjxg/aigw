package usage

import (
	"testing"
)

func TestAPIKeyPageFilters(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	uid := int64(3)
	keys := []APIKeyRow{
		{Key: "sk-alpha", Name: "Alpha Key", UserID: &uid, AllowedChannelGroups: []string{"default", "vip"}},
		{Key: "sk-beta", Name: "Beta Key", Disabled: true, AllowedChannelGroups: []string{"vip"}},
		{Key: "sk-gamma", Name: "Gamma", UserID: nil, AllowedChannelGroups: []string{}},
	}
	for _, k := range keys {
		if err := UpsertAPIKey(k); err != nil {
			t.Fatalf("UpsertAPIKey(%s): %v", k.Key, err)
		}
	}

	// 1. no filter, page through all
	rows, total := ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10})
	if total != 3 || len(rows) != 3 {
		t.Fatalf("no filter: got %d rows, total %d; want 3/3", len(rows), total)
	}

	// 2. search by name
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, Search: "beta"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-beta" {
		t.Fatalf("search name: got rows=%v total=%d, want sk-beta/1", keysOf(rows), total)
	}

	// 3. search by key
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, Search: "sk-gamma"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-gamma" {
		t.Fatalf("search key: got rows=%v total=%d, want sk-gamma/1", keysOf(rows), total)
	}

	// 4. search combined with status (OR-in-search + AND status)
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, Search: "key", Status: "enabled"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-alpha" {
		t.Fatalf("search+status: got rows=%v total=%d, want sk-alpha/1", keysOf(rows), total)
	}

	// 5. status disabled
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, Status: "disabled"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-beta" {
		t.Fatalf("status disabled: got rows=%v total=%d, want sk-beta/1", keysOf(rows), total)
	}

	// 6. user filter (assigned)
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, UserID: 3})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-alpha" {
		t.Fatalf("user 3: got rows=%v total=%d, want sk-alpha/1", keysOf(rows), total)
	}

	// 7. user filter (unassigned = -1)
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, UserID: -1})
	if total != 2 || len(rows) != 2 {
		t.Fatalf("unassigned: got rows=%v total=%d, want 2 rows (sk-beta, sk-gamma)", keysOf(rows), total)
	}

	// 8. channel group
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, ChannelGroup: "vip"})
	if total != 2 || len(rows) != 2 {
		t.Fatalf("group vip: got rows=%v total=%d, want 2 rows", keysOf(rows), total)
	}
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, ChannelGroup: "default"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-alpha" {
		t.Fatalf("group default: got rows=%v total=%d, want sk-alpha/1", keysOf(rows), total)
	}

	// 9. pagination
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 2, PageSize: 2})
	if total != 3 || len(rows) != 1 {
		t.Fatalf("page 2 size 2: got %d rows total %d, want 1 row total 3", len(rows), total)
	}

	// 10. combined user + group
	rows, total = ListAPIKeysPage(APIKeyPageFilter{Page: 1, PageSize: 10, UserID: 3, ChannelGroup: "vip"})
	if total != 1 || len(rows) != 1 || rows[0].Key != "sk-alpha" {
		t.Fatalf("user+group: got rows=%v total=%d, want sk-alpha/1", keysOf(rows), total)
	}
}

func keysOf(rows []APIKeyRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Key)
	}
	return out
}
