//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newPlazaChannelService 构造 ListPlazaGroups 测试用的 ChannelService。
func newPlazaChannelService(channels []Channel, groups []Group, pricing *PricingService) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{activeGroups: groups}, nil, nil)
	svc.pricingService = pricing
	return svc
}

// stubPlazaAccountSource implements PlazaAccountSource for unit tests.
type stubPlazaAccountSource struct {
	byGroup map[int64][]Account
	err     error
}

func (s *stubPlazaAccountSource) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	if s == nil {
		return nil, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.byGroup == nil {
		return nil, nil
	}
	return append([]Account(nil), s.byGroup[groupID]...), nil
}

func newPlazaChannelServiceWithAccounts(channels []Channel, groups []Group, pricing *PricingService, accounts map[int64][]Account) *ChannelService {
	svc := newPlazaChannelService(channels, groups, pricing)
	svc.SetPlazaAccountSource(&stubPlazaAccountSource{byGroup: accounts})
	return svc
}

func plazaMappedAccount(id int64, platform string, models ...string) Account {
	mapping := make(map[string]any, len(models))
	for _, m := range models {
		mapping[m] = m
	}
	return Account{
		ID:          id,
		Name:        "acc",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "k", "model_mapping": mapping},
	}
}

func plazaUnrestrictedAccount(id int64, platform string) Account {
	return Account{
		ID:          id,
		Name:        "acc",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "k"},
	}
}

func plazaPricedChannel(id int64, name string, groupIDs []int64, platform string, models ...string) Channel {
	return Channel{
		ID:       id,
		Name:     name,
		Status:   StatusActive,
		GroupIDs: groupIDs,
		ModelPricing: []ChannelModelPricing{{
			Platform:    platform,
			Models:      models,
			BillingMode: BillingModeToken,
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
		}},
	}
}

func TestListPlazaGroups_GroupCentricAggregation(t *testing.T) {
	// 两个渠道挂同一分组:模型并入同一 PlazaGroup;无模型的分组不返回。
	channels := []Channel{
		plazaPricedChannel(1, "chA", []int64{10}, "anthropic", "claude-sonnet"),
		plazaPricedChannel(2, "chB", []int64{10}, "anthropic", "claude-opus"),
	}
	groups := []Group{
		{ID: 10, Name: "g-main", Description: "desc", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "g-empty", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1, "无模型的分组不应返回")
	require.Equal(t, int64(10), out[0].ID)
	require.Equal(t, "desc", out[0].Description)
	require.Len(t, out[0].Models, 2)
	// 组内模型按名称排序
	require.Equal(t, "claude-opus", out[0].Models[0].Name)
	require.Equal(t, "claude-sonnet", out[0].Models[1].Name)
}

func TestListPlazaGroups_DedupFirstWinsWithPricingUpgrade(t *testing.T) {
	// 同名模型:先见者胜;仅当已存条目无定价而新条目有定价时升级替换。
	unpriced := Channel{
		ID: 1, Name: "alpha", Status: StatusActive, GroupIDs: []int64{10},
		// mapping-only → SupportedModels 产出无定价条目
		ModelMapping: map[string]map[string]string{
			"anthropic": {"claude-sonnet": "claude-sonnet"},
		},
	}
	priced := plazaPricedChannel(2, "beta", []int64{10}, "anthropic", "claude-sonnet")
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}

	// alpha(无价)按名称序先于 beta(有价):先见者无价,应被有价条目升级。
	svc := newPlazaChannelService([]Channel{priced, unpriced}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	require.NotNil(t, out[0].Models[0].Pricing, "无价条目应被有价条目升级")
	require.NotNil(t, out[0].Models[0].Pricing.InputPrice)
}

func TestListPlazaGroups_PlatformIsolation(t *testing.T) {
	// 渠道同时有 anthropic/openai 定价,anthropic 分组只应看到 anthropic 模型。
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{Platform: "anthropic", Models: []string{"claude-sonnet"}, InputPrice: testPtrFloat64(3e-6)},
			{Platform: "openai", Models: []string{"gpt-5"}, InputPrice: testPtrFloat64(2e-6)},
		},
	}
	groups := []Group{
		{ID: 10, Name: "g-claude", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "g-gpt", Platform: "openai", RateMultiplier: 1},
	}
	svc := newPlazaChannelService([]Channel{ch}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string][]PlazaModel{}
	for _, g := range out {
		byName[g.Name] = g.Models
	}
	require.Len(t, byName["g-claude"], 1)
	require.Equal(t, "claude-sonnet", byName["g-claude"][0].Name)
	require.Len(t, byName["g-gpt"], 1)
	require.Equal(t, "gpt-5", byName["g-gpt"][0].Name)
}

func TestListPlazaGroups_CompositeIncludesConfiguredConcretePlatforms(t *testing.T) {
	anthropicPrice := 3e-6
	openAIPrice := 2e-6
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"shared-model"}, InputPrice: &anthropicPrice},
			{Platform: PlatformOpenAI, Models: []string{"shared-model"}, InputPrice: &openAIPrice},
			{Platform: "", Models: []string{"empty-platform"}},
			{Platform: PlatformComposite, Models: []string{"nested-composite"}},
			{Platform: "unknown-platform", Models: []string{"unknown-platform"}},
		},
	}
	groups := []Group{{ID: 10, Name: "composite", Platform: PlatformComposite, RateMultiplier: 1}}

	out, err := newPlazaChannelService([]Channel{ch}, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2, "only concrete platforms are included and same-named models remain distinct")
	require.Equal(t, PlatformAnthropic, out[0].Models[0].Platform)
	require.Equal(t, PlatformOpenAI, out[0].Models[1].Platform)
	require.InDelta(t, anthropicPrice, *out[0].Models[0].Pricing.InputPrice, 1e-12)
	require.InDelta(t, openAIPrice, *out[0].Models[1].Pricing.InputPrice, 1e-12)
}

func TestListPlazaGroups_CompositeAndOrdinaryGroupsDoNotLeakPlatforms(t *testing.T) {
	ch := Channel{
		ID: 1, Name: "multi", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"claude-sonnet"}, InputPrice: testPtrFloat64(3e-6)},
			{Platform: PlatformOpenAI, Models: []string{"gpt-5"}, InputPrice: testPtrFloat64(2e-6)},
		},
	}
	groups := []Group{
		{ID: 10, Name: "anthropic-only", Platform: PlatformAnthropic, RateMultiplier: 1},
		{ID: 20, Name: "composite", Platform: PlatformComposite, RateMultiplier: 1},
	}

	out, err := newPlazaChannelService([]Channel{ch}, groups, nil).ListPlazaGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string]PlazaGroup{}
	for _, group := range out {
		byName[group.Name] = group
	}
	require.Len(t, byName["anthropic-only"].Models, 1)
	require.Equal(t, []PlazaModel{{
		Name: "claude-sonnet", Platform: PlatformAnthropic, Pricing: byName["anthropic-only"].Models[0].Pricing,
	}}, byName["anthropic-only"].Models)
	require.Len(t, byName["composite"].Models, 2)
	require.Equal(t, []string{"claude-sonnet", "gpt-5"}, []string{
		byName["composite"].Models[0].Name,
		byName["composite"].Models[1].Name,
	})
	require.Equal(t, []string{PlatformAnthropic, PlatformOpenAI}, []string{
		byName["composite"].Models[0].Platform,
		byName["composite"].Models[1].Platform,
	})
}

func TestListPlazaGroups_InactiveChannelSkipped(t *testing.T) {
	inactive := plazaPricedChannel(1, "off", []int64{10}, "anthropic", "claude-sonnet")
	inactive.Status = "inactive"
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService([]Channel{inactive}, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestListPlazaGroups_SortedByRateMultiplierAsc(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10, 20, 30}, "anthropic", "claude-sonnet"),
	}
	groups := []Group{
		{ID: 10, Name: "b-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 20, Name: "a-standard", Platform: "anthropic", RateMultiplier: 1},
		{ID: 30, Name: "cheap", Platform: "anthropic", RateMultiplier: 0.5},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "cheap", out[0].Name, "倍率低者在前")
	require.Equal(t, "a-standard", out[1].Name, "同倍率按名称")
	require.Equal(t, "b-standard", out[2].Name)
}

func TestListPlazaGroups_OfficialPricingFill(t *testing.T) {
	pricingSvc := newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
		"claude-sonnet": {
			Mode:                                "chat",
			InputCostPerToken:                   3e-6,
			OutputCostPerToken:                  1.5e-5,
			CacheCreationInputTokenCost:         3.75e-6,
			CacheCreationInputTokenCostAbove1hr: 6e-6,
			CacheReadInputTokenCost:             3e-7,
		},
		"token-absent": {Mode: "image_generation", TokenPricingAbsent: true, OutputCostPerImage: 0.04},
	})
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, "anthropic", "claude-sonnet", "unknown-model", "token-absent"),
	}
	groups := []Group{{ID: 10, Name: "g", Platform: "anthropic", RateMultiplier: 1}}
	svc := newPlazaChannelService(channels, groups, pricingSvc)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 3)

	byName := map[string]PlazaModel{}
	for _, m := range out[0].Models {
		byName[m.Name] = m
	}
	// 命中:填充完整官方价(含 1h 缓存写入)
	official := byName["claude-sonnet"].OfficialPricing
	require.NotNil(t, official)
	require.InDelta(t, 3e-6, *official.InputPrice, 1e-12)
	require.InDelta(t, 6e-6, *official.CacheWrite1hPrice, 1e-12)
	require.InDelta(t, 3e-7, *official.CacheReadPrice, 1e-12)
	// 未命中:nil(GetModelPricing 的 claude 系列模糊匹配对非 claude 名不生效)
	require.Nil(t, byName["unknown-model"].OfficialPricing)
	// TokenPricingAbsent 条目不作为官方 token 价展示
	require.Nil(t, byName["token-absent"].OfficialPricing)
}

func TestListPlazaGroups_GroupImagePriceOverridesChannelPricing(t *testing.T) {
	// 图片计费模型:档位价按实收口径合成(分组图片价 > 渠道档位价 > 渠道默认按次价),
	// 分组独立倍率字段透传;未配图片价的分组保持渠道定价原样。
	perReq := 0.2
	tier4K := 0.3
	imgPrice := 0.02
	channels := []Channel{{
		ID: 1, Name: "img-ch", Status: StatusActive, GroupIDs: []int64{10, 20},
		ModelPricing: []ChannelModelPricing{{
			Platform:        "openai",
			Models:          []string{"gpt-image-2"},
			BillingMode:     BillingModeImage,
			PerRequestPrice: &perReq,
			Intervals:       []PricingInterval{{TierLabel: "4K", PerRequestPrice: &tier4K}},
		}},
	}}
	groups := []Group{
		{ID: 10, Name: "g-media", Platform: "openai", RateMultiplier: 1,
			ImagePrice1K: &imgPrice, ImageRateIndependent: true, ImageRateMultiplier: 1},
		{ID: 20, Name: "g-plain", Platform: "openai", RateMultiplier: 0.1},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	byName := map[string]PlazaGroup{}
	for _, g := range out {
		byName[g.Name] = g
	}

	media := byName["g-media"]
	require.True(t, media.ImageRateIndependent)
	require.InDelta(t, 1.0, media.ImageRateMultiplier, 1e-9)
	require.Len(t, media.Models, 1)
	p := media.Models[0].Pricing
	require.NotNil(t, p)
	require.Len(t, p.Intervals, 3)
	tierPrices := map[string]float64{}
	for _, iv := range p.Intervals {
		require.NotNil(t, iv.PerRequestPrice)
		tierPrices[iv.TierLabel] = *iv.PerRequestPrice
	}
	require.InDelta(t, 0.02, tierPrices["1K"], 1e-9, "1K 用分组图片价")
	require.InDelta(t, 0.2, tierPrices["2K"], 1e-9, "2K 分组未配,回落渠道默认按次价")
	require.InDelta(t, 0.3, tierPrices["4K"], 1e-9, "4K 分组未配,回落渠道档位价")

	plain := byName["g-plain"]
	require.False(t, plain.ImageRateIndependent)
	require.Len(t, plain.Models, 1)
	pp := plain.Models[0].Pricing
	require.NotNil(t, pp)
	require.Len(t, pp.Intervals, 1, "未配分组图片价:渠道定价原样")
	require.InDelta(t, 0.2, *pp.PerRequestPrice, 1e-9)

	// 合成为克隆,渠道原始定价不被修改
	require.Len(t, channels[0].ModelPricing[0].Intervals, 1)
}

func TestListPlazaGroups_GroupImagePriceIgnoredForNonImageModes(t *testing.T) {
	// token 模式定价不受分组图片价影响。
	imgPrice := 0.02
	channels := []Channel{plazaPricedChannel(1, "ch", []int64{10}, "openai", "gpt-5")}
	groups := []Group{{ID: 10, Name: "g", Platform: "openai", RateMultiplier: 1, ImagePrice1K: &imgPrice}}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	p := out[0].Models[0].Pricing
	require.NotNil(t, p)
	require.Empty(t, p.Intervals)
	require.NotNil(t, p.InputPrice)
	require.Nil(t, p.PerRequestPrice)
}

func TestListPlazaGroups_RepoErrorsPropagate(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	svc := NewChannelService(repo, &stubGroupRepoForAvailable{}, nil, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)

	svc2 := NewChannelService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, nil }},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
		nil, nil,
	)
	out2, err2 := svc2.ListPlazaGroups(context.Background())
	require.Nil(t, out2)
	require.ErrorIs(t, err2, sentinel)
}

func TestListPlazaGroups_VideoAndImageModels(t *testing.T) {
	price720 := 0.15
	price1k := 0.04
	groups := []Group{
		{
			ID: 10, Name: "seedance-a", Platform: PlatformSeedance, RateMultiplier: 1,
			VideoBillingUnit: VideoBillingUnitPerSecond,
			VideoModelPrices: VideoModelPrices{
				"seedance-2.0": {BillingUnit: VideoBillingUnitPerRequest, Price720P: &price720},
			},
			ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"seedance-2.0"}},
		},
		{
			ID: 20, Name: "seedance-b", Platform: PlatformSeedance, RateMultiplier: 0.8,
			VideoBillingUnit: VideoBillingUnitPerRequest,
			VideoModelPrices: VideoModelPrices{
				"seedance-2.0": {Price720P: &price720},
			},
			ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"seedance-2.0"}},
		},
		{
			ID: 30, Name: "img-g", Platform: PlatformOpenAI, RateMultiplier: 1,
			AllowImageGeneration: true,
			ImagePrice1K:         &price1k,
		},
	}
	svc := newPlazaChannelService(nil, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)

	byName := map[string]PlazaGroup{}
	for _, g := range out {
		byName[g.Name] = g
	}
	// Video groups expose model with resolution price + billing unit.
	m0 := byName["seedance-a"].Models[0]
	require.Equal(t, "seedance-2.0", m0.Name)
	require.Equal(t, PlazaKindVideo, m0.Kind)
	require.Equal(t, VideoBillingUnitPerRequest, m0.VideoBillingUnit)
	require.NotNil(t, m0.VideoPrices)
	require.NotNil(t, m0.VideoPrices.Price720P)
	require.InDelta(t, 0.15, *m0.VideoPrices.Price720P, 1e-12)

	m1 := byName["seedance-b"].Models[0]
	require.Equal(t, VideoBillingUnitPerRequest, m1.VideoBillingUnit)

	// Image group exposes image models with tier prices.
	var img *PlazaModel
	for i := range byName["img-g"].Models {
		if byName["img-g"].Models[i].Name == "gpt-image-2" {
			img = &byName["img-g"].Models[i]
			break
		}
	}
	require.NotNil(t, img)
	require.Equal(t, PlazaKindImage, img.Kind)
	require.NotNil(t, img.ImagePrices)
	require.NotNil(t, img.ImagePrices["1K"])
	require.InDelta(t, 0.04, *img.ImagePrices["1K"], 1e-12)
}

func TestListPlazaGroups_TianyueOriginalCaseGetsVideoPricing(t *testing.T) {
	standardPrice := 5.5
	fastPrice := 4.0
	group := Group{
		ID: 28, Name: "seedance-933", Platform: PlatformSeedance, Status: StatusActive, RateMultiplier: 1,
		VideoBillingUnit: VideoBillingUnitPerSecond,
		VideoModelPrices: VideoModelPrices{
			strings.ToLower(SeedanceTianyueSD20Model): {
				BillingUnit: VideoBillingUnitPerRequest,
				Price720P:   &standardPrice,
			},
			strings.ToLower(SeedanceTianyueSD20FastModel): {
				BillingUnit: VideoBillingUnitPerRequest,
				Price720P:   &fastPrice,
			},
		},
	}
	account := Account{
		ID: 820, Name: "tianyue", Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"api_key":        "k",
			"video_provider": VideoProviderTianyue,
			"model_mapping": map[string]any{
				SeedanceTianyueSD20Model:     SeedanceTianyueSD20Model,
				SeedanceTianyueSD20FastModel: SeedanceTianyueSD20FastModel,
			},
		},
	}

	svc := newPlazaChannelServiceWithAccounts(nil, []Group{group}, nil, map[int64][]Account{28: {account}})
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)

	models := make(map[string]PlazaModel, len(out[0].Models))
	for _, model := range out[0].Models {
		models[model.Name] = model
	}
	for modelID, wantPrice := range map[string]float64{
		SeedanceTianyueSD20Model:     standardPrice,
		SeedanceTianyueSD20FastModel: fastPrice,
	} {
		model, ok := models[modelID]
		require.True(t, ok, "original Tianyue model casing must be preserved")
		require.Equal(t, PlazaKindVideo, model.Kind)
		require.Equal(t, VideoBillingUnitPerRequest, model.VideoBillingUnit)
		require.Equal(t, []string{VideoBillingResolution720P}, model.VideoResolutions)
		require.NotNil(t, model.VideoPrices)
		require.NotNil(t, model.VideoPrices.Price720P)
		require.InDelta(t, wantPrice, *model.VideoPrices.Price720P, 1e-12)
	}
}

func TestListPlazaGroups_AvgFirstTokenAlwaysPresent(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "c1", []int64{1}, "openai", "gpt-4o"),
	}
	groups := []Group{
		{ID: 1, Name: "g1", Platform: "openai", Status: StatusActive, RateMultiplier: 1},
	}
	svc := newPlazaChannelService(channels, groups, nil)
	out, err := svc.ListPlazaGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].AvgFirstTokenMs <= 0 {
		t.Fatalf("expected baseline ttft, got %d", out[0].AvgFirstTokenMs)
	}
	if out[0].TTFTDisclaimer == "" {
		t.Fatal("expected disclaimer")
	}
}

func TestListPlazaGroups_AccountMappingIsSourceOfTruth(t *testing.T) {
	// Channel catalog lists image2 + claude, but group accounts only enable claude.
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, PlatformOpenAI, "gpt-4o", "gpt-image-2"),
	}
	groups := []Group{
		{
			ID: 10, Name: "g-img", Platform: PlatformOpenAI, RateMultiplier: 1,
			AllowImageGeneration: true,
			Status:               StatusActive,
		},
	}
	accounts := map[int64][]Account{
		10: {
			plazaMappedAccount(1, PlatformOpenAI, "gpt-4o"),
			plazaMappedAccount(2, PlatformOpenAI, "gpt-4o-mini"),
		},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, accounts)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	names := make([]string, 0, len(out[0].Models))
	for _, m := range out[0].Models {
		names = append(names, m.Name)
	}
	require.Contains(t, names, "gpt-4o")
	require.Contains(t, names, "gpt-4o-mini")
	require.NotContains(t, names, "gpt-image-2", "accounts do not enable image2")
	require.NotContains(t, names, "gpt-image-1", "defaults must not invent image models without account support")
}

func TestPlazaWeijin900RequiresExplicitPriceAndNeverShowsPrivateID(t *testing.T) {
	price720 := 0.05
	account := Account{
		ID: 1, Platform: PlatformSeedance, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "k", "model_mapping": map[string]any{
			SeedanceWeijin900Model:         SeedanceWeijin900UpstreamModel,
			SeedanceWeijin900UpstreamModel: SeedanceWeijin900UpstreamModel,
		}},
	}
	unpriced := &Group{ID: 10, Platform: PlatformSeedance}
	priced := &Group{
		ID: 20, Platform: PlatformSeedance,
		VideoModelPrices: VideoModelPrices{
			SeedanceWeijin900Model: {BillingUnit: VideoBillingUnitPerRequest, Price720P: &price720},
		},
	}

	require.NotContains(t, plazaCollectCandidateModels(unpriced, []Account{account}, nil), SeedanceWeijin900Model)
	require.NotContains(t, plazaOpenVideoModelIDs(unpriced), SeedanceWeijin900Model)

	pricedCandidates := plazaCollectCandidateModels(priced, []Account{account}, nil)
	require.Contains(t, pricedCandidates, SeedanceWeijin900Model)
	require.NotContains(t, pricedCandidates, legacyWeijin900PublicModelForTest)
	require.NotContains(t, pricedCandidates, SeedanceWeijin900UpstreamModel)
	openModels := plazaOpenVideoModelIDs(priced)
	require.Contains(t, openModels, SeedanceWeijin900Model)
	require.NotContains(t, openModels, legacyWeijin900PublicModelForTest)
	require.NotContains(t, openModels, SeedanceWeijin900UpstreamModel)
}

func TestListPlazaGroups_SameModelOnlyOnGroupsWithSupportingAccounts(t *testing.T) {
	// Shared channel attaches both groups, but only group A accounts support 5.6.
	channels := []Channel{
		plazaPricedChannel(1, "shared", []int64{10, 20}, PlatformOpenAI, "gpt-5.6", "gpt-4o"),
	}
	groups := []Group{
		{ID: 10, Name: "g-a", Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive},
		{ID: 20, Name: "g-b", Platform: PlatformOpenAI, RateMultiplier: 1.2, Status: StatusActive},
	}
	accounts := map[int64][]Account{
		10: {plazaMappedAccount(1, PlatformOpenAI, "gpt-5.6", "gpt-4o")},
		20: {plazaMappedAccount(2, PlatformOpenAI, "gpt-4o")},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, accounts)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	byName := map[string]PlazaGroup{}
	for _, g := range out {
		byName[g.Name] = g
	}
	require.Contains(t, byName, "g-a")
	require.Contains(t, byName, "g-b")

	aNames := modelNames(byName["g-a"])
	bNames := modelNames(byName["g-b"])
	require.Contains(t, aNames, "gpt-5.6")
	require.NotContains(t, bNames, "gpt-5.6")
	require.Contains(t, bNames, "gpt-4o")
}

func TestListPlazaGroups_UnrestrictedAccountUsesCatalogButStillSchedulable(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, PlatformOpenAI, "gpt-4o"),
	}
	groups := []Group{
		{
			ID: 10, Name: "g", Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive,
			AllowImageGeneration: true,
		},
	}
	accounts := map[int64][]Account{
		10: {plazaUnrestrictedAccount(1, PlatformOpenAI)},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, accounts)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	names := modelNames(out[0])
	require.Contains(t, names, "gpt-4o")
	// Unrestricted can serve default image models when group allows image generation.
	require.Contains(t, names, "gpt-image-2")
}

func TestListPlazaGroups_MixedMappedAndUnrestrictedUsesMappingOnly(t *testing.T) {
	// One unrestricted account must NOT expand image catalog when another account
	// already declares an explicit model_mapping (align with GetAvailableModels).
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, PlatformOpenAI, "gpt-4o", "gpt-image-2"),
	}
	groups := []Group{
		{
			ID: 10, Name: "mixed", Platform: PlatformOpenAI, RateMultiplier: 1,
			AllowImageGeneration: true, Status: StatusActive,
		},
	}
	accounts := map[int64][]Account{
		10: {
			plazaMappedAccount(1, PlatformOpenAI, "gpt-4o"),
			plazaUnrestrictedAccount(2, PlatformOpenAI),
		},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, accounts)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	names := modelNames(out[0])
	require.Contains(t, names, "gpt-4o")
	require.NotContains(t, names, "gpt-image-2", "mapped keys win over unrestricted catalog expansion")
	require.NotContains(t, names, "gpt-image-1")
}

func TestListPlazaGroups_ChannelCatalogDoesNotInventForMappedGroup(t *testing.T) {
	// Shared channel prices gpt-5.6 for both groups; only group A enables it on accounts.
	channels := []Channel{
		plazaPricedChannel(1, "shared", []int64{10, 20}, PlatformOpenAI, "gpt-5.6", "gpt-image-2"),
	}
	groups := []Group{
		{ID: 10, Name: "g-a", Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive, AllowImageGeneration: true},
		{ID: 20, Name: "g-b", Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive, AllowImageGeneration: true},
	}
	accounts := map[int64][]Account{
		10: {
			plazaMappedAccount(1, PlatformOpenAI, "gpt-5.6", "gpt-4o"),
			plazaMappedAccount(2, PlatformOpenAI, "gpt-5.6"),
		},
		20: {
			plazaMappedAccount(3, PlatformOpenAI, "gpt-4o"),
			plazaMappedAccount(4, PlatformOpenAI, "gpt-4o-mini"),
		},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, accounts)
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	byName := map[string]PlazaGroup{}
	for _, g := range out {
		byName[g.Name] = g
	}
	aNames := modelNames(byName["g-a"])
	bNames := modelNames(byName["g-b"])
	require.Contains(t, aNames, "gpt-5.6")
	require.NotContains(t, aNames, "gpt-image-2")
	require.NotContains(t, bNames, "gpt-5.6")
	require.NotContains(t, bNames, "gpt-image-2")
	require.Contains(t, bNames, "gpt-4o")
	require.Contains(t, bNames, "gpt-4o-mini")
}

func TestListPlazaGroups_NoSchedulableAccountsHidesGroup(t *testing.T) {
	channels := []Channel{
		plazaPricedChannel(1, "ch", []int64{10}, PlatformOpenAI, "gpt-4o"),
	}
	groups := []Group{
		{ID: 10, Name: "g", Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive},
	}
	svc := newPlazaChannelServiceWithAccounts(channels, groups, nil, map[int64][]Account{10: {}})
	out, err := svc.ListPlazaGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 0)
}

func TestFillPlazaMissingPricing_NormalizedCrossGroupAndMedia(t *testing.T) {
	in := 3e-6
	out := 1.5e-5
	v720 := 0.12
	i1k := 0.04
	svc := &ChannelService{}

	byGroup := map[int64]*PlazaGroup{
		1: {
			ID: 1, Name: "alpha",
			Models: []PlazaModel{
				{
					Name: "GPT-5.6",
					Kind: PlazaKindChat,
					Pricing: &ChannelModelPricing{
						BillingMode: BillingModeToken,
						InputPrice:  &in,
						OutputPrice: &out,
					},
				},
				{
					Name:             "seedance-2.0",
					Kind:             PlazaKindVideo,
					VideoBillingUnit: "second",
					VideoResolutions: []string{"720p"},
					VideoPrices:      &VideoModelPrice{Price720P: &v720},
				},
				{
					Name:        "gpt-image-2",
					Kind:        PlazaKindImage,
					ImagePrices: map[string]*float64{"1K": &i1k},
				},
			},
		},
		2: {
			ID: 2, Name: "beta",
			Models: []PlazaModel{
				{Name: "gpt_5.6", Kind: PlazaKindChat, Pricing: nil},
				{Name: "Seedance-2.0", Kind: PlazaKindVideo},
				{Name: "GPT Image 2", Kind: PlazaKindImage},
			},
		},
	}
	svc.fillPlazaMissingPricing(byGroup, []int64{1, 2})

	beta := byGroup[2]
	require.Len(t, beta.Models, 3)

	require.NotNil(t, beta.Models[0].Pricing)
	require.NotNil(t, beta.Models[0].Pricing.InputPrice)
	require.InDelta(t, in, *beta.Models[0].Pricing.InputPrice, 1e-15)
	require.InDelta(t, out, *beta.Models[0].Pricing.OutputPrice, 1e-15)

	require.True(t, plazaVideoPricesUsable(beta.Models[1].VideoPrices))
	require.NotNil(t, beta.Models[1].VideoPrices.Price720P)
	require.InDelta(t, v720, *beta.Models[1].VideoPrices.Price720P, 1e-12)
	require.Equal(t, "second", beta.Models[1].VideoBillingUnit)

	require.True(t, plazaImagePricesUsable(beta.Models[2].ImagePrices))
	require.NotNil(t, beta.Models[2].ImagePrices["1K"])
	require.InDelta(t, i1k, *beta.Models[2].ImagePrices["1K"], 1e-12)
}

func TestPlazaLookupPriced_CaseInsensitive(t *testing.T) {
	in := 1e-6
	priced := map[string]plazaPricedModel{
		"gpt-5.6": {platform: PlatformOpenAI, pricing: &ChannelModelPricing{InputPrice: &in}},
	}
	pm, ok := plazaLookupPriced(priced, "GPT_5.6")
	require.True(t, ok)
	require.NotNil(t, pm.pricing)
	require.InDelta(t, in, *pm.pricing.InputPrice, 1e-15)
}

func TestPlazaNormalizeModelKey(t *testing.T) {
	require.Equal(t, "gpt-5.6", plazaNormalizeModelKey("GPT_5.6"))
	require.Equal(t, "gpt-5.6", plazaNormalizeModelKey(" gpt 5.6 "))
	require.Equal(t, "seedance-2.0", plazaNormalizeModelKey("Seedance-2.0"))
}

func modelNames(g PlazaGroup) []string {
	out := make([]string, 0, len(g.Models))
	for _, m := range g.Models {
		out = append(out, m.Name)
	}
	return out
}
