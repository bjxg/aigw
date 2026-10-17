package usage

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ClientIdentity 是入站客户端的值快照，不依赖请求正文或上游指纹。
type ClientIdentity struct {
	ClientName               string `json:"client_name" gorm:"not null;default:''"`
	ClientVariant            string `json:"client_variant" gorm:"not null;default:''"`
	ClientVersion            string `json:"client_version" gorm:"not null;default:''"`
	ClientUserAgent          string `json:"client_user_agent" gorm:"not null;default:''"`
	ClientUserAgentTruncated bool   `json:"client_user_agent_truncated" gorm:"not null;default:false"`
}

type clientIdentityContextKey struct{}

func WithClientIdentity(ctx context.Context, identity ClientIdentity) context.Context {
	return context.WithValue(ctx, clientIdentityContextKey{}, identity)
}

func ClientIdentityFromContext(ctx context.Context) (ClientIdentity, bool) {
	if ctx == nil {
		return ClientIdentity{}, false
	}
	identity, ok := ctx.Value(clientIdentityContextKey{}).(ClientIdentity)
	return identity, ok
}

// 新增规则必须有可核实的客户端样本，不能将通用 SDK 当作 Agent。
var clientProducts = map[string]string{
	"codex_cli_rs":    "codex",
	"codex-tui":       "codex",
	"claude-cli":      "claudecode",
	"pi":              "pi",
	"pi-coding-agent": "pi",
	"opencode":        "opencode",
	"workbuddy":       "workbuddy",
	"codebuddy":       "codebuddy",
}

// OpenCode v2 将渠道放在版本前，不能按普通 product/version 取第二段。
var openCodeProductPattern = regexp.MustCompile(`(?i)^(opencode)/[a-z0-9._+-]{1,128}/([a-z0-9][a-z0-9._+~-]{0,127})/[a-z0-9._+-]{1,128}$`)

var clientProductPattern = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:/([A-Za-z0-9][A-Za-z0-9._+~-]{0,127}))?$`)

func ParseClientIdentity(userAgent string) ClientIdentity {
	identity, _ := parseClientIdentity(userAgent)
	return identity
}

func parseClientIdentity(userAgent string) (ClientIdentity, bool) {
	identity := ClientIdentity{}
	if len(userAgent) > 4096 {
		end := 4096
		for end > 0 && !utf8.RuneStart(userAgent[end]) {
			end--
		}
		userAgent = userAgent[:end]
		identity.ClientUserAgentTruncated = true
	}
	identity.ClientUserAgent = strings.ToValidUTF8(userAgent, "?")
	// 只匹配产品 token，忽略括号注释，防止终端信息或注释误命中。
	var products strings.Builder
	depth := 0
	escaped := false
	parseUA := identity.ClientUserAgent
	if identity.ClientUserAgentTruncated {
		// 截断位置可能落在版本中间，舍弃最后一个不完整 token。
		if boundary := strings.LastIndexAny(parseUA, " \t"); boundary >= 0 {
			parseUA = parseUA[:boundary]
		} else {
			parseUA = ""
		}
	}
	for _, ch := range parseUA {
		if depth > 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '(' {
				depth++
			}
			if ch == ')' {
				depth--
			}
			continue
		}
		if ch == '(' {
			depth++
			products.WriteByte(' ')
			continue
		}
		products.WriteRune(ch)
	}

	candidates := make(map[string]ClientIdentity)
	ambiguous := make(map[string]bool)
	for _, token := range strings.Fields(products.String()) {
		match := openCodeProductPattern.FindStringSubmatch(token)
		if match == nil {
			match = clientProductPattern.FindStringSubmatch(token)
		}
		if match == nil {
			continue
		}
		name := clientProducts[strings.ToLower(match[1])]
		if name == "" {
			continue
		}
		version := match[2]
		if strings.EqualFold(version, "unknown") {
			version = ""
		}
		candidate := ClientIdentity{ClientName: name, ClientVariant: match[1], ClientVersion: version}
		previous, exists := candidates[name]
		if exists {
			// WorkBuddy 的复合 UA 可重复出现产品名，裸标识不能抹掉已有版本。
			if previous.ClientVersion != "" && version != "" && previous.ClientVersion != version {
				ambiguous[name] = true
			}
			if previous.ClientVersion != "" || version == "" {
				continue
			}
		}
		candidates[name] = candidate
	}
	// WorkBuddy 官方桌面端可嵌入 CodeBuddy CLI，外层产品的版本才是桌面端版本。
	if _, ok := candidates["workbuddy"]; ok {
		delete(candidates, "codebuddy")
	}
	if len(candidates) != 1 {
		return identity, len(candidates) > 1
	}
	for name, candidate := range candidates {
		if ambiguous[name] {
			return identity, true
		}
		identity.ClientName, identity.ClientVariant, identity.ClientVersion = candidate.ClientName, candidate.ClientVariant, candidate.ClientVersion
	}
	return identity, false
}

// ParseClientHeaders 补充已核实的 Pi originator 标识；绝不把模拟 Codex 版本当 Pi 版本。
func ParseClientHeaders(headers http.Header) ClientIdentity {
	identity, ambiguous := parseClientIdentity(headers.Get("User-Agent"))
	if ambiguous {
		return identity
	}
	if headers.Get("Originator") != "pi" {
		return identity
	}
	if identity.ClientName == "pi" {
		return identity
	}
	if identity.ClientName == "" || identity.ClientName == "codex" {
		identity.ClientName, identity.ClientVariant, identity.ClientVersion = "pi", "pi", ""
		return identity
	}
	// 不同独立产品信号冲突时，不猜测。
	identity.ClientName, identity.ClientVariant, identity.ClientVersion = "", "", ""
	return identity
}
