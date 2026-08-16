//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type creazyCanvasAPIKeyStub struct {
	keys           map[int64]*APIKey
	listKeys       []APIKey
	videoPrices    VideoModelPrices
	videoPricesErr error
	quotaErr       error
	getErr         error
	listErr        error
}

func (s *creazyCanvasAPIKeyStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	key := s.keys[id]
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	cloned := *key
	if key.Group != nil {
		g := *key.Group
		cloned.Group = &g
	}
	return &cloned, nil
}

func (s *creazyCanvasAPIKeyStub) List(_ context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	out := make([]APIKey, 0)
	for _, key := range s.listKeys {
		if key.UserID != userID {
			continue
		}
		if filters.Status != "" && key.Status != filters.Status {
			continue
		}
		out = append(out, key)
	}
	return out, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Total: int64(len(out)), Pages: 1}, nil
}

func (s *creazyCanvasAPIKeyStub) CheckAPIKeyQuotaAndExpiry(_ *APIKey) error {
	return s.quotaErr
}

func (s *creazyCanvasAPIKeyStub) GetVideoModelPricesByUserAndGroup(_ context.Context, _, _ int64) (VideoModelPrices, error) {
	if s.videoPricesErr != nil {
		return nil, s.videoPricesErr
	}
	return cloneVideoModelPrices(s.videoPrices), nil
}

type creazyCanvasWorkRepoStub struct {
	nextID int64
	works  map[int64]*CreazyCanvasWork
}

func (r *creazyCanvasWorkRepoStub) ListAdminImageWorks(_ context.Context, params pagination.PaginationParams, filters CreazyCanvasAdminWorkFilters) ([]CreazyCanvasAdminWork, *pagination.PaginationResult, error) {
	out := make([]CreazyCanvasAdminWork, 0)
	for _, work := range r.works {
		if work.Kind != CreazyCanvasWorkKindImage || work.DeletedAt != nil {
			continue
		}
		if filters.Status != "" && work.Status != filters.Status {
			continue
		}
		if filters.GatewayType != "" && work.GatewayType != filters.GatewayType {
			continue
		}
		if filters.ActiveOnly && isCreazyCanvasWorkTerminalStatus(work.Status) {
			continue
		}
		if filters.Search != "" && !strings.Contains(strings.ToLower(work.Prompt+" "+work.PublicModel+" "+work.GatewayRemoteID), strings.ToLower(filters.Search)) {
			continue
		}
		out = append(out, CreazyCanvasAdminWork{CreazyCanvasWork: *work, UserEmail: "owner@example.com", APIKeyName: "canvas-key"})
	}
	return out, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Total: int64(len(out)), Pages: 1}, nil
}

func (r *creazyCanvasWorkRepoStub) GetAdminImageWork(_ context.Context, id int64) (*CreazyCanvasAdminWork, error) {
	work := r.works[id]
	if work == nil || work.Kind != CreazyCanvasWorkKindImage || work.DeletedAt != nil {
		return nil, ErrCreazyCanvasWorkNotFound
	}
	return &CreazyCanvasAdminWork{CreazyCanvasWork: *work, UserEmail: "owner@example.com", APIKeyName: "canvas-key"}, nil
}

func (r *creazyCanvasWorkRepoStub) UpdateAdminImageWorkStatus(_ context.Context, id int64, status, errorMessage string) error {
	work := r.works[id]
	if work == nil || work.Kind != CreazyCanvasWorkKindImage || work.DeletedAt != nil {
		return ErrCreazyCanvasWorkNotFound
	}
	work.Status = status
	work.ErrorMessage = errorMessage
	work.UpdatedAt = time.Now().UTC()
	return nil
}

func newCreazyCanvasWorkRepoStub() *creazyCanvasWorkRepoStub {
	return &creazyCanvasWorkRepoStub{nextID: 1, works: make(map[int64]*CreazyCanvasWork)}
}

func (r *creazyCanvasWorkRepoStub) Create(_ context.Context, work *CreazyCanvasWork) error {
	if work.ID == 0 {
		work.ID = r.nextID
		r.nextID++
	}
	cloned := *work
	if work.ParamsJSON != nil {
		params := make(map[string]any, len(work.ParamsJSON))
		for k, v := range work.ParamsJSON {
			params[k] = v
		}
		cloned.ParamsJSON = params
	}
	r.works[work.ID] = &cloned
	return nil
}

func (r *creazyCanvasWorkRepoStub) GetByIDForUser(_ context.Context, id, userID int64) (*CreazyCanvasWork, error) {
	work := r.works[id]
	if work == nil || work.UserID != userID || work.DeletedAt != nil {
		return nil, ErrCreazyCanvasWorkNotFound
	}
	cloned := *work
	return &cloned, nil
}

func (r *creazyCanvasWorkRepoStub) ListByUser(_ context.Context, userID int64, params pagination.PaginationParams, filters CreazyCanvasWorkListFilters) ([]CreazyCanvasWork, *pagination.PaginationResult, error) {
	out := make([]CreazyCanvasWork, 0)
	for _, work := range r.works {
		if work.UserID != userID || work.DeletedAt != nil {
			continue
		}
		if filters.Kind != "" && work.Kind != filters.Kind {
			continue
		}
		if filters.Status != "" && work.Status != filters.Status {
			continue
		}
		if filters.APIKeyID != nil && work.APIKeyID != *filters.APIKeyID {
			continue
		}
		out = append(out, *work)
	}
	return out, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Total: int64(len(out)), Pages: 1}, nil
}

func (r *creazyCanvasWorkRepoStub) SoftDelete(_ context.Context, id, userID int64) error {
	work := r.works[id]
	if work == nil || work.UserID != userID || work.DeletedAt != nil {
		return ErrCreazyCanvasWorkNotFound
	}
	now := time.Now()
	work.DeletedAt = &now
	return nil
}

func (r *creazyCanvasWorkRepoStub) UpdateContentMeta(_ context.Context, work *CreazyCanvasWork) error {
	existing := r.works[work.ID]
	if existing == nil || existing.UserID != work.UserID {
		return ErrCreazyCanvasWorkNotFound
	}
	existing.ObjectKey = work.ObjectKey
	existing.StorageProvider = work.StorageProvider
	existing.Bucket = work.Bucket
	existing.ObjectURL = work.ObjectURL
	existing.PreviewURL = work.PreviewURL
	existing.MimeType = work.MimeType
	existing.SizeBytes = work.SizeBytes
	existing.Status = work.Status
	existing.ErrorMessage = work.ErrorMessage
	existing.GatewayType = work.GatewayType
	existing.GatewayRemoteID = work.GatewayRemoteID
	existing.PublicModel = work.PublicModel
	existing.Prompt = work.Prompt
	if work.ParamsJSON != nil {
		params := make(map[string]any, len(work.ParamsJSON))
		for k, v := range work.ParamsJSON {
			params[k] = v
		}
		existing.ParamsJSON = params
	}
	return nil
}

func TestCreazyCanvasDeleteWorkOnlyAllowsTerminalTasks(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	repo.works[1] = &CreazyCanvasWork{ID: 1, UserID: 7, Status: CreazyCanvasWorkStatusRunning}
	repo.works[2] = &CreazyCanvasWork{ID: 2, UserID: 7, Status: CreazyCanvasWorkStatusSucceeded}
	repo.works[3] = &CreazyCanvasWork{ID: 3, UserID: 7, Status: CreazyCanvasWorkStatusRunning, ExpiresAt: time.Now().Add(-time.Minute)}
	repo.works[4] = &CreazyCanvasWork{ID: 4, UserID: 7, Status: CreazyCanvasWorkStatusRunning, ExpiresAt: time.Now().Add(time.Hour)}
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)

	require.ErrorIs(t, svc.DeleteWork(context.Background(), 7, 1), ErrCreazyCanvasWorkActive)
	require.Nil(t, repo.works[1].DeletedAt)
	require.NoError(t, svc.DeleteWork(context.Background(), 7, 2))
	require.NotNil(t, repo.works[2].DeletedAt)
	require.NoError(t, svc.DeleteWork(context.Background(), 7, 3))
	require.NotNil(t, repo.works[3].DeletedAt)
	require.ErrorIs(t, svc.DeleteWork(context.Background(), 7, 4), ErrCreazyCanvasWorkActive)
	require.Nil(t, repo.works[4].DeletedAt)
}

func TestCreazyCanvasAdminCanAuditAndTerminateImageWork(t *testing.T) {
	repo := newCreazyCanvasWorkRepoStub()
	repo.works[1] = &CreazyCanvasWork{
		ID:              1,
		UserID:          7,
		APIKeyID:        9,
		Kind:            CreazyCanvasWorkKindImage,
		PublicModel:     "gpt-image-2",
		Status:          CreazyCanvasWorkStatusRunning,
		Prompt:          "audit this image",
		GatewayType:     CreazyCanvasGatewayImageTask,
		GatewayRemoteID: "imgtask_123",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(time.Hour),
	}
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{}, nil, nil)

	items, result, err := svc.AdminListImageWorks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, CreazyCanvasAdminWorkFilters{ActiveOnly: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, items, 1)
	require.Equal(t, "owner@example.com", items[0].UserEmail)

	terminated, err := svc.AdminTerminateImageWork(context.Background(), 1, "policy review")
	require.NoError(t, err)
	require.Equal(t, CreazyCanvasWorkStatusCanceled, terminated.Status)
	require.Equal(t, "policy review", terminated.ErrorMessage)

	completed := CreazyCanvasWorkStatusSucceeded
	_, err = svc.UpdateWork(context.Background(), UpdateCreazyCanvasWorkInput{UserID: 7, WorkID: 1, Status: &completed})
	require.ErrorIs(t, err, ErrCreazyCanvasWorkTerminated)
}

func creazyCanvasI64(v int64) *int64 { return &v }

func TestCreazyCanvasListKeysFiltersByAllowCreazyCanvas(t *testing.T) {
	groupOpen := &Group{ID: 1, Name: "open", Platform: PlatformSeedance, AllowCreazyCanvas: true}
	groupClosed := &Group{ID: 2, Name: "closed", Platform: PlatformSeedance, AllowCreazyCanvas: false}
	keys := []APIKey{
		{ID: 11, UserID: 7, Name: "k-open", Status: StatusAPIKeyActive, GroupID: creazyCanvasI64(1), Group: groupOpen},
		{ID: 12, UserID: 7, Name: "k-closed", Status: StatusAPIKeyActive, GroupID: creazyCanvasI64(2), Group: groupClosed},
		{ID: 13, UserID: 7, Name: "k-nogroup", Status: StatusAPIKeyActive},
		{ID: 14, UserID: 8, Name: "other-user", Status: StatusAPIKeyActive, GroupID: creazyCanvasI64(1), Group: groupOpen},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{listKeys: keys, keys: map[int64]*APIKey{}}, nil, nil)

	items, err := svc.ListKeys(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(11), items[0].ID)
	require.True(t, items[0].AllowCreazyCanvas)
	require.Equal(t, PlatformSeedance, items[0].Platform)
}

func TestCreazyCanvasCatalogRequiresOpenGroup(t *testing.T) {
	groupID := int64(3)
	price720 := 0.02
	openGroup := &Group{
		ID:                   groupID,
		Name:                 "seedance",
		Platform:             PlatformSeedance,
		AllowCreazyCanvas:    true,
		AllowImageGeneration: false,
		VideoPrice720P:       &price720,
		VideoBillingUnit:     VideoBillingUnitPerSecond,
	}
	closedGroup := &Group{ID: 4, Name: "closed", Platform: PlatformSeedance, AllowCreazyCanvas: false}
	keys := map[int64]*APIKey{
		21: {ID: 21, UserID: 1, Status: StatusAPIKeyActive, GroupID: &groupID, Group: openGroup},
		22: {ID: 22, UserID: 1, Status: StatusAPIKeyActive, GroupID: creazyCanvasI64(4), Group: closedGroup},
		23: {ID: 23, UserID: 2, Status: StatusAPIKeyActive, GroupID: &groupID, Group: openGroup},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)

	catalog, err := svc.Catalog(context.Background(), 1, 21)
	require.NoError(t, err)
	require.Equal(t, int64(21), catalog.APIKeyID)
	require.Equal(t, PlatformSeedance, catalog.Platform)
	require.NotEmpty(t, catalog.VideoModels)
	require.Empty(t, catalog.ImageModels)
	foundSeedance := false
	for _, model := range catalog.VideoModels {
		require.NotEqual(t, SeedanceWeijin900Model, model.ID)
		if model.ID == "seedance-2.0" {
			foundSeedance = true
			require.Contains(t, model.AllowedResolutions, VideoBillingResolution720P)
			require.Equal(t, model.AllowedResolutions, model.Resolutions)
			require.NotNil(t, model.Prices[VideoBillingResolution720P])
			require.Equal(t, 4, model.MaxImageReferences)
			require.Equal(t, 3, model.MaxVideoReferences)
			require.Equal(t, 1, model.MaxAudioReferences)
			require.True(t, model.AllowStartFrame)
			require.True(t, model.AllowEndFrame)
		}
	}
	require.True(t, foundSeedance)
	// 933 系列合计上限 12
	found933 := false
	for _, model := range catalog.VideoModels {
		if model.ID == SeedanceMX933Model || model.ID == SeedanceMX933FastModel {
			found933 = true
			require.Equal(t, 12, model.MaxTotalMedia)
			require.Equal(t, 9, model.MaxImageReferences)
			require.Equal(t, 3, model.MaxVideoReferences)
			require.Equal(t, 3, model.MaxAudioReferences)
		}
	}
	require.True(t, found933)

	_, err = svc.Catalog(context.Background(), 1, 22)
	require.ErrorIs(t, err, ErrCreazyCanvasKeyNotAllowed)

	_, err = svc.Catalog(context.Background(), 1, 23)
	require.Error(t, err)
}

func TestCreazyCanvasCatalogFiltersByVideoModelPrices(t *testing.T) {
	groupID := int64(33)
	price720 := 0.03
	group := &Group{
		ID:                groupID,
		Name:              "seedance-priced",
		Platform:          PlatformSeedance,
		AllowCreazyCanvas: true,
		VideoBillingUnit:  VideoBillingUnitPerSecond,
		VideoModelPrices: VideoModelPrices{
			"seedance-2.0": {
				BillingUnit: VideoBillingUnitPerRequest,
				Price720P:   &price720,
			},
			// Legacy alias should surface as the public Huiqu model only when present.
			"sd2-mx933-720-1s": {
				Price720P: &price720,
			},
		},
	}
	keys := map[int64]*APIKey{
		41: {ID: 41, UserID: 1, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)

	catalog, err := svc.Catalog(context.Background(), 1, 41)
	require.NoError(t, err)

	ids := make([]string, 0, len(catalog.VideoModels))
	for _, model := range catalog.VideoModels {
		ids = append(ids, model.ID)
	}
	require.Contains(t, ids, "seedance-2.0")
	require.Contains(t, ids, SeedanceMX933Model)
	require.NotContains(t, ids, SeedanceMX933FastModel)
	require.NotContains(t, ids, "seedance-2.0-fast")
	require.NotContains(t, ids, SeedanceXimeiSD25Model)
	require.NotContains(t, ids, SeedanceMX933LegacyModel)
	for _, model := range catalog.VideoModels {
		if model.ID == "seedance-2.0" {
			require.Equal(t, VideoBillingUnitPerRequest, model.BillingUnit)
		}
	}

	// Deleting every matrix entry except one keeps the catalog tight.
	group.VideoModelPrices = VideoModelPrices{
		"seedance-2.0-mini": {Price720P: &price720},
	}
	catalog, err = svc.Catalog(context.Background(), 1, 41)
	require.NoError(t, err)
	require.Len(t, catalog.VideoModels, 1)
	require.Equal(t, "seedance-2.0-mini", catalog.VideoModels[0].ID)
}

func TestCreazyCanvasCatalogUsesUserVideoPriceWithoutChangingBillingUnit(t *testing.T) {
	groupID := int64(34)
	groupPrice := 0.05
	userPrice := 0.02
	group := &Group{
		ID: groupID, Platform: PlatformSeedance, AllowCreazyCanvas: true,
		VideoBillingUnit: VideoBillingUnitPerSecond,
		VideoModelPrices: VideoModelPrices{
			SeedanceWeijin900Model: {
				BillingUnit: VideoBillingUnitPerRequest,
				Price720P:   &groupPrice,
			},
		},
	}
	keys := map[int64]*APIKey{
		42: {ID: 42, UserID: 7, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	apiKeys := &creazyCanvasAPIKeyStub{
		keys: keys,
		videoPrices: VideoModelPrices{
			SeedanceWeijin900Model: {
				BillingUnit: VideoBillingUnitPerSecond,
				Price720P:   &userPrice,
			},
		},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), apiKeys, nil, nil)

	catalog, err := svc.Catalog(context.Background(), 7, 42)
	require.NoError(t, err)
	require.Len(t, catalog.VideoModels, 1)
	model := catalog.VideoModels[0]
	require.Equal(t, SeedanceWeijin900Model, model.ID)
	require.NotEqual(t, legacyWeijin900PublicModelForTest, model.ID)
	require.Equal(t, VideoBillingUnitPerRequest, model.BillingUnit)
	require.NotNil(t, model.Prices[VideoBillingResolution720P])
	require.InDelta(t, userPrice, *model.Prices[VideoBillingResolution720P], 1e-12)
	require.InDelta(t, groupPrice, *group.VideoModelPrices[SeedanceWeijin900Model].Price720P, 1e-12)
}

func TestCreazyCanvasCatalogAppliesUserPriceWithoutShrinkingLegacyCatalog(t *testing.T) {
	groupID := int64(35)
	legacyPrice := 0.08
	userPrice := 0.04
	group := &Group{
		ID: groupID, Platform: PlatformSeedance, AllowCreazyCanvas: true,
		VideoBillingUnit: VideoBillingUnitPerSecond,
		VideoPrice720P:   &legacyPrice,
	}
	keys := map[int64]*APIKey{
		43: {ID: 43, UserID: 8, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	apiKeys := &creazyCanvasAPIKeyStub{
		keys: keys,
		videoPrices: VideoModelPrices{
			"seedance-2.0": {Price720P: &userPrice},
		},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), apiKeys, nil, nil)

	catalog, err := svc.Catalog(context.Background(), 8, 43)
	require.NoError(t, err)
	require.Greater(t, len(catalog.VideoModels), 1)

	models := make(map[string]CreazyCanvasVideoModel, len(catalog.VideoModels))
	for _, model := range catalog.VideoModels {
		models[model.ID] = model
	}
	require.NotContains(t, models, SeedanceWeijin900Model)
	require.Contains(t, models, "seedance-2.0")
	require.Contains(t, models, "seedance-2.0-fast")
	require.InDelta(t, userPrice, *models["seedance-2.0"].Prices[VideoBillingResolution720P], 1e-12)
	require.InDelta(t, legacyPrice, *models["seedance-2.0-fast"].Prices[VideoBillingResolution720P], 1e-12)
	require.Equal(t, VideoBillingUnitPerSecond, models["seedance-2.0"].BillingUnit)
}

func TestCreazyCanvasCreateWorkAndDownloadHint(t *testing.T) {
	groupID := int64(9)
	group := &Group{
		ID:                   groupID,
		Name:                 "img",
		Platform:             PlatformGrok,
		AllowCreazyCanvas:    true,
		AllowImageGeneration: true,
	}
	keys := map[int64]*APIKey{
		31: {ID: 31, UserID: 5, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group},
	}
	repo := newCreazyCanvasWorkRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)

	work, err := svc.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:          5,
		APIKeyID:        31,
		Kind:            CreazyCanvasWorkKindImage,
		PublicModel:     "grok-imagine",
		Prompt:          "a cat",
		GatewayType:     CreazyCanvasGatewayImageTask,
		GatewayRemoteID: "imgtask_1",
		PreviewURL:      "https://example.com/a.png",
		Status:          CreazyCanvasWorkStatusSucceeded,
	})
	require.Equal(t, CreazyCanvasGatewayImageTask, work.GatewayType)
	require.Equal(t, "https://example.com/a.png", work.PreviewURL)
	require.NoError(t, err)
	require.Equal(t, int64(1), work.ID)
	require.Equal(t, CreazyCanvasWorkKindImage, work.Kind)
	require.WithinDuration(t, time.Now().Add(3*24*time.Hour), work.ExpiresAt, time.Minute)

	group.AllowImageGeneration = false
	keys[31].Group = group
	_, err = svc.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:   5,
		APIKeyID: 31,
		Kind:     CreazyCanvasWorkKindImage,
	})
	require.Error(t, err)

	group.AllowCreazyCanvas = true
	group.Platform = PlatformSeedance
	keys[31].Group = group
	videoWork, err := svc.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID:          5,
		APIKeyID:        31,
		Kind:            CreazyCanvasWorkKindVideo,
		PublicModel:     "seedance-2.0",
		Prompt:          "city night",
		GatewayType:     "seedance", // 兼容旧值，归一到 video_job
		GatewayRemoteID: "vidjob_abc",
		Status:          CreazyCanvasWorkStatusSucceeded,
	})
	require.NoError(t, err)
	require.Equal(t, CreazyCanvasGatewayVideoJob, videoWork.GatewayType)

	dl, err := svc.GetDownloadURL(context.Background(), 5, videoWork.ID)
	require.NoError(t, err)
	require.Equal(t, "session", dl.Source)
	require.Contains(t, dl.GatewayHint, "vidjob_abc")
	require.Contains(t, dl.GatewayHint, "/v1/videos/jobs/")
}

func TestCreazyCanvasImageCatalogByPlatform(t *testing.T) {
	price := 0.01
	group := &Group{
		ID:                   1,
		Platform:             PlatformGrok,
		AllowCreazyCanvas:    true,
		AllowImageGeneration: true,
		ImagePrice1K:         &price,
	}
	models := buildCreazyCanvasImageModels(group)
	require.NotEmpty(t, models)
	require.Equal(t, "grok-imagine", models[0].ID)
	require.Contains(t, models[0].Sizes, "1024x1024")
	require.False(t, models[0].Async)
	require.Equal(t, 1, models[0].MaxN)
	require.True(t, models[0].SupportsReference)
	require.Equal(t, 1, models[0].MaxReferenceImages)
	// edit models require a reference image
	var edit *CreazyCanvasImageModel
	for i := range models {
		if strings.Contains(models[i].ID, "edit") {
			edit = &models[i]
			break
		}
	}
	require.NotNil(t, edit)
	require.True(t, edit.RequireReference)
	require.NotNil(t, models[0].Prices["1K"])

	group.Platform = PlatformGemini
	models = buildCreazyCanvasImageModels(group)
	require.Contains(t, models[0].Sizes, "1K")

	group.AllowImageGeneration = false
	require.Empty(t, buildCreazyCanvasImageModels(group))
}

func TestCreazyCanvasOpenAIImageSizePolicy(t *testing.T) {
	price := 0.02
	group := &Group{
		ID:                   9,
		Platform:             PlatformOpenAI,
		AllowCreazyCanvas:    true,
		AllowImageGeneration: true,
		ImagePrice1K:         &price,
		ImagePrice2K:         &price,
		ImagePrice4K:         &price,
	}
	models := buildCreazyCanvasImageModels(group)
	require.NotEmpty(t, models)

	var img2, img1 *CreazyCanvasImageModel
	for i := range models {
		switch models[i].ID {
		case "gpt-image-2":
			img2 = &models[i]
		case "gpt-image-1":
			img1 = &models[i]
		}
	}
	require.NotNil(t, img2)
	require.NotNil(t, img1)

	// gpt-image-2: free-form with official constraints + 2K/4K presets.
	require.True(t, img2.AllowCustomSize)
	require.False(t, img2.Async)
	require.NotNil(t, img2.SizeConstraints)
	require.Equal(t, 3840, img2.SizeConstraints.MaxEdge)
	require.Equal(t, 16, img2.SizeConstraints.MultipleOf)
	require.Equal(t, 3.0, img2.SizeConstraints.MaxAspectRatio)
	require.EqualValues(t, 655360, img2.SizeConstraints.MinPixels)
	require.EqualValues(t, 8294400, img2.SizeConstraints.MaxPixels)
	require.Contains(t, img2.Sizes, "2048x2048")
	require.Contains(t, img2.Sizes, "3840x2160")
	require.Contains(t, img2.Sizes, "auto")
	require.Equal(t, []string{"1K", "2K", "4K"}, img2.QualityTiers)
	require.Contains(t, img2.AspectRatios, "1:1")
	require.Contains(t, img2.AspectRatios, "16:9")
	require.Contains(t, img2.AspectRatios, "9:16")
	require.Contains(t, img2.AspectRatios, "21:9")
	require.Contains(t, img2.AspectRatios, "9:21")

	// gpt-image-1: presets only.
	require.False(t, img1.AllowCustomSize)
	require.Nil(t, img1.SizeConstraints)
	require.Contains(t, img1.Sizes, "1024x1024")
	require.Contains(t, img1.Sizes, "auto")
	require.NotContains(t, img1.Sizes, "3840x2160")
	require.Equal(t, []string{"1K", "2K"}, img1.QualityTiers)
}

func TestCreazyCanvasGeminiImageControls(t *testing.T) {
	group := &Group{Platform: PlatformGemini, AllowImageGeneration: true}
	models := buildCreazyCanvasImageModels(group)
	require.NotEmpty(t, models)
	require.Equal(t, []string{"1K", "2K", "4K"}, models[0].QualityTiers)
	require.Contains(t, models[0].AspectRatios, "1:1")
	require.Contains(t, models[0].AspectRatios, "16:9")
	require.Contains(t, models[0].AspectRatios, "9:16")
}

func TestValidateCreazyCanvasImageSizeGPTImage2(t *testing.T) {
	model := &CreazyCanvasImageModel{
		ID:              "gpt-image-2",
		Sizes:           []string{"1024x1024", "1536x1024", "1024x1536", "2048x2048", "3840x2160", "auto"},
		AllowCustomSize: true,
		SizeConstraints: &CreazyCanvasImageSizeConstraints{
			MaxEdge:        3840,
			MultipleOf:     16,
			MaxAspectRatio: 3,
			MinPixels:      655360,
			MaxPixels:      8294400,
			Aliases:        []string{"auto"},
		},
	}

	valid := []string{
		"1024x1024",
		"1536x864", // 16 multiple, pixels/ratio ok
		"3840x2160",
		"auto",
		"AUTO",
	}
	for _, size := range valid {
		require.Truef(t, ValidateCreazyCanvasImageSize(model, size), "expected valid: %s", size)
	}

	invalid := []string{
		"1000x1000", // not multiple of 16
		"64x64",     // below min pixels
		"5000x5000", // over max edge and pixels
		"3840x960",  // 4:1 > 3:1
		"1K",        // not an openai free-form alias for gpt-image-2
		"2K",
		"",
	}
	for _, size := range invalid {
		require.Falsef(t, ValidateCreazyCanvasImageSize(model, size), "expected invalid: %s", size)
	}

	// gpt-image-1 rejects free-form even if geometrically valid.
	img1 := &CreazyCanvasImageModel{
		ID:              "gpt-image-1",
		Sizes:           []string{"1024x1024", "1536x1024", "1024x1536", "auto"},
		AllowCustomSize: false,
	}
	require.True(t, ValidateCreazyCanvasImageSize(img1, "1024x1024"))
	require.True(t, ValidateCreazyCanvasImageSize(img1, "auto"))
	require.False(t, ValidateCreazyCanvasImageSize(img1, "1536x864"))
	require.False(t, ValidateCreazyCanvasImageSize(img1, "2048x2048"))
}

func TestDescribeCreazyCanvasImageSizeInvalidGPTImage2(t *testing.T) {
	model := &CreazyCanvasImageModel{
		ID:              "gpt-image-2",
		Sizes:           []string{"1024x1024", "1536x1024", "1024x1536", "auto"},
		AllowCustomSize: true,
		SizeConstraints: &CreazyCanvasImageSizeConstraints{
			MultipleOf:     16,
			MaxEdge:        3840,
			MinPixels:      655360,
			MaxPixels:      8294400,
			MaxAspectRatio: 3,
		},
	}
	require.Equal(t, "", DescribeCreazyCanvasImageSizeInvalid(model, "1024x1024"))
	msg := DescribeCreazyCanvasImageSizeInvalid(model, "1000x1000")
	require.Contains(t, msg, "multiples of 16")
	msg = DescribeCreazyCanvasImageSizeInvalid(model, "64x64")
	require.Contains(t, msg, "minimum")
	msg = DescribeCreazyCanvasImageSizeInvalid(model, "not-a-size")
	require.Contains(t, msg, "invalid size format")
}

func TestBuildCreazyCanvasObjectKeyPrefix(t *testing.T) {
	key := buildCreazyCanvasObjectKey(&CreazyCanvasWork{UserID: 7, ID: 3, Kind: "video"}, "out.mp4")
	require.True(t, strings.HasPrefix(key, "creazy-canvas/7/video/3/"))
	require.True(t, strings.HasSuffix(key, "-out.mp4"))
}

func TestCreazyCanvasCreateRejectsInvalidKind(t *testing.T) {
	groupID := int64(1)
	keys := map[int64]*APIKey{
		1: {
			ID: 1, UserID: 1, Status: StatusAPIKeyActive, GroupID: &groupID,
			Group: &Group{ID: 1, AllowCreazyCanvas: true, AllowImageGeneration: true, Platform: PlatformGrok},
		},
	}
	svc := NewCreazyCanvasService(newCreazyCanvasWorkRepoStub(), &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)
	_, err := svc.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID: 1, APIKeyID: 1, Kind: "audio",
	})
	require.Error(t, err)
}

func TestNormalizeCreazyCanvasGatewayType(t *testing.T) {
	require.Equal(t, CreazyCanvasGatewayVideoJob, normalizeCreazyCanvasGatewayType("seedance", CreazyCanvasWorkKindVideo))
	require.Equal(t, CreazyCanvasGatewayImageTask, normalizeCreazyCanvasGatewayType("images", CreazyCanvasWorkKindImage))
	require.Equal(t, CreazyCanvasGatewayImageSync, normalizeCreazyCanvasGatewayType("image_sync", CreazyCanvasWorkKindImage))
}

func TestCreazyCanvasDownloadRejectsExpired(t *testing.T) {
	groupID := int64(1)
	group := &Group{ID: groupID, AllowCreazyCanvas: true, AllowImageGeneration: true, Platform: PlatformGrok}
	keys := map[int64]*APIKey{1: {ID: 1, UserID: 1, Status: StatusAPIKeyActive, GroupID: &groupID, Group: group}}
	repo := newCreazyCanvasWorkRepoStub()
	svc := NewCreazyCanvasService(repo, &creazyCanvasAPIKeyStub{keys: keys}, nil, nil)
	work, err := svc.CreateWork(context.Background(), CreateCreazyCanvasWorkInput{
		UserID: 1, APIKeyID: 1, Kind: CreazyCanvasWorkKindImage, PublicModel: "grok-imagine", Status: CreazyCanvasWorkStatusSucceeded,
	})
	require.NoError(t, err)
	// force expire in stub
	stored := repo.works[work.ID]
	stored.ExpiresAt = time.Now().Add(-time.Hour)
	_, err = svc.GetDownloadURL(context.Background(), 1, work.ID)
	require.Error(t, err)
}

func TestDescribeImageSizeInvalidForGateway(t *testing.T) {
	// valid
	require.Equal(t, "", DescribeImageSizeInvalidForGateway("openai", "gpt-image-2", "1024x1024"))
	// empty size ignored
	require.Equal(t, "", DescribeImageSizeInvalidForGateway("openai", "gpt-image-2", ""))
	// multiple-of / pixels
	msg := DescribeImageSizeInvalidForGateway("openai", "gpt-image-2", "1000x1000")
	require.NotEmpty(t, msg)
	require.Contains(t, msg, "size")
}
