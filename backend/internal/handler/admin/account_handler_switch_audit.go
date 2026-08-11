package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ListAccountSwitchAudit returns recent automatic account-switch decisions for admin audit.
// Retention is 24h (process-local, no probe cost).
func (h *AccountHandler) ListAccountSwitchAudit(c *gin.Context) {
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	groupID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("group_id")), 10, 64)
	accountID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("account_id")), 10, 64)
	reason := strings.TrimSpace(c.Query("reason"))

	events := service.ListAccountSwitchAudit(limit, userID, groupID, accountID, reason)
	if events == nil {
		events = []service.AccountSwitchAuditEvent{}
	}

	response.Success(c, gin.H{
		"retention_hours": service.AccountSwitchAuditRetentionHours(),
		"total":           len(events),
		"items":           events,
		"disclaimer":      "根据近期真实请求样本生成的自动切号审计，仅供参考（非主动探测）。含 sticky 逃逸与上游失败切号；Redis 共享保留 24 小时（无 Redis 时进程内存）",
	})
}

// ListAccountSwitchAuditMeta returns filter metadata for the switch-audit UI.
func (h *AccountHandler) ListAccountSwitchAuditMeta(c *gin.Context) {
	response.Success(c, gin.H{
		"retention_hours": service.AccountSwitchAuditRetentionHours(),
		"reasons": []string{
			"ttft",
			"ttft_relative",
			"error_rate",
			"concurrency_full",
			"safe_rate",
			"upstream_failover",
			"upstream_429",
			"upstream_500",
			"upstream_502",
			"upstream_503",
		},
		"event_types": []string{"sticky_escape", "failover_switch"},
		"disclaimer":  "自动切号审计保留 24 小时（Redis 共享优先）；含 sticky_escape 与 failover_switch；同账号临时重试不记入",
	})
}
