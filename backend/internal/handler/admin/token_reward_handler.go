package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type TokenRewardHandler struct {
	tokenRewardService *service.TokenRewardService
}

func NewTokenRewardHandler(tokenRewardService *service.TokenRewardService) *TokenRewardHandler {
	return &TokenRewardHandler{tokenRewardService: tokenRewardService}
}

func (h *TokenRewardHandler) GetConfig(c *gin.Context) {
	cfg, err := h.tokenRewardService.GetConfig(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, cfg)
}

func (h *TokenRewardHandler) UpdateConfig(c *gin.Context) {
	var cfg service.TokenRewardConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	updated, err := h.tokenRewardService.UpdateConfig(c.Request.Context(), cfg)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, updated)
}

func (h *TokenRewardHandler) ListClaims(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.TokenRewardAdminClaimFilter{
		Search:    strings.TrimSpace(c.Query("search")),
		TierID:    strings.TrimSpace(c.Query("tier_id")),
		CycleType: strings.TrimSpace(c.Query("cycle_type")),
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = userID
	}
	if raw := firstNonEmptyQuery(c, "claimed_from", "from"); raw != "" {
		claimedFrom, _, err := parseTokenRewardClaimTime(raw)
		if err != nil {
			response.BadRequest(c, "Invalid claimed_from")
			return
		}
		filter.ClaimedFrom = &claimedFrom
	}
	if raw := firstNonEmptyQuery(c, "claimed_to", "to"); raw != "" {
		claimedTo, dateOnly, err := parseTokenRewardClaimTime(raw)
		if err != nil {
			response.BadRequest(c, "Invalid claimed_to")
			return
		}
		if dateOnly {
			claimedTo = claimedTo.Add(24*time.Hour - time.Nanosecond)
		}
		filter.ClaimedTo = &claimedTo
	}
	result, err := h.tokenRewardService.ListAllClaimHistory(c.Request.Context(), filter, page, pageSize)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func firstNonEmptyQuery(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if raw := strings.TrimSpace(c.Query(key)); raw != "" {
			return raw
		}
	}
	return ""
}

func parseTokenRewardClaimTime(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	if t, err := time.Parse("2006-01-02T15:04", raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
