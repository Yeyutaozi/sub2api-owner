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
		"disclaimer":      "根据近期真实请求样本生成的自动切号审计，仅供参考（非主动探测）；进程内保留 24 小时",
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
		},
		"event_types": []string{"sticky_escape", "failover_switch"},
		"disclaimer":  "自动切号审计保留 24 小时；包含用户、来源/目标账号、阈值与候选评分",
	})
}
