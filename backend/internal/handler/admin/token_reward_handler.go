package admin

import (
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
	claims, total, err := h.tokenRewardService.ListAllClaimHistory(c.Request.Context(), page, pageSize)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, claims, total, page, pageSize)
}
