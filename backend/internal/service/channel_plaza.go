package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Plaza model kinds for showcase cards.
const (
	PlazaKindChat  = "chat"
	PlazaKindImage = "image"
	PlazaKindVideo = "video"
)

// PlazaOfficialPricing 模型广场展示用的 LiteLLM 官方参考价（USD per token）。
// 字段为 nil 表示官方数据中该项缺失（0 视为未配置）。
type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入（LiteLLM cache_creation_above_1hr）
	CacheReadPrice    *float64
}

// PlazaModel 模型广场中单个模型条目：渠道定价 + 官方参考价 + 媒体（图/视频）分组价。
type PlazaModel struct {
	Name            string
	Platform        string
	Kind            string // chat | image | video
	Pricing         *ChannelModelPricing
	OfficialPricing *PlazaOfficialPricing
	// Video-specific (group-level matrix / legacy resolution prices).
	VideoBillingUnit string
	// Allowed video resolution keys for this model (even if unpriced).
	VideoResolutions []string
	VideoPrices      *VideoModelPrice
	// Image-specific (group-level 1K/2K/4K when no channel pricing intervals).
	ImagePrices map[string]*float64
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型（按分组平台隔离，防跨平台泄漏）+ 分组开放的图/视频模型。
type PlazaGroup struct {
	ID                 int64
	Name               string
	Description        string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
	// AvgFirstTokenMs is always populated for user-facing cards (baseline if cold).
	AvgFirstTokenMs int
	// TTFTDisclaimer is shown next to the latency figure (never sample=0 messaging).
	TTFTDisclaimer string
	Models         []PlazaModel
}

// ListPlazaGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 聚合口径与 ListAvailable 一致（Active 渠道、SupportedModels ∪ 全局定价回落、
// 平台隔离），并补充：
//   - FFLink 视频平台：分组开放模型 + 分辨率档位价 + 计费单位（秒/条）
//   - 允许生图的分组：开放图片模型 + 1K/2K/4K 价
//   - 同分组同名模型「先见者胜」，渠道无价可被有价升级；媒体模型补齐 Kind/价卡
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称）
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ChannelService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})

	sourceGroups := make(map[int64]*Group, len(groups))
	byGroup := make(map[int64]*PlazaGroup, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		sourceGroups[g.ID] = g
		byGroup[g.ID] = &PlazaGroup{
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
		}
		order = append(order, g.ID)
	}

	// modelIdx[groupID][modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[string]int, len(groups))
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()
		s.fillGlobalPricingFallback(supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			idx := modelIdx[gid]
			if idx == nil {
				idx = make(map[string]int, len(supported))
				modelIdx[gid] = idx
			}
			for j := range supported {
				m := supported[j]
				if m.Platform != pg.Platform {
					continue
				}
				if at, seen := idx[m.Name]; seen {
					// 先见者胜；仅当已存条目无定价而新条目有定价时升级。
					if pg.Models[at].Pricing == nil && m.Pricing != nil {
						pg.Models[at].Pricing = m.Pricing
						pg.Models[at].Kind = plazaKindFromPricing(m.Pricing)
					}
					continue
				}
				idx[m.Name] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:     m.Name,
					Platform: m.Platform,
					Kind:     plazaKindFromPricing(m.Pricing),
					Pricing:  m.Pricing,
				})
			}
		}
	}

	// Inject open media models from groups (video matrix / image tier prices).
	for _, gid := range order {
		pg := byGroup[gid]
		src := sourceGroups[gid]
		idx := modelIdx[gid]
		if idx == nil {
			idx = make(map[string]int)
			modelIdx[gid] = idx
		}
		appendPlazaMediaModels(pg, src, idx)
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Kind != pg.Models[j].Kind {
				return plazaKindRank(pg.Models[i].Kind) < plazaKindRank(pg.Models[j].Kind)
			}
			return pg.Models[i].Name < pg.Models[j].Name
		})
		for j := range pg.Models {
			if pg.Models[j].Kind == "" {
				pg.Models[j].Kind = PlazaKindChat
			}
			if pg.Models[j].Kind == PlazaKindChat || pg.Models[j].Kind == PlazaKindImage {
				pg.Models[j].OfficialPricing = s.lookupOfficialPricing(pg.Models[j].Name, officialMemo)
			}
		}
		ttft := DefaultGroupTTFTDisplay.GetDisplay(pg.ID, pg.Platform)
		pg.AvgFirstTokenMs = ttft.AvgFirstTokenMs
		pg.TTFTDisclaimer = ttft.Disclaimer
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func plazaKindFromPricing(p *ChannelModelPricing) string {
	if p == nil {
		return PlazaKindChat
	}
	switch p.BillingMode {
	case BillingModeImage:
		return PlazaKindImage
	case BillingModeVideo:
		return PlazaKindVideo
	default:
		return PlazaKindChat
	}
}

func plazaKindRank(kind string) int {
	switch kind {
	case PlazaKindVideo:
		return 0
	case PlazaKindImage:
		return 1
	default:
		return 2
	}
}

// appendPlazaMediaModels adds group-open video/image models that are not already
// present from channel SupportedModels.
func appendPlazaMediaModels(pg *PlazaGroup, g *Group, idx map[string]int) {
	if pg == nil || g == nil || idx == nil {
		return
	}

	if IsFFLinkVideoPlatform(g.Platform) {
		unit := g.EffectiveVideoBillingUnit()
		for _, modelID := range plazaOpenVideoModelIDs(g) {
			prices := plazaVideoPricesForModel(g, modelID)
			resolutions := plazaVideoResolutionsForModel(modelID)
			if at, seen := idx[modelID]; seen {
				// Enrich channel-derived entry with video price card.
				if pg.Models[at].Kind == "" || pg.Models[at].Kind == PlazaKindChat {
					pg.Models[at].Kind = PlazaKindVideo
				}
				if pg.Models[at].VideoBillingUnit == "" {
					pg.Models[at].VideoBillingUnit = unit
				}
				if len(pg.Models[at].VideoResolutions) == 0 {
					pg.Models[at].VideoResolutions = resolutions
				}
				if pg.Models[at].VideoPrices == nil {
					pg.Models[at].VideoPrices = prices
				}
				continue
			}
			idx[modelID] = len(pg.Models)
			pg.Models = append(pg.Models, PlazaModel{
				Name:             modelID,
				Platform:         g.Platform,
				Kind:             PlazaKindVideo,
				VideoBillingUnit: unit,
				VideoResolutions: resolutions,
				VideoPrices:      prices,
			})
		}
	}

	if g.AllowImageGeneration {
		for _, modelID := range plazaOpenImageModelIDs(g) {
			imagePrices := plazaImagePricesForGroup(g)
			if at, seen := idx[modelID]; seen {
				if pg.Models[at].Kind == PlazaKindChat {
					// Keep chat kind if it has token pricing; still attach image tiers.
					if pg.Models[at].Pricing == nil || pg.Models[at].Pricing.BillingMode == BillingModeImage {
						pg.Models[at].Kind = PlazaKindImage
					}
				}
				if pg.Models[at].ImagePrices == nil {
					pg.Models[at].ImagePrices = imagePrices
				}
				continue
			}
			idx[modelID] = len(pg.Models)
			pg.Models = append(pg.Models, PlazaModel{
				Name:        modelID,
				Platform:    g.Platform,
				Kind:        PlazaKindImage,
				ImagePrices: imagePrices,
			})
		}
	}
}

// plazaOpenVideoModelIDs returns models the group currently exposes for video generation.
// Custom models_list (when enabled) is authoritative; otherwise platform defaults.
func plazaOpenVideoModelIDs(g *Group) []string {
	if g == nil || !IsFFLinkVideoPlatform(g.Platform) {
		return nil
	}
	defaults := FFLinkVideoModelIDsForPlatform(g.Platform)
	if !g.CustomModelsListEnabled() {
		return defaults
	}
	allowed := make(map[string]struct{}, len(defaults))
	for _, id := range defaults {
		allowed[strings.ToLower(id)] = struct{}{}
	}
	out := make([]string, 0, len(g.ModelsListConfig.Models))
	seen := make(map[string]struct{}, len(g.ModelsListConfig.Models))
	for _, raw := range g.ModelsListConfig.Models {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := allowed[id]; !ok {
			// Still include if profile matches this platform (covers newly mapped IDs).
			if profile, ok := ffLinkVideoModelProfileFor(id); !ok || profile.Platform != g.Platform {
				continue
			}
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// plazaOpenImageModelIDs returns image models the group currently exposes.
func plazaOpenImageModelIDs(g *Group) []string {
	if g == nil || !g.AllowImageGeneration {
		return nil
	}
	defaults := defaultCreazyCanvasImageModels(g.Platform)
	if !g.CustomModelsListEnabled() {
		return defaults
	}
	// Prefer intersection with known image models; fall back to custom list entries
	// that look like image models when defaults are empty for the platform.
	defaultSet := make(map[string]struct{}, len(defaults))
	for _, id := range defaults {
		defaultSet[strings.ToLower(id)] = struct{}{}
	}
	out := make([]string, 0, len(g.ModelsListConfig.Models))
	seen := make(map[string]struct{})
	for _, raw := range g.ModelsListConfig.Models {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if len(defaultSet) > 0 {
			if _, ok := defaultSet[id]; !ok {
				continue
			}
		} else if !looksLikeImageModelID(id) {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return defaults
	}
	return out
}

func looksLikeImageModelID(id string) bool {
	id = strings.ToLower(id)
	return strings.Contains(id, "image") || strings.Contains(id, "imagine") || strings.Contains(id, "dall")
}

func plazaVideoPricesForModel(g *Group, modelID string) *VideoModelPrice {
	if g == nil {
		return nil
	}
	profile, ok := ffLinkVideoModelProfileFor(modelID)
	prices := &VideoModelPrice{}
	hasAny := false
	set := func(res string, dst **float64) {
		p := g.GetVideoPriceForModel(modelID, res)
		if p != nil {
			*dst = p
			hasAny = true
		}
	}
	if ok {
		for res := range profile.AllowedResolutions {
			switch res {
			case VideoBillingResolution480P:
				set(res, &prices.Price480P)
			case VideoBillingResolution720P:
				set(res, &prices.Price720P)
			case VideoBillingResolution1080P:
				set(res, &prices.Price1080P)
			case VideoBillingResolution1440P:
				set(res, &prices.Price1440P)
			case VideoBillingResolution2160P:
				set(res, &prices.Price2160P)
			}
		}
	} else {
		set(VideoBillingResolution480P, &prices.Price480P)
		set(VideoBillingResolution720P, &prices.Price720P)
		set(VideoBillingResolution1080P, &prices.Price1080P)
		set(VideoBillingResolution1440P, &prices.Price1440P)
		set(VideoBillingResolution2160P, &prices.Price2160P)
	}
	if !hasAny {
		return &VideoModelPrice{}
	}
	return prices
}

func plazaVideoResolutionsForModel(modelID string) []string {
	profile, ok := ffLinkVideoModelProfileFor(modelID)
	if !ok || len(profile.AllowedResolutions) == 0 {
		return nil
	}
	return sortedStringKeys(profile.AllowedResolutions)
}

func plazaImagePricesForGroup(g *Group) map[string]*float64 {
	if g == nil {
		return nil
	}
	out := map[string]*float64{
		"1K": g.GetImagePrice("1K"),
		"2K": g.GetImagePrice("2K"),
		"4K": g.GetImagePrice("4K"),
	}
	// Drop entirely-null map for cleaner JSON (all nil still useful for UI "未配置").
	return out
}

// lookupOfficialPricing 查询模型的 LiteLLM 官方参考价，带 memo 避免同名模型重复转换。
// pricingService 为 nil（测试场景）或查不到时返回 nil。
func (s *ChannelService) lookupOfficialPricing(modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if s.pricingService == nil {
		return nil
	}
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if lp := s.pricingService.GetModelPricing(modelName); lp != nil && !lp.TokenPricingAbsent {
		result = &PlazaOfficialPricing{
			InputPrice:        nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:       nonZeroPtr(lp.OutputCostPerToken),
			CacheWritePrice:   nonZeroPtr(lp.CacheCreationInputTokenCost),
			CacheWrite1hPrice: nonZeroPtr(lp.CacheCreationInputTokenCostAbove1hr),
			CacheReadPrice:    nonZeroPtr(lp.CacheReadInputTokenCost),
		}
		if result.InputPrice == nil && result.OutputPrice == nil &&
			result.CacheWritePrice == nil && result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil {
			result = nil
		}
	}
	memo[modelName] = result
	return result
}
