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
	keys     map[int64]*APIKey
	listKeys []APIKey
	quotaErr error
	getErr   error
	listErr  error
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

type creazyCanvasWorkRepoStub struct {
	nextID int64
	works  map[int64]*CreazyCanvasWork
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
	require.True(t, models[0].Async)
	require.Equal(t, 1, models[0].MaxN)
	require.False(t, models[0].SupportsReference)
	require.NotNil(t, models[0].Prices["1K"])

	group.Platform = PlatformGemini
	models = buildCreazyCanvasImageModels(group)
	require.Contains(t, models[0].Sizes, "1K")

	group.AllowImageGeneration = false
	require.Empty(t, buildCreazyCanvasImageModels(group))
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
