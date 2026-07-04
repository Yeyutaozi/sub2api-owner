package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type TokenRewardHandler struct {
	tokenRewardService *service.TokenRewardService
}

func NewTokenRewardHandler(tokenRewardService *service.TokenRewardService) *TokenRewardHandler {
	return &TokenRewardHandler{tokenRewardService: tokenRewardService}
}

type claimTokenRewardRequest struct {
	TierID string `json:"tier_id"`
}

func (h *TokenRewardHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	status, err := h.tokenRewardService.GetStatus(c.Request.Context(), subject.UserID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, status)
}

func (h *TokenRewardHandler) ListClaims(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	claims, total, err := h.tokenRewardService.ListClaimHistory(c.Request.Context(), subject.UserID, page, pageSize)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, claims, total, page, pageSize)
}

func (h *TokenRewardHandler) Claim(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req claimTokenRewardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	tierID := strings.TrimSpace(req.TierID)
	if tierID == "" {
		response.BadRequest(c, "tier_id is required")
		return
	}
	result, err := h.tokenRewardService.Claim(c.Request.Context(), subject.UserID, tierID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
