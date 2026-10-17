package usage

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseClientIdentity(t *testing.T) {
	for _, tc := range []struct{ ua, name, variant, version string }{
		{"codex_cli_rs/0.125.0", "codex", "codex_cli_rs", "0.125.0"},
		{"codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)", "codex", "codex-tui", "0.118.0"},
		{"claude-cli/2.1.88 (external, cli)", "claudecode", "claude-cli", "2.1.88"},
		{"claude-cli/2.2.0-beta.1+abc (external, sdk-cli)", "claudecode", "claude-cli", "2.2.0-beta.1+abc"},
		{"Claude-CLI", "claudecode", "Claude-CLI", ""},
		{"node/22 claude-cli", "claudecode", "claude-cli", ""},

		// Pi 官方 ai adapter 不带版本；管理请求的 pi/version 只解析实际出现的值。
		{"pi (darwin 25.0; arm64)", "pi", "pi", ""},
		{"pi (browser)", "pi", "pi", ""},
		{"pi/0.70.0 (darwin; node/v24.0.0; arm64)", "pi", "pi", "0.70.0"},
		{"pi-coding-agent", "pi", "pi-coding-agent", ""},
		{"opencode/1.2.3", "opencode", "opencode", "1.2.3"},
		{"opencode/latest/2.0.0/cli", "opencode", "opencode", "2.0.0"},
		{"opencode/beta/2.0.0-beta.1/desktop", "opencode", "opencode", "2.0.0-beta.1"},
		{"opencode/unknown/unknown/opencode", "opencode", "opencode", ""},
		{"opencode/latest/2.0.0/cli gitlab-ai-provider/3.0 (linux)", "opencode", "opencode", "2.0.0"},
		{"WorkBuddy/5.8.3 WorkBuddy/5.8.3 CLI/2.168.2", "workbuddy", "WorkBuddy", "5.8.3"},
		{"WorkBuddy/5.5.2 WorkBuddy AI/5.5.2 CLI/5.5.2", "workbuddy", "WorkBuddy", "5.5.2"},
		{"WorkBuddy/5.8.3 CodeBuddy/2.168.2", "workbuddy", "WorkBuddy", "5.8.3"},
		{"CLI/unknown CodeBuddy/2.136.0", "codebuddy", "CodeBuddy", "2.136.0"},
		{"WorkBuddy/unknown CLI/2.168.2", "workbuddy", "WorkBuddy", ""},
		{"pi/1.2.3 opencode/2.0.0", "", "", ""},
		{"WorkBuddy/5.8.3 WorkBuddy/5.8.4", "", "", ""},
		{"not-pi/1.0", "", "", ""},
		{"Mozilla/5.0 (pi/1.0 WorkBuddy/5.8.3)", "", "", ""},
		{"opencode/latest/2.0.0/cli/extra", "", "", ""},
		{"opencode/latest//cli", "", "", ""},
		{"", "", "", ""},
		{"OpenAI/JS 4.0", "", "", ""},
		{"node/22", "", "", ""},
		{"future-agent/1.0", "", "", ""},
		{"not-codex-tui/1.2", "", "", ""},
		{"Mozilla/5.0 (compatible; claude-cli/2.1.88)", "", "", ""},
		{"codex_cli_rs/1.0 claude-cli/2.0", "", "", ""},
		{"codex_cli_rs/1.0 codex_cli_rs/2.0", "", "", ""},
		{"claude-cli/", "", "", ""},
	} {
		t.Run(tc.ua, func(t *testing.T) {
			got := ParseClientIdentity(tc.ua)
			if got.ClientName != tc.name || got.ClientVariant != tc.variant || got.ClientVersion != tc.version {
				t.Fatalf("got %#v", got)
			}
			if got.ClientUserAgent != tc.ua || got.ClientUserAgentTruncated {
				t.Fatal("原始 UA 未保留")
			}
		})
	}
}

func TestClientIdentityBounds(t *testing.T) {
	ua := "codex-tui/1.2 (" + strings.Repeat("界", 2000) + ")"
	got := ParseClientIdentity(ua)
	if !got.ClientUserAgentTruncated || len(got.ClientUserAgent) > 4096 || !utf8.ValidString(got.ClientUserAgent) || got.ClientVersion != "1.2" {
		t.Fatalf("截断结果错误: %#v", got)
	}
	exact := strings.Repeat("a", 4096)
	if ParseClientIdentity(exact).ClientUserAgentTruncated {
		t.Fatal("不应截断边界长度")
	}
	invalid := ParseClientIdentity("future/1 \xff")
	if !utf8.ValidString(invalid.ClientUserAgent) {
		t.Fatal("数据库字符串必须是合法 UTF-8")
	}
}

func TestParseClientHeadersPiOriginator(t *testing.T) {
	for _, tc := range []struct{ ua, originator, name, version string }{
		{"pi (linux; x64)", "pi", "pi", ""},
		{"pi/0.70.0 (linux; node/v24)", "pi", "pi", "0.70.0"},
		{"codex_cli_rs/0.125.0", "pi", "pi", ""},
		{"OpenAI/JS 4.0", "pi", "pi", ""},
		{"claude-cli/2.1.88", "pi", "", ""},
		{"pi/1.0 opencode/2.0", "pi", "", ""},
		{"codex_cli_rs/1.0 codex_cli_rs/2.0", "pi", "", ""},
		{"codex_cli_rs/0.125.0", "other", "codex", "0.125.0"},
	} {
		headers := http.Header{}
		headers.Set("User-Agent", tc.ua)
		headers.Set("Originator", tc.originator)
		got := ParseClientHeaders(headers)
		if got.ClientName != tc.name || got.ClientVersion != tc.version || got.ClientUserAgent != tc.ua {
			t.Fatalf("%#v: %#v", tc, got)
		}
	}
}
