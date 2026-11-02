package admin

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GroupHandler) isSimpleMode() bool {
	return h != nil && h.cfg != nil && h.cfg.RunMode == config.RunModeSimple
}

type simpleModeGroupOperation string

const (
	simpleModeGroupList   simpleModeGroupOperation = "list"
	simpleModeGroupGetAll simpleModeGroupOperation = "get_all"
	simpleModeGroupGet    simpleModeGroupOperation = "get"
	simpleModeGroupCreate simpleModeGroupOperation = "create"
	simpleModeGroupUpdate simpleModeGroupOperation = "update"
	simpleModeGroupDelete simpleModeGroupOperation = "delete"
)

var simpleModeGroupOperations = map[simpleModeGroupOperation]struct{}{
	simpleModeGroupList: {}, simpleModeGroupGetAll: {}, simpleModeGroupGet: {},
	simpleModeGroupCreate: {}, simpleModeGroupUpdate: {}, simpleModeGroupDelete: {},
}

func (h *GroupHandler) rejectUnsupportedSimpleModeOperation(c *gin.Context, operation simpleModeGroupOperation) bool {
	if _, allowed := simpleModeGroupOperations[operation]; allowed {
		return false
	}
	if err := service.ValidateSimpleModeGroupOperation(h.cfg, service.AdminGroupOperation(operation)); err != nil {
		response.ErrorFrom(c, err)
		return true
	}
	return false
}

type simpleModeGroupResponse struct {
	ID                      int64             `json:"id"`
	Name                    string            `json:"name"`
	Description             string            `json:"description"`
	Platform                string            `json:"platform"`
	Status                  string            `json:"status"`
	I18n                    service.GroupI18n `json:"i18n,omitempty"`
	AccountCount            int64             `json:"account_count,omitempty"`
	ActiveAccountCount      int64             `json:"active_account_count,omitempty"`
	RateLimitedAccountCount int64             `json:"rate_limited_account_count,omitempty"`
	SortOrder               int               `json:"sort_order"`
	CreatedAt               time.Time         `json:"created_at"`
	UpdatedAt               time.Time         `json:"updated_at"`
}

func groupForSimpleMode(group *service.Group) *simpleModeGroupResponse {
	if group == nil {
		return nil
	}
	return &simpleModeGroupResponse{
		ID: group.ID, Name: group.Name, Description: group.Description, Platform: group.Platform,
		Status: group.Status, I18n: group.I18n, AccountCount: group.AccountCount,
		ActiveAccountCount: group.ActiveAccountCount, RateLimitedAccountCount: group.RateLimitedAccountCount,
		SortOrder: group.SortOrder, CreatedAt: group.CreatedAt, UpdatedAt: group.UpdatedAt,
	}
}

func sanitizeCreateGroupRequestForSimpleMode(req *CreateGroupRequest) {
	if req == nil {
		return
	}
	allowed := CreateGroupRequest{Name: req.Name, Description: req.Description, I18n: req.I18n, Platform: req.Platform}
	allowed.RateMultiplier = 1
	allowed.SubscriptionType = service.SubscriptionTypeStandard
	*req = allowed
}

func sanitizeUpdateGroupRequestForSimpleMode(req *UpdateGroupRequest) {
	if req == nil {
		return
	}
	*req = UpdateGroupRequest{Name: req.Name, Description: req.Description, I18n: req.I18n}
}
