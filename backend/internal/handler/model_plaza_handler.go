package handler

import (
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 处理「模型广场」查询。
//
// 广场路由挂 OptionalJWT 中间件：匿名可访问（除非 require_auth 开启），带 token 则
// 识别用户。可见性规则（橱窗语义，与「可用渠道」的可绑定语义不同）：
//   - 匿名：仅非专属分组（订阅型照常展示）；
//   - 登录：非专属分组 + user_allowed_groups 授权的专属分组（不检查订阅有效性）；
//   - 管理员：全部活跃分组（一键获取当前全站开放模型目录）。
type ModelPlazaHandler struct {
	channelService *service.ChannelService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

// NewModelPlazaHandler 创建模型广场 handler。
func NewModelPlazaHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		channelService: channelService,
		apiKeyService:  apiKeyService,
		settingService: settingService,
	}
}

// modelPlazaOfficialPricing LiteLLM 官方参考价（USD per token）。
type modelPlazaOfficialPricing struct {
	InputPrice        *float64 `json:"input_price"`
	OutputPrice       *float64 `json:"output_price"`
	CacheWritePrice   *float64 `json:"cache_write_price"`
	CacheWrite1hPrice *float64 `json:"cache_write_1h_price,omitempty"`
	CacheReadPrice    *float64 `json:"cache_read_price"`
}

// modelPlazaVideoPrices 视频分辨率档位单价（单位由 video_billing_unit 解释）。
type modelPlazaVideoPrices struct {
	Price480P  *float64 `json:"480p,omitempty"`
	Price720P  *float64 `json:"720p,omitempty"`
	Price1080P *float64 `json:"1080p,omitempty"`
	Price1440P *float64 `json:"1440p,omitempty"`
	Price2160P *float64 `json:"2160p,omitempty"`
}

// modelPlazaModel 广场模型条目：渠道定价 + 官方参考价 + 媒体档位价。
type modelPlazaModel struct {
	Name             string                     `json:"name"`
	Platform         string                     `json:"platform"`
	Kind             string                     `json:"kind"` // chat | image | video
	Pricing          *userSupportedModelPricing `json:"pricing"`
	OfficialPricing  *modelPlazaOfficialPricing `json:"official_pricing"`
	VideoBillingUnit string                     `json:"video_billing_unit,omitempty"`
	VideoResolutions []string                   `json:"video_resolutions,omitempty"`
	VideoPrices      *modelPlazaVideoPrices     `json:"video_prices,omitempty"`
	ImagePrices      map[string]*float64        `json:"image_prices,omitempty"`
}

// modelPlazaGroup 广场分组条目（白名单字段）。
type modelPlazaGroup struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Platform           string            `json:"platform"`
	SubscriptionType   string            `json:"subscription_type"`
	RateMultiplier     float64           `json:"rate_multiplier"`
	UserRateMultiplier *float64          `json:"user_rate_multiplier,omitempty"`
	PeakRateEnabled    bool              `json:"peak_rate_enabled"`
	PeakStart          string            `json:"peak_start"`
	PeakEnd            string            `json:"peak_end"`
	PeakRateMultiplier float64           `json:"peak_rate_multiplier"`
	IsExclusive        bool              `json:"is_exclusive"`
	AvgFirstTokenMs    int               `json:"avg_first_token_ms"`
	TTFTDisclaimer     string            `json:"ttft_disclaimer"`
	Models             []modelPlazaModel `json:"models"`
}

// modelPlazaStats 目录汇总（前端页头统计）。
type modelPlazaStats struct {
	Groups int `json:"groups"`
	Models int `json:"models"`
	Offers int `json:"offers"`
}

// modelPlazaResponse 广场页响应。
type modelPlazaResponse struct {
	Description string            `json:"description"`
	SyncedAt    string            `json:"synced_at"`
	IsAdminView bool              `json:"is_admin_view"`
	Stats       modelPlazaStats   `json:"stats"`
	Groups      []modelPlazaGroup `json:"groups"`
}

// Get 返回模型广场数据。
// GET /api/v1/model-plaza
// 数据实时聚合自当前活跃分组开放模型，管理员调用等同「一键获取全站目录」。
func (h *ModelPlazaHandler) Get(c *gin.Context) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}

	role, _ := middleware.GetUserRoleFromContext(c)
	isAdmin := authed && role == service.RoleAdmin

	groups, err := h.channelService.ListPlazaGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// allowedExclusive == nil 表示匿名；登录用户恒为非 nil（可能为空集合）。
	// 管理员跳过专属裁剪，直接拿到全部分组开放模型。
	var allowedExclusive map[int64]struct{}
	var userRates map[int64]float64
	if authed && !isAdmin {
		allowedExclusive, err = h.apiKeyService.GetUserAllowedGroupIDSet(c.Request.Context(), subject.UserID)
		if err != nil {
			// 可见性数据拿不到时不能静默降级成匿名视图（会错漏专属分组），直接报错。
			response.ErrorFrom(c, err)
			return
		}
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			// 专属倍率仅是展示增强，失败降级为分组默认倍率。
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	} else if authed && isAdmin {
		// 管理员仍可展示自己的专属倍率（若有配置）。
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	}

	var visible []service.PlazaGroup
	if isAdmin {
		visible = groups
	} else {
		visible = filterPlazaVisibleGroups(groups, allowedExclusive)
	}

	out := make([]modelPlazaGroup, 0, len(visible))
	uniqueModels := make(map[string]struct{})
	offers := 0
	for i := range visible {
		dto := toModelPlazaGroupDTO(&visible[i], userRates)
		out = append(out, dto)
		for _, m := range dto.Models {
			key := m.Platform + "\x00" + m.Name
			uniqueModels[key] = struct{}{}
			offers++
		}
	}
	response.Success(c, modelPlazaResponse{
		Description: rt.Description,
		SyncedAt:    time.Now().UTC().Format(time.RFC3339),
		IsAdminView: isAdmin,
		Stats: modelPlazaStats{
			Groups: len(out),
			Models: len(uniqueModels),
			Offers: offers,
		},
		Groups: out,
	})
}

// filterPlazaVisibleGroups 按登录态裁剪分组可见性。
// allowedExclusive == nil 表示匿名（仅非专属）；非 nil 表示登录（非专属 + 授权专属）。
func filterPlazaVisibleGroups(
	groups []service.PlazaGroup,
	allowedExclusive map[int64]struct{},
) []service.PlazaGroup {
	visible := make([]service.PlazaGroup, 0, len(groups))
	for _, g := range groups {
		if g.IsExclusive {
			if allowedExclusive == nil {
				continue
			}
			if _, ok := allowedExclusive[g.ID]; !ok {
				continue
			}
		}
		visible = append(visible, g)
	}
	return visible
}

// toModelPlazaGroupDTO 将 service 层广场分组映射为白名单 DTO,并合并用户专属倍率。
func toModelPlazaGroupDTO(g *service.PlazaGroup, userRates map[int64]float64) modelPlazaGroup {
	models := make([]modelPlazaModel, 0, len(g.Models))
	for i := range g.Models {
		m := &g.Models[i]
		kind := m.Kind
		if kind == "" {
			kind = service.PlazaKindChat
		}
		models = append(models, modelPlazaModel{
			Name:             m.Name,
			Platform:         m.Platform,
			Kind:             kind,
			Pricing:          toUserPricing(m.Pricing),
			OfficialPricing:  toModelPlazaOfficialPricing(m.OfficialPricing),
			VideoBillingUnit: m.VideoBillingUnit,
			VideoResolutions: m.VideoResolutions,
			VideoPrices:      toModelPlazaVideoPrices(m.VideoPrices),
			ImagePrices:      m.ImagePrices,
		})
	}
	dto := modelPlazaGroup{
		ID:                 g.ID,
		Name:               g.Name,
		Description:        g.Description,
		Platform:           g.Platform,
		SubscriptionType:   g.SubscriptionType,
		RateMultiplier:     g.RateMultiplier,
		PeakRateEnabled:    g.PeakRateEnabled,
		PeakStart:          g.PeakStart,
		PeakEnd:            g.PeakEnd,
		PeakRateMultiplier: g.PeakRateMultiplier,
		IsExclusive:        g.IsExclusive,
		AvgFirstTokenMs:    g.AvgFirstTokenMs,
		TTFTDisclaimer:     g.TTFTDisclaimer,
		Models:             models,
	}
	if rate, ok := userRates[g.ID]; ok {
		dto.UserRateMultiplier = &rate
	}
	if dto.AvgFirstTokenMs <= 0 || dto.TTFTDisclaimer == "" {
		ttft := service.DefaultGroupTTFTDisplay.GetDisplay(g.ID, g.Platform)
		if dto.AvgFirstTokenMs <= 0 {
			dto.AvgFirstTokenMs = ttft.AvgFirstTokenMs
		}
		if dto.TTFTDisclaimer == "" {
			dto.TTFTDisclaimer = ttft.Disclaimer
		}
	}
	return dto
}

func toModelPlazaVideoPrices(p *service.VideoModelPrice) *modelPlazaVideoPrices {
	if p == nil {
		return nil
	}
	if p.Price480P == nil && p.Price720P == nil && p.Price1080P == nil &&
		p.Price1440P == nil && p.Price2160P == nil {
		// Still return empty object so frontend can show "未配置" tiers for video models.
		return &modelPlazaVideoPrices{}
	}
	return &modelPlazaVideoPrices{
		Price480P:  p.Price480P,
		Price720P:  p.Price720P,
		Price1080P: p.Price1080P,
		Price1440P: p.Price1440P,
		Price2160P: p.Price2160P,
	}
}

// toModelPlazaOfficialPricing 转换官方参考价；nil 透传（前端显示 "-"）。
func toModelPlazaOfficialPricing(p *service.PlazaOfficialPricing) *modelPlazaOfficialPricing {
	if p == nil {
		return nil
	}
	return &modelPlazaOfficialPricing{
		InputPrice:        p.InputPrice,
		OutputPrice:       p.OutputPrice,
		CacheWritePrice:   p.CacheWritePrice,
		CacheWrite1hPrice: p.CacheWrite1hPrice,
		CacheReadPrice:    p.CacheReadPrice,
	}
}
