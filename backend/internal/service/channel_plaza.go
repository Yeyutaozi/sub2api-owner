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

// plazaPricedModel is channel pricing overlay for a model name under a group.
type plazaPricedModel struct {
	platform string
	pricing  *ChannelModelPricing
}

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
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 以分组内可调度账号实际可承接的模型为准
// （账号 model_mapping / IsModelSupported），渠道定价仅作价卡叠加；再补齐图/视频元数据。
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
// 归属口径（与 GetAvailableModels / 真实账号承接能力对齐）：
//   - 模型成员资格以「分组内可调度账号」为准（ListSchedulableByGroupID + model_mapping）
//   - 只要分组内存在声明了 model_mapping 的账号：仅展示映射键并集（含通配展开），
//     不再用渠道目录/分组开放图视频默认去「发明」账号未开启的模型
//   - 仅当全部分组账号均为空 mapping / 透传时：才展开开放目录（渠道价卡 ∪ 平台默认 ∪ 开放媒体）
//   - 最终再经 IsModelSupported 过滤，保证至少一账号能承接
//   - 分组启用自定义 models_list 时：再与 models_list 求交
//   - 渠道 SupportedModels 只提供定价叠加，不再单独决定「分组有哪些模型」
//   - 图/视频价卡仍按分组配置补齐 Kind/分辨率/档位（不新增无账号支持的模型）
//   - accountSource 未注入时回退旧的渠道+分组目录（仅测试兼容）
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

	// channelPricing[groupID][modelName] = best channel pricing entry (for overlay only).
	channelPricing := make(map[int64]map[string]plazaPricedModel, len(groups))
	channelModelNames := make(map[int64][]string, len(groups))
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
			pm := channelPricing[gid]
			if pm == nil {
				pm = make(map[string]plazaPricedModel)
				channelPricing[gid] = pm
			}
			for j := range supported {
				m := supported[j]
				if m.Platform != pg.Platform {
					continue
				}
				name := strings.TrimSpace(m.Name)
				if name == "" {
					continue
				}
				if existing, seen := pm[name]; seen {
					if existing.pricing == nil && m.Pricing != nil {
						pm[name] = plazaPricedModel{platform: m.Platform, pricing: m.Pricing}
					}
					continue
				}
				pm[name] = plazaPricedModel{platform: m.Platform, pricing: m.Pricing}
				channelModelNames[gid] = append(channelModelNames[gid], name)
			}
		}
	}

	// modelIdx[groupID][modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[string]int, len(groups))

	// Production path: account-based membership. Legacy path when accountSource unset.
	if s != nil && s.accountSource != nil {
		for _, gid := range order {
			pg := byGroup[gid]
			src := sourceGroups[gid]
			if src == nil {
				continue
			}
			accounts, aerr := s.accountSource.ListSchedulableByGroupID(ctx, gid)
			if aerr != nil {
				return nil, fmt.Errorf("list schedulable accounts for group %d: %w", gid, aerr)
			}
			groupAccounts := plazaFilterAccountsForGroup(accounts, src.Platform)
			if len(groupAccounts) == 0 {
				// No schedulable account ⇒ group cannot serve any model.
				continue
			}

			candidates := plazaCollectCandidateModels(src, groupAccounts, channelModelNames[gid])
			if src.CustomModelsListEnabled() {
				candidates = plazaIntersectModelsList(candidates, src.ModelsListConfig.Models)
			}

			idx := modelIdx[gid]
			if idx == nil {
				idx = make(map[string]int)
				modelIdx[gid] = idx
			}
			for _, modelID := range candidates {
				if !plazaAnyAccountSupportsModel(groupAccounts, modelID) {
					continue
				}
				plazaUpsertModel(pg, idx, modelID, src.Platform, channelPricing[gid])
			}
			// Enrich kind/price only — do not invent media models beyond account mappings.
			// Open-media defaults enter earlier via plazaCollectCandidateModels when
			// (and only when) every account is unrestricted.
			appendPlazaMediaModelsOpt(pg, src, idx, false)
			plazaFilterUnsupportedModels(pg, idx, groupAccounts)
		}
	} else {
		// Legacy (tests without accountSource): channel SupportedModels + group media.
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
	}

	// Cross-group / LiteLLM pricing fill: models may appear via account support without
	// channel pricing overlay. Reuse same-model base pricing from any group first, then
	// LiteLLM synthesize, so plaza cards don't show empty prices for valid offers.
	s.fillPlazaMissingPricing(byGroup, order)

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
		// Rebuild index after sort for stability of later ops (none currently).
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

// plazaFilterAccountsForGroup keeps schedulable-list accounts that can serve the group platform.
func plazaFilterAccountsForGroup(accounts []Account, groupPlatform string) []Account {
	if len(accounts) == 0 {
		return nil
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if groupPlatform == PlatformComposite {
			if !isConcreteRequestPlatform(acc.Platform) {
				continue
			}
			out = append(out, *acc)
			continue
		}
		if strings.TrimSpace(acc.Platform) != strings.TrimSpace(groupPlatform) {
			continue
		}
		out = append(out, *acc)
	}
	return out
}

// plazaCollectCandidateModels builds the candidate set before IsModelSupported filtering.
//
// Membership mirrors gateway GetAvailableModels:
//   - If any schedulable account declares a non-empty model_mapping (and is not pure
//     OpenAI passthrough), the plaza only exposes the union of those mapping keys
//     (plus wildcard expansions against known catalogs). Channel pricing and group
//     open-media defaults MUST NOT invent models the accounts never enabled.
//   - If every account is unrestricted (empty mapping / passthrough), expand the
//     open catalog: channel-priced names ∪ platform defaults ∪ group-open media.
func plazaCollectCandidateModels(g *Group, accounts []Account, channelModels []string) []string {
	seen := make(map[string]struct{})
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if !GroupAllowsVideoModelExposure(g, id) {
			return
		}
		// Skip pure wildcard patterns as display names.
		if strings.Contains(id, "*") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
	}

	hasAnyMapping := false
	hasUnrestricted := false
	for i := range accounts {
		acc := &accounts[i]
		// Pure passthrough: model semantics belong to upstream; treat as unrestricted
		// for catalog expansion only when no mapped account exists in the group.
		if acc.IsOpenAIPassthroughEnabled() {
			hasUnrestricted = true
			continue
		}
		mapping := acc.GetModelMapping()
		if len(mapping) == 0 {
			// Empty mapping usually means unrestricted (OpenAI OAuth still filters later).
			hasUnrestricted = true
			continue
		}
		hasAnyMapping = true
		for key := range mapping {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if strings.Contains(key, "*") {
				continue
			}
			add(key)
		}
	}

	// Expand wildcards from restricted mappings against known catalogs.
	patterns := plazaAccountWildcardPatterns(accounts)
	if len(patterns) > 0 {
		hasAnyMapping = true
		pool := make([]string, 0, len(channelModels)+32)
		pool = append(pool, channelModels...)
		if g != nil {
			pool = append(pool, defaultModelsListCandidateIDs(g.Platform)...)
			pool = append(pool, plazaOpenVideoModelIDs(g)...)
			pool = append(pool, plazaOpenImageModelIDs(g)...)
		}
		for _, id := range pool {
			for _, pat := range patterns {
				if matchWildcard(pat, id) {
					add(id)
					break
				}
			}
		}
	}

	// Only when NO account declares a mapping do we expand the open catalog.
	// This matches GetAvailableModels: mapped keys win; unrestricted-only groups
	// fall back to defaults (channel + platform + open media).
	if !hasAnyMapping && hasUnrestricted {
		for _, id := range channelModels {
			add(id)
		}
		if g != nil {
			for _, id := range defaultModelsListCandidateIDs(g.Platform) {
				add(id)
			}
			for _, id := range plazaOpenVideoModelIDs(g) {
				add(id)
			}
			for _, id := range plazaOpenImageModelIDs(g) {
				add(id)
			}
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func plazaAccountWildcardPatterns(accounts []Account) []string {
	var patterns []string
	seen := make(map[string]struct{})
	for i := range accounts {
		mapping := accounts[i].GetModelMapping()
		for key := range mapping {
			key = strings.TrimSpace(key)
			if key == "" || !strings.Contains(key, "*") {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			patterns = append(patterns, key)
		}
	}
	return patterns
}

func plazaIntersectModelsList(candidates, modelsList []string) []string {
	if len(modelsList) == 0 {
		return candidates
	}
	allowed := make([]string, 0, len(modelsList))
	for _, raw := range modelsList {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			allowed = append(allowed, raw)
		}
	}
	if len(allowed) == 0 {
		return candidates
	}
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, model := range candidates {
		if !plazaModelsListAllows(allowed, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	// Also include exact models_list entries that accounts may serve even if not in candidates.
	for _, model := range allowed {
		if strings.Contains(model, "*") {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func plazaModelsListAllows(patterns []string, model string) bool {
	for _, pattern := range patterns {
		if pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

func plazaAnyAccountSupportsModel(accounts []Account, modelID string) bool {
	for i := range accounts {
		if accounts[i].IsModelSupported(modelID) {
			return true
		}
	}
	return false
}

func plazaUpsertModel(pg *PlazaGroup, idx map[string]int, modelID, platform string, priced map[string]plazaPricedModel) {
	if pg == nil || idx == nil {
		return
	}
	var pricing *ChannelModelPricing
	plat := platform
	if pm, ok := plazaLookupPriced(priced, modelID); ok {
		pricing = pm.pricing
		if pm.platform != "" {
			plat = pm.platform
		}
	}
	kind := plazaKindFromPricing(pricing)
	if kind == PlazaKindChat {
		// Prefer media kind hints when model id looks like image/video.
		if looksLikeImageModelID(modelID) {
			kind = PlazaKindImage
		} else if _, ok := ffLinkVideoModelProfileFor(modelID); ok {
			kind = PlazaKindVideo
		} else if IsFFLinkVideoPlatform(platform) {
			kind = PlazaKindVideo
		}
	}
	if at, seen := idx[modelID]; seen {
		if pg.Models[at].Pricing == nil && pricing != nil {
			pg.Models[at].Pricing = pricing
			if pg.Models[at].Kind == "" || pg.Models[at].Kind == PlazaKindChat {
				pg.Models[at].Kind = kind
			}
		}
		return
	}
	idx[modelID] = len(pg.Models)
	pg.Models = append(pg.Models, PlazaModel{
		Name:     modelID,
		Platform: plat,
		Kind:     kind,
		Pricing:  pricing,
	})
}

func plazaFilterUnsupportedModels(pg *PlazaGroup, idx map[string]int, accounts []Account) {
	if pg == nil || len(pg.Models) == 0 {
		return
	}
	kept := make([]PlazaModel, 0, len(pg.Models))
	newIdx := make(map[string]int, len(pg.Models))
	for i := range pg.Models {
		m := pg.Models[i]
		if !plazaAnyAccountSupportsModel(accounts, m.Name) {
			continue
		}
		newIdx[m.Name] = len(kept)
		kept = append(kept, m)
	}
	pg.Models = kept
	// refresh caller's idx map
	for k := range idx {
		delete(idx, k)
	}
	for k, v := range newIdx {
		idx[k] = v
	}
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

// appendPlazaMediaModels enriches already-admitted models with video/image price
// cards and kind metadata. It does NOT invent new model membership: membership is
// decided earlier by account mappings (production) or channel catalog (legacy).
//
// Legacy path (accountSource unset) still relies on this helper to inject open
// media models when groups enable image/video generation with no channel pricing.
// Production path passes inventMedia=false so open-media defaults cannot bypass
// account model_mapping.
func appendPlazaMediaModels(pg *PlazaGroup, g *Group, idx map[string]int) {
	appendPlazaMediaModelsOpt(pg, g, idx, true)
}

func appendPlazaMediaModelsOpt(pg *PlazaGroup, g *Group, idx map[string]int, inventMedia bool) {
	if pg == nil || g == nil || idx == nil {
		return
	}

	if IsFFLinkVideoPlatform(g.Platform) {
		// Account mappings are authoritative for plaza membership. Enrich those
		// admitted names directly so provider-specific casing/catalog omissions do
		// not detach an explicit group price card from its public model.
		for at := range pg.Models {
			modelID := pg.Models[at].Name
			if !GroupAllowsVideoModelExposure(g, modelID) {
				continue
			}
			if pg.Models[at].Kind == "" || pg.Models[at].Kind == PlazaKindChat {
				pg.Models[at].Kind = PlazaKindVideo
			}
			if pg.Models[at].VideoBillingUnit == "" {
				pg.Models[at].VideoBillingUnit = g.EffectiveVideoBillingUnitForModel(modelID)
			}
			if len(pg.Models[at].VideoResolutions) == 0 {
				pg.Models[at].VideoResolutions = plazaVideoResolutionsForModel(modelID)
			}
			if pg.Models[at].VideoPrices == nil {
				pg.Models[at].VideoPrices = plazaVideoPricesForModel(g, modelID)
			}
		}

		for _, modelID := range plazaOpenVideoModelIDs(g) {
			unit := g.EffectiveVideoBillingUnitForModel(modelID)
			prices := plazaVideoPricesForModel(g, modelID)
			resolutions := plazaVideoResolutionsForModel(modelID)
			if at, seen := plazaFindModelIndex(idx, modelID); seen {
				// Enrich existing entry with video price card.
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
			if !inventMedia {
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
			if at, seen := plazaFindModelIndex(idx, modelID); seen {
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
			if !inventMedia {
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

func plazaFindModelIndex(idx map[string]int, modelID string) (int, bool) {
	if at, ok := idx[modelID]; ok {
		return at, true
	}
	want := plazaNormalizeModelKey(modelID)
	if want == "" {
		return 0, false
	}
	for candidate, at := range idx {
		if plazaNormalizeModelKey(candidate) == want {
			return at, true
		}
	}
	return 0, false
}

// plazaOpenVideoModelIDs returns models the group currently exposes for video generation.
// Custom models_list (when enabled) is authoritative; otherwise platform defaults.
func plazaOpenVideoModelIDs(g *Group) []string {
	if g == nil || !IsFFLinkVideoPlatform(g.Platform) {
		return nil
	}
	defaults := FFLinkVideoModelIDsForPlatform(g.Platform)
	if !g.CustomModelsListEnabled() {
		out := make([]string, 0, len(defaults))
		for _, id := range defaults {
			if GroupAllowsVideoModelExposure(g, id) {
				out = append(out, id)
			}
		}
		return out
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
		if !GroupAllowsVideoModelExposure(g, id) {
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

// plazaNormalizeModelKey aligns plaza model names for cross-group matching.
// Mirrors frontend normalizeModelKey: lower-case, collapse spaces/underscores to '-'.
func plazaNormalizeModelKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	prevDash := false
	for _, r := range name {
		switch {
		case r == ' ' || r == '_' || r == '\t' || r == '\n' || r == '\r':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		case r == '-':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			b.WriteRune(r)
			prevDash = false
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

// plazaLookupPriced finds channel pricing overlay for a model id with exact then
// case/normalize-insensitive match so account model ids still get channel prices.
func plazaLookupPriced(priced map[string]plazaPricedModel, modelID string) (plazaPricedModel, bool) {
	if priced == nil {
		return plazaPricedModel{}, false
	}
	if pm, ok := priced[modelID]; ok {
		return pm, true
	}
	key := plazaNormalizeModelKey(modelID)
	if key == "" {
		return plazaPricedModel{}, false
	}
	for k, pm := range priced {
		if plazaNormalizeModelKey(k) == key {
			return pm, true
		}
	}
	return plazaPricedModel{}, false
}

func plazaVideoPricesUsable(p *VideoModelPrice) bool {
	if p == nil {
		return false
	}
	return p.Price480P != nil || p.Price720P != nil || p.Price1080P != nil ||
		p.Price1440P != nil || p.Price2160P != nil
}

func plazaImagePricesUsable(prices map[string]*float64) bool {
	if len(prices) == 0 {
		return false
	}
	for _, v := range prices {
		if v != nil {
			return true
		}
	}
	return false
}

// fillPlazaMissingPricing fills nil/empty model Pricing for plaza display only.
// Priority: (1) same model (normalized name) pricing already present on another group
// (2) LiteLLM synthesize via pricingService.
// Also copies video/image group price matrices across same-name models so multi-group
// cards do not show empty prices when only one group has the matrix configured.
func (s *ChannelService) fillPlazaMissingPricing(byGroup map[int64]*PlazaGroup, order []int64) {
	if byGroup == nil {
		return
	}
	// First pass: collect usable bases by normalized model name.
	baseByName := make(map[string]*ChannelModelPricing)
	videoByName := make(map[string]*PlazaModel)
	imageByName := make(map[string]*PlazaModel)
	for _, gid := range order {
		pg := byGroup[gid]
		if pg == nil {
			continue
		}
		for i := range pg.Models {
			m := &pg.Models[i]
			key := plazaNormalizeModelKey(m.Name)
			if key == "" {
				continue
			}
			if !pricingNeedsFallback(m.Pricing) {
				if _, ok := baseByName[key]; !ok {
					baseByName[key] = m.Pricing
				}
			}
			if plazaVideoPricesUsable(m.VideoPrices) {
				if _, ok := videoByName[key]; !ok {
					cp := *m
					videoByName[key] = &cp
				}
			}
			if plazaImagePricesUsable(m.ImagePrices) {
				if _, ok := imageByName[key]; !ok {
					cp := *m
					imageByName[key] = &cp
				}
			}
		}
	}
	// Second pass: fill missing entries.
	for _, gid := range order {
		pg := byGroup[gid]
		if pg == nil {
			continue
		}
		for i := range pg.Models {
			m := &pg.Models[i]
			key := plazaNormalizeModelKey(m.Name)
			if key == "" {
				continue
			}

			if pricingNeedsFallback(m.Pricing) {
				if p, ok := baseByName[key]; ok && p != nil {
					m.Pricing = p
					if m.Kind == "" || m.Kind == PlazaKindChat {
						m.Kind = plazaKindFromPricing(p)
					}
				} else if s != nil && s.pricingService != nil {
					lp := s.pricingService.GetModelPricing(m.Name)
					if lp != nil {
						synthesized := synthesizePricingFromLiteLLM(lp, m.Pricing)
						if !pricingNeedsFallback(synthesized) {
							m.Pricing = synthesized
							if m.Kind == "" || m.Kind == PlazaKindChat {
								m.Kind = plazaKindFromPricing(m.Pricing)
							}
						}
					}
				}
			}

			if !plazaVideoPricesUsable(m.VideoPrices) {
				if src, ok := videoByName[key]; ok && src != nil {
					m.VideoPrices = src.VideoPrices
					if m.VideoBillingUnit == "" {
						m.VideoBillingUnit = src.VideoBillingUnit
					}
					if len(m.VideoResolutions) == 0 && len(src.VideoResolutions) > 0 {
						m.VideoResolutions = append([]string(nil), src.VideoResolutions...)
					}
					if m.Kind == "" || m.Kind == PlazaKindChat {
						m.Kind = PlazaKindVideo
					}
				}
			}

			if !plazaImagePricesUsable(m.ImagePrices) {
				if src, ok := imageByName[key]; ok && src != nil {
					m.ImagePrices = src.ImagePrices
					if m.Kind == "" || m.Kind == PlazaKindChat {
						if m.Pricing == nil || m.Pricing.BillingMode == BillingModeImage {
							m.Kind = PlazaKindImage
						}
					}
				}
			}
		}
	}
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
