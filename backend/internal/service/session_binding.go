package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrSessionBindingMismatch 会话硬指纹（主要为 User-Agent）发生变化，会话已失效。
// 注意：仅 IP 变更（VPN/代理切换）不会触发该错误，而是软重绑放行。
var ErrSessionBindingMismatch = infraerrors.Unauthorized("SESSION_BINDING_MISMATCH", "session network fingerprint changed, please login again")

// SessionBinding 会话指纹：登录时的客户端 IP 与 User-Agent。
// 会话绑定开启时：
//   - User-Agent 变化 → 硬失效（撤销会话家族）
//   - 仅 IP 变化 → 软重绑（放行，下一轮签发写入新指纹）
// 这样可在防凭证异地盗用（常见 UA 不同）与 VPN/代理切换体验之间取得平衡。
type SessionBinding struct {
	IP        string
	UserAgent string
}

// bindingHashV2Prefix 标记可拆分 IP/UA 的新指纹格式。
const bindingHashV2Prefix = "v2."

// BindingDecision 会话绑定校验结果。
type BindingDecision int

const (
	// BindingMatch 指纹一致（或无需校验）。
	BindingMatch BindingDecision = iota
	// BindingSoftIPChange 仅 IP 变化：放行并在签发新 token 时写入新指纹。
	BindingSoftIPChange
	// BindingHardMismatch UA（或不可接受的指纹）变化：撤销会话。
	BindingHardMismatch
)

func shortBindingHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

// LegacyHash 返回旧版（v1）IP+UA 合并哈希，仅用于兼容历史 token。
func (b *SessionBinding) LegacyHash() string {
	if b == nil {
		return ""
	}
	ip := strings.TrimSpace(b.IP)
	ua := strings.TrimSpace(b.UserAgent)
	if ip == "" && ua == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip + "\n" + ua))
	return hex.EncodeToString(sum[:16])
}

// Hash 计算写入 JWT / refresh token 的会话指纹。
// 新格式：v2.<ipHash>.<uaHash>，支持 IP-only 软重绑。
func (b *SessionBinding) Hash() string {
	if b == nil {
		return ""
	}
	ip := strings.TrimSpace(b.IP)
	ua := strings.TrimSpace(b.UserAgent)
	if ip == "" && ua == "" {
		return ""
	}
	return bindingHashV2Prefix + shortBindingHash(ip) + "." + shortBindingHash(ua)
}

func parseV2BindingHash(stored string) (ipHash, uaHash string, ok bool) {
	if !strings.HasPrefix(stored, bindingHashV2Prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(stored, bindingHashV2Prefix)
	parts := strings.Split(rest, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] == "" && parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// EvaluateSessionBinding 比较已存储指纹与当前请求指纹。
//
// 规则：
//  1. stored 为空 / current 为空哈希 → Match（兼容旧会话）
//  2. v2 格式：UA 不同 → Hard；仅 IP 不同 → SoftIP；都相同 → Match
//  3. v1 旧格式：完整相等 → Match；否则 SoftIP（无法区分 IP/UA，优先保证 VPN 体验，
//     下一轮 refresh 会升级为 v2）
func EvaluateSessionBinding(stored string, current *SessionBinding) BindingDecision {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return BindingMatch
	}
	if current == nil {
		return BindingMatch
	}
	curHash := current.Hash()
	if curHash == "" {
		return BindingMatch
	}
	if stored == curHash {
		return BindingMatch
	}

	if ipHash, uaHash, ok := parseV2BindingHash(stored); ok {
		curIP := shortBindingHash(strings.TrimSpace(current.IP))
		curUA := shortBindingHash(strings.TrimSpace(current.UserAgent))
		if uaHash != curUA {
			return BindingHardMismatch
		}
		if ipHash != curIP {
			return BindingSoftIPChange
		}
		return BindingMatch
	}

	// Legacy v1 combined hash.
	if stored == current.LegacyHash() {
		return BindingMatch
	}
	// 旧格式无法拆分 IP/UA：对不匹配做软放行，避免 VPN 切换强制登出；
	// refresh 时会写入 v2 指纹。
	return BindingSoftIPChange
}

// SessionBindingHardMismatch 是否为需要撤销会话的硬不匹配。
func SessionBindingHardMismatch(stored string, current *SessionBinding) bool {
	return EvaluateSessionBinding(stored, current) == BindingHardMismatch
}

type sessionBindingCtxKey struct{}

// WithSessionBinding 将会话指纹注入 context（由 HTTP 入口中间件调用）。
func WithSessionBinding(ctx context.Context, binding *SessionBinding) context.Context {
	if binding == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionBindingCtxKey{}, binding)
}

// SessionBindingFromContext 从 context 提取会话指纹；不存在时返回 nil。
func SessionBindingFromContext(ctx context.Context) *SessionBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(sessionBindingCtxKey{}).(*SessionBinding)
	return binding
}

// sessionBindingHashFromContext 提取指纹哈希，缺失时返回空串。
func sessionBindingHashFromContext(ctx context.Context) string {
	return SessionBindingFromContext(ctx).Hash()
}
