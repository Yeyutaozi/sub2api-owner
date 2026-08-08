package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	CreazyCanvasWorkKindImage = "image"
	CreazyCanvasWorkKindVideo = "video"

	CreazyCanvasWorkStatusCreated   = "created"
	CreazyCanvasWorkStatusQueued    = "queued"
	CreazyCanvasWorkStatusRunning   = "running"
	CreazyCanvasWorkStatusSucceeded = "succeeded"
	CreazyCanvasWorkStatusFailed    = "failed"
	CreazyCanvasWorkStatusCanceled  = "canceled"
	CreazyCanvasWorkStatusExpired   = "expired"

	creazyCanvasWorkTTL      = 3 * 24 * time.Hour
	creazyCanvasDownloadTTL  = time.Hour
	creazyCanvasObjectPrefix = "creazy-canvas"

	// 公开 gateway_type（作品元数据，非上游供应商名）
	CreazyCanvasGatewayImageTask = "image_task"
	CreazyCanvasGatewayImageSync = "image_sync"
	CreazyCanvasGatewayVideoJob  = "video_job"
)

var (
	ErrCreazyCanvasWorkNotFound = infraerrors.NotFound("CREAZY_CANVAS_WORK_NOT_FOUND", "作品不存在")
	ErrCreazyCanvasKeyNotAllowed = infraerrors.Forbidden("CREAZY_CANVAS_KEY_NOT_ALLOWED", "该 API Key 所属分组未开放 Creazy 画布")
)

type CreazyCanvasWork struct {
	ID               int64
	UserID           int64
	APIKeyID         int64
	GroupID          *int64
	Kind             string
	PublicModel      string
	Status           string
	Prompt           string
	ParamsJSON       map[string]any
	GatewayType      string
	GatewayRemoteID  string
	ObjectKey        string
	StorageProvider  string
	Bucket           string
	ObjectURL        string
	PreviewURL       string
	MimeType         string
	SizeBytes        int64
	ErrorMessage     string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type CreazyCanvasWorkListFilters struct {
	Kind     string
	Status   string
	APIKeyID *int64
}

type CreateCreazyCanvasWorkInput struct {
	UserID          int64
	APIKeyID        int64
	Kind            string
	PublicModel     string
	Prompt          string
	ParamsJSON      map[string]any
	GatewayType     string
	GatewayRemoteID string
	Status          string
	ErrorMessage    string
	PreviewURL      string
	ObjectURL       string
	MimeType        string
	SizeBytes       int64
}


type UpdateCreazyCanvasWorkInput struct {
	UserID          int64
	WorkID          int64
	Status          *string
	ErrorMessage    *string
	ParamsJSON      map[string]any
	GatewayType     *string
	GatewayRemoteID *string
	PreviewURL      *string
	ObjectURL       *string
	MimeType        *string
	SizeBytes       *int64
	PublicModel     *string
	Prompt          *string
}
type CreazyCanvasKeyInfo struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Status               string `json:"status"`
	GroupID              *int64 `json:"group_id,omitempty"`
	GroupName            string `json:"group_name,omitempty"`
	Platform             string `json:"platform,omitempty"`
	AllowCreazyCanvas    bool   `json:"allow_creazy_canvas"`
	AllowImageGeneration bool   `json:"allow_image_generation"`
}

type CreazyCanvasCatalog struct {
	APIKeyID             int64                    `json:"api_key_id"`
	GroupID              *int64                   `json:"group_id,omitempty"`
	Platform             string                   `json:"platform"`
	AllowImageGeneration bool                     `json:"allow_image_generation"`
	VideoModels          []CreazyCanvasVideoModel `json:"video_models"`
	ImageModels          []CreazyCanvasImageModel `json:"image_models"`
}

type CreazyCanvasVideoModel struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name,omitempty"`
	Platform            string              `json:"platform"`
	DefaultResolution   string              `json:"default_resolution"`
	DefaultDuration     int                 `json:"default_duration"`
	AllowedDurations    []int               `json:"allowed_durations"`
	Durations           []int               `json:"durations"`
	AllowedResolutions  []string            `json:"allowed_resolutions"`
	// resolutions/aspect_ratios: 前端常用字段别名
	Resolutions         []string            `json:"resolutions"`
	AllowedAspectRatios []string            `json:"allowed_aspect_ratios"`
	AspectRatios        []string            `json:"aspect_ratios"`
	Prices              map[string]*float64 `json:"prices"`
	BillingUnit         string              `json:"billing_unit"`
	AllowStartFrame     bool                `json:"allow_start_frame"`
	RequireStartFrame   bool                `json:"require_start_frame"`
	AllowEndFrame       bool                `json:"allow_end_frame"`
	AllowGeneratedAudio bool                `json:"allow_generated_audio"`
	// 参考素材上限（按模型；0 表示该类型不支持/不可上传）
	MaxImageReferences int `json:"max_image_references"`
	MaxVideoReferences int `json:"max_video_references"`
	MaxAudioReferences int `json:"max_audio_references"`
	// 全部参考合计上限；0 表示仅受分项上限约束
	MaxTotalMedia int `json:"max_total_media"`
	// 含首帧/尾帧在内的图片合计上限；0 表示仅受分项上限约束
	MaxTotalImages int `json:"max_total_images"`
	// 与上游校验对齐的模式约束（画布 UI 据此显隐/拦截）
	// 首帧/尾帧 与 参考图/参考音频 互斥（如 MiniMax H3）
	FramesExclusiveWithRefs bool `json:"frames_exclusive_with_refs"`
	// 参考音频必须搭配参考图（如 MiniMax H3）
	AudioRequiresImageRefs bool `json:"audio_requires_image_refs"`
	// 上游强制原生音频，忽略用户关闭（如 MiniMax H3）
	ForceGeneratedAudio bool `json:"force_generated_audio"`
	// Prompt 字符上限
	PromptLimit int `json:"prompt_limit"`
}

// CreazyCanvasImageSizeConstraints describes free-form WxH rules for a model
// (OpenAI gpt-image-2 style). Gateway billing still uses max-edge tiers separately.
type CreazyCanvasImageSizeConstraints struct {
	// MaxEdge is the max length of either side in pixels (gpt-image-2: 3840).
	MaxEdge int `json:"max_edge,omitempty"`
	// MultipleOf requires both sides to be divisible by this value (gpt-image-2: 16).
	MultipleOf int `json:"multiple_of,omitempty"`
	// MaxAspectRatio is long:short upper bound (gpt-image-2: 3).
	MaxAspectRatio float64 `json:"max_aspect_ratio,omitempty"`
	// MinPixels / MaxPixels bound total width*height (gpt-image-2: 655360..8294400).
	MinPixels int64 `json:"min_pixels,omitempty"`
	MaxPixels int64 `json:"max_pixels,omitempty"`
	// Aliases are non-WxH free-form tokens accepted when custom size is allowed (e.g. auto).
	Aliases []string `json:"aliases,omitempty"`
}

type CreazyCanvasImageModel struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name,omitempty"`
	Sizes              []string            `json:"sizes"`
	// AllowCustomSize marks free-form sizes beyond Sizes (WxH and/or SizeConstraints.Aliases).
	// Gateway bills by max-edge tier via ClassifyImageBillingTier.
	AllowCustomSize    bool                             `json:"allow_custom_size"`
	SizeConstraints    *CreazyCanvasImageSizeConstraints `json:"size_constraints,omitempty"`
	Prices             map[string]*float64              `json:"prices"`
	Async              bool                             `json:"async"`
	MaxN               int                              `json:"max_n"`
	SupportsReference  bool                             `json:"supports_reference"`
	MaxReferenceImages int                              `json:"max_reference_images,omitempty"`
	RequireReference   bool                             `json:"require_reference,omitempty"`
}

type CreazyCanvasDownloadURL struct {
	WorkID      int64  `json:"work_id"`
	URL         string `json:"url,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Source      string `json:"source"` // object | session | gateway
	GatewayHint string `json:"gateway_hint,omitempty"`
}

// CreazyCanvasWorkContent is a streamable work payload for JWT session playback.
// Callers must Close Body when non-nil.
type CreazyCanvasWorkContent struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	Filename      string
	Header        http.Header
	// RedirectURL is set when content is better served via HTTP redirect (public/object URL).
	RedirectURL string
}

type CreazyCanvasWorkRepository interface {
	Create(ctx context.Context, work *CreazyCanvasWork) error
	GetByIDForUser(ctx context.Context, id, userID int64) (*CreazyCanvasWork, error)
	ListByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters CreazyCanvasWorkListFilters) ([]CreazyCanvasWork, *pagination.PaginationResult, error)
	SoftDelete(ctx context.Context, id, userID int64) error
	UpdateContentMeta(ctx context.Context, work *CreazyCanvasWork) error
}

type CreazyCanvasAPIKeyService interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
	List(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
	CheckAPIKeyQuotaAndExpiry(apiKey *APIKey) error
}

type CreazyCanvasService struct {
	workRepo      CreazyCanvasWorkRepository
	apiKeyService CreazyCanvasAPIKeyService
	artifactStore AgentArtifactStore
	cfg           *config.Config
	httpClient    *http.Client
}

func NewCreazyCanvasService(
	workRepo CreazyCanvasWorkRepository,
	apiKeyService CreazyCanvasAPIKeyService,
	artifactStore AgentArtifactStore,
	cfg *config.Config,
) *CreazyCanvasService {
	if artifactStore == nil {
		artifactStore = disabledAgentArtifactStore{}
	}
	return &CreazyCanvasService{
		workRepo:      workRepo,
		apiKeyService: apiKeyService,
		artifactStore: artifactStore,
		cfg:           cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
			// Manual redirect handling so Authorization is not leaked to foreign hosts.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (s *CreazyCanvasService) ListKeys(ctx context.Context, userID int64) ([]CreazyCanvasKeyInfo, error) {
	if userID <= 0 {
		return nil, infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	// 拉取用户全部 active key（分页扫完），再按分组 AllowCreazyCanvas 过滤。
	out := make([]CreazyCanvasKeyInfo, 0)
	page := 1
	for {
		keys, result, err := s.apiKeyService.List(ctx, userID, pagination.PaginationParams{
			Page:     page,
			PageSize: 100,
			SortBy:   "id",
			SortOrder: pagination.SortOrderAsc,
		}, APIKeyListFilters{Status: StatusAPIKeyActive})
		if err != nil {
			return nil, err
		}
		for i := range keys {
			info, ok := creazyCanvasKeyInfoFromAPIKey(&keys[i])
			if !ok {
				continue
			}
			out = append(out, info)
		}
		if result == nil || page >= result.Pages || len(keys) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (s *CreazyCanvasService) Catalog(ctx context.Context, userID, apiKeyID int64) (*CreazyCanvasCatalog, error) {
	apiKey, err := s.loadCanvasAPIKey(ctx, userID, apiKeyID)
	if err != nil {
		return nil, err
	}
	group := apiKey.Group
	platform := ""
	groupID := apiKey.GroupID
	allowImage := false
	if group != nil {
		platform = group.Platform
		allowImage = group.AllowImageGeneration
	}

	catalog := &CreazyCanvasCatalog{
		APIKeyID:             apiKey.ID,
		GroupID:              groupID,
		Platform:             platform,
		AllowImageGeneration: allowImage,
		VideoModels:          buildCreazyCanvasVideoModels(group),
		ImageModels:          buildCreazyCanvasImageModels(group),
	}
	return catalog, nil
}

func (s *CreazyCanvasService) CreateWork(ctx context.Context, input CreateCreazyCanvasWorkInput) (*CreazyCanvasWork, error) {
	if input.UserID <= 0 {
		return nil, infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind != CreazyCanvasWorkKindImage && kind != CreazyCanvasWorkKindVideo {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_KIND_INVALID", "kind 必须是 image 或 video")
	}
	apiKey, err := s.loadCanvasAPIKey(ctx, input.UserID, input.APIKeyID)
	if err != nil {
		return nil, err
	}
	if kind == CreazyCanvasWorkKindImage {
		if apiKey.Group == nil || !apiKey.Group.AllowImageGeneration {
			return nil, infraerrors.Forbidden("CREAZY_CANVAS_IMAGE_NOT_ALLOWED", "该分组未开放图片生成")
		}
	}
	if kind == CreazyCanvasWorkKindVideo && apiKey.Group != nil && !IsFFLinkVideoPlatform(apiKey.Group.Platform) {
		// 非 FFLink 视频平台（seedance/ltx/happyhorse/minimax）也可记录元数据，但 catalog 不会给视频模型。
		// 这里不强制拦截，方便后续扩展。
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = CreazyCanvasWorkStatusCreated
	}
	if !isCreazyCanvasWorkStatus(status) {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_STATUS_INVALID", "无效的 status")
	}
	now := time.Now()
	work := &CreazyCanvasWork{
		UserID:          input.UserID,
		APIKeyID:        apiKey.ID,
		GroupID:         apiKey.GroupID,
		Kind:            kind,
		PublicModel:     strings.TrimSpace(input.PublicModel),
		Status:          status,
		Prompt:          strings.TrimSpace(input.Prompt),
		ParamsJSON:      input.ParamsJSON,
		GatewayType:     normalizeCreazyCanvasGatewayType(input.GatewayType, kind),
		GatewayRemoteID: strings.TrimSpace(input.GatewayRemoteID),
		ErrorMessage:    strings.TrimSpace(input.ErrorMessage),
		PreviewURL:      sanitizeCreazyCanvasMediaURL(input.PreviewURL),
		ObjectURL:       sanitizeCreazyCanvasMediaURL(input.ObjectURL),
		MimeType:        strings.TrimSpace(input.MimeType),
		SizeBytes:       input.SizeBytes,
		ExpiresAt:       now.Add(creazyCanvasWorkTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if work.ParamsJSON == nil {
		work.ParamsJSON = map[string]any{}
	}
	if err := s.workRepo.Create(ctx, work); err != nil {
		return nil, err
	}
	applyCreazyCanvasWorkView(work)
	return work, nil
}


func (s *CreazyCanvasService) UpdateWork(ctx context.Context, input UpdateCreazyCanvasWorkInput) (*CreazyCanvasWork, error) {
	if input.UserID <= 0 {
		return nil, infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	if input.WorkID <= 0 {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_ID_REQUIRED", "作品 ID 无效")
	}
	work, err := s.workRepo.GetByIDForUser(ctx, input.WorkID, input.UserID)
	if err != nil {
		return nil, err
	}
	if input.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*input.Status))
		if status == "" {
			return nil, infraerrors.BadRequest("CREAZY_CANVAS_STATUS_INVALID", "无效的 status")
		}
		if !isCreazyCanvasWorkStatus(status) {
			return nil, infraerrors.BadRequest("CREAZY_CANVAS_STATUS_INVALID", "无效的 status")
		}
		work.Status = status
	}
	if input.ErrorMessage != nil {
		work.ErrorMessage = strings.TrimSpace(*input.ErrorMessage)
	}
	if input.ParamsJSON != nil {
		work.ParamsJSON = input.ParamsJSON
	}
	if work.ParamsJSON == nil {
		work.ParamsJSON = map[string]any{}
	}
	if input.GatewayType != nil {
		work.GatewayType = normalizeCreazyCanvasGatewayType(*input.GatewayType, work.Kind)
	}
	if input.GatewayRemoteID != nil {
		work.GatewayRemoteID = strings.TrimSpace(*input.GatewayRemoteID)
	}
	if input.PreviewURL != nil {
		work.PreviewURL = sanitizeCreazyCanvasMediaURL(*input.PreviewURL)
	}
	if input.ObjectURL != nil {
		work.ObjectURL = sanitizeCreazyCanvasMediaURL(*input.ObjectURL)
	}
	if input.MimeType != nil {
		work.MimeType = strings.TrimSpace(*input.MimeType)
	}
	if input.SizeBytes != nil {
		work.SizeBytes = *input.SizeBytes
	}
	if input.PublicModel != nil {
		work.PublicModel = strings.TrimSpace(*input.PublicModel)
	}
	if input.Prompt != nil {
		work.Prompt = strings.TrimSpace(*input.Prompt)
	}
	// Always sanitize stored media fields before persist.
	work.PreviewURL = sanitizeCreazyCanvasMediaURL(work.PreviewURL)
	work.ObjectURL = sanitizeCreazyCanvasMediaURL(work.ObjectURL)
	if err := s.workRepo.UpdateContentMeta(ctx, work); err != nil {
		return nil, err
	}
	applyCreazyCanvasWorkView(work)
	return work, nil
}

func (s *CreazyCanvasService) ListWorks(ctx context.Context, userID int64, params pagination.PaginationParams, filters CreazyCanvasWorkListFilters) ([]CreazyCanvasWork, *pagination.PaginationResult, error) {
	if userID <= 0 {
		return nil, nil, infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	filters.Kind = strings.ToLower(strings.TrimSpace(filters.Kind))
	filters.Status = strings.ToLower(strings.TrimSpace(filters.Status))
	items, result, err := s.workRepo.ListByUser(ctx, userID, params, filters)
	if err != nil {
		return nil, nil, err
	}
	for i := range items {
		applyCreazyCanvasWorkView(&items[i])
	}
	return items, result, nil
}

func (s *CreazyCanvasService) GetWork(ctx context.Context, userID, workID int64) (*CreazyCanvasWork, error) {
	if userID <= 0 {
		return nil, infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	if workID <= 0 {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_ID_INVALID", "无效的作品 ID")
	}
	work, err := s.workRepo.GetByIDForUser(ctx, workID, userID)
	if err != nil {
		return nil, err
	}
	applyCreazyCanvasWorkView(work)
	return work, nil
}

func (s *CreazyCanvasService) DeleteWork(ctx context.Context, userID, workID int64) error {
	if userID <= 0 {
		return infraerrors.Unauthorized("CREAZY_CANVAS_USER_REQUIRED", "需要登录")
	}
	if workID <= 0 {
		return infraerrors.BadRequest("CREAZY_CANVAS_WORK_ID_INVALID", "无效的作品 ID")
	}
	return s.workRepo.SoftDelete(ctx, workID, userID)
}

func (s *CreazyCanvasService) GetDownloadURL(ctx context.Context, userID, workID int64) (*CreazyCanvasDownloadURL, error) {
	work, err := s.GetWork(ctx, userID, workID)
	if err != nil {
		return nil, err
	}
	if work.Status == CreazyCanvasWorkStatusExpired || (!work.ExpiresAt.IsZero() && time.Now().After(work.ExpiresAt)) {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_EXPIRED", "作品已过期，无法下载")
	}
	if work.Status == CreazyCanvasWorkStatusFailed {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_NOT_READY", "作品未成功生成，无法下载")
	}
	if strings.TrimSpace(work.ObjectKey) != "" && s.artifactStore != nil && s.artifactStore.IsConfigured() {
		ttl := creazyCanvasDownloadTTL
		url, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{
			StorageProvider: work.StorageProvider,
			Bucket:          work.Bucket,
			ObjectKey:       work.ObjectKey,
		}, ttl)
		if err != nil {
			return nil, infraerrors.InternalServer("CREAZY_CANVAS_PRESIGN_FAILED", "生成下载链接失败: "+err.Error())
		}
		return &CreazyCanvasDownloadURL{
			WorkID:    work.ID,
			URL:       url,
			ExpiresAt: time.Now().Add(ttl).Format(time.RFC3339),
			Source:    "object",
		}, nil
	}
	// Prefer session proxy: owner is already JWT-authenticated; frontend does not need
	// to re-paste the API key secret just to replay a succeeded work.
	if creazyCanvasWorkHasGatewayContent(work) {
		return &CreazyCanvasDownloadURL{
			WorkID:      work.ID,
			URL:         fmt.Sprintf("/creazy-canvas/works/%d/content", work.ID),
			Source:      "session",
			GatewayHint: creazyCanvasGatewayContentHint(work),
		}, nil
	}
	if url := creazyCanvasPublicMediaURL(work); url != "" {
		return &CreazyCanvasDownloadURL{
			WorkID: work.ID,
			URL:    url,
			Source: "object",
		}, nil
	}
	hint := creazyCanvasGatewayContentHint(work)
	return &CreazyCanvasDownloadURL{
		WorkID:      work.ID,
		Source:      "gateway",
		GatewayHint: hint,
	}, nil
}

// OpenWorkContent streams work media for the owning user using the stored API key
// and platform gateway (loopback). Generation still requires client-held secret;
// replaying already-created works does not.
func (s *CreazyCanvasService) OpenWorkContent(ctx context.Context, userID, workID int64, rangeHeader string) (*CreazyCanvasWorkContent, error) {
	work, err := s.GetWork(ctx, userID, workID)
	if err != nil {
		return nil, err
	}
	if work.Status == CreazyCanvasWorkStatusExpired || (!work.ExpiresAt.IsZero() && time.Now().After(work.ExpiresAt)) {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_EXPIRED", "作品已过期，无法预览")
	}
	if work.Status == CreazyCanvasWorkStatusFailed {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_NOT_READY", "作品未成功生成，无法预览")
	}
	if work.Status != CreazyCanvasWorkStatusSucceeded && work.Status != CreazyCanvasWorkStatusCreated {
		if creazyCanvasPublicMediaURL(work) == "" && !creazyCanvasWorkHasGatewayContent(work) {
			return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_NOT_READY", "作品尚未就绪，无法预览")
		}
	}

	// 1) COS / object storage
	if strings.TrimSpace(work.ObjectKey) != "" && s.artifactStore != nil && s.artifactStore.IsConfigured() {
		ttl := creazyCanvasDownloadTTL
		url, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{
			StorageProvider: work.StorageProvider,
			Bucket:          work.Bucket,
			ObjectKey:       work.ObjectKey,
		}, ttl)
		if err != nil {
			return nil, infraerrors.InternalServer("CREAZY_CANVAS_PRESIGN_FAILED", "生成预览链接失败: "+err.Error())
		}
		return &CreazyCanvasWorkContent{
			StatusCode:  http.StatusFound,
			RedirectURL: url,
			Filename:    creazyCanvasContentFilename(work),
		}, nil
	}

	// 2) Public absolute media URL stored on the work
	if url := creazyCanvasPublicMediaURL(work); url != "" {
		return &CreazyCanvasWorkContent{
			StatusCode:  http.StatusFound,
			RedirectURL: url,
			Filename:    creazyCanvasContentFilename(work),
		}, nil
	}

	// 3) Gateway content via stored API key (loopback to this server)
	if !creazyCanvasWorkHasGatewayContent(work) {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_CONTENT_UNAVAILABLE", "作品没有可预览的内容")
	}
	apiKey, err := s.loadCanvasAPIKey(ctx, userID, work.APIKeyID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey.Key) == "" {
		return nil, infraerrors.InternalServer("CREAZY_CANVAS_API_KEY_EMPTY", "API Key 密钥不可用")
	}
	gatewayPath := creazyCanvasGatewayContentPath(work)
	if gatewayPath == "" {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_CONTENT_UNAVAILABLE", "无法解析网关内容路径")
	}
	return s.openGatewayContent(ctx, apiKey.Key, gatewayPath, rangeHeader, work)
}

func (s *CreazyCanvasService) openGatewayContent(ctx context.Context, apiKeySecret, gatewayPath, rangeHeader string, work *CreazyCanvasWork) (*CreazyCanvasWorkContent, error) {
	baseURL := s.gatewayLoopbackBaseURL()
	target := strings.TrimRight(baseURL, "/") + gatewayPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("CREAZY_CANVAS_CONTENT_REQUEST_FAILED", "构建预览请求失败")
	}
	req.Header.Set("Authorization", "Bearer "+apiKeySecret)
	req.Header.Set("Accept", "*/*")
	if rh := strings.TrimSpace(rangeHeader); rh != "" {
		req.Header.Set("Range", rh)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, infraerrors.InternalServer("CREAZY_CANVAS_CONTENT_UPSTREAM_FAILED", "预览内容拉取失败: "+err.Error())
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && isCreazyCanvasJSONContentType(contentType) {
		defer func() { _ = resp.Body.Close() }()
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return nil, infraerrors.InternalServer("CREAZY_CANVAS_CONTENT_READ_FAILED", "读取预览元数据失败")
		}
		var payload map[string]any
		if json.Unmarshal(raw, &payload) == nil {
			if nested := creazyCanvasExtractMediaURL(payload); nested != "" {
				if strings.HasPrefix(nested, "/") {
					return s.openGatewayContent(ctx, apiKeySecret, nested, rangeHeader, work)
				}
				if strings.HasPrefix(nested, "http://") || strings.HasPrefix(nested, "https://") {
					return &CreazyCanvasWorkContent{
						StatusCode:  http.StatusFound,
						RedirectURL: nested,
						Filename:    creazyCanvasContentFilename(work),
					}, nil
				}
			}
		}
		return nil, infraerrors.New(http.StatusBadGateway, "CREAZY_CANVAS_CONTENT_INVALID", "网关返回了无法解析的预览元数据")
	}

	if resp.StatusCode == http.StatusMovedPermanently ||
		resp.StatusCode == http.StatusFound ||
		resp.StatusCode == http.StatusTemporaryRedirect ||
		resp.StatusCode == http.StatusPermanentRedirect {
		loc := strings.TrimSpace(resp.Header.Get("Location"))
		_ = resp.Body.Close()
		if loc == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "CREAZY_CANVAS_CONTENT_INVALID", "网关重定向缺少 Location")
		}
		if strings.HasPrefix(loc, "/") {
			return s.openGatewayContent(ctx, apiKeySecret, loc, rangeHeader, work)
		}
		return &CreazyCanvasWorkContent{
			StatusCode:  http.StatusFound,
			RedirectURL: loc,
			Filename:    creazyCanvasContentFilename(work),
		}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = fmt.Sprintf("upstream status %d", resp.StatusCode)
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, infraerrors.New(resp.StatusCode, "CREAZY_CANVAS_CONTENT_UPSTREAM_ERROR", "预览内容拉取失败: "+msg)
	}

	cl := int64(-1)
	if raw := strings.TrimSpace(resp.Header.Get("Content-Length")); raw != "" {
		if n, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			cl = n
		}
	}
	filename := creazyCanvasContentFilename(work)
	return &CreazyCanvasWorkContent{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentType:   contentType,
		ContentLength: cl,
		Filename:      filename,
		Header:        resp.Header.Clone(),
	}, nil
}

func (s *CreazyCanvasService) gatewayLoopbackBaseURL() string {
	host := "127.0.0.1"
	port := 8080
	if s != nil && s.cfg != nil {
		host = creazyCanvasLoopbackHost(s.cfg.Server.Host)
		if s.cfg.Server.Port > 0 {
			port = s.cfg.Server.Port
		}
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func creazyCanvasLoopbackHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return host
	}
}

func creazyCanvasWorkHasGatewayContent(work *CreazyCanvasWork) bool {
	if work == nil {
		return false
	}
	remoteID := strings.TrimSpace(work.GatewayRemoteID)
	if remoteID == "" {
		return false
	}
	gt := normalizeCreazyCanvasGatewayType(work.GatewayType, work.Kind)
	return gt == CreazyCanvasGatewayVideoJob || gt == CreazyCanvasGatewayImageTask
}

func creazyCanvasGatewayContentPath(work *CreazyCanvasWork) string {
	if work == nil {
		return ""
	}
	remoteID := strings.TrimSpace(work.GatewayRemoteID)
	if remoteID == "" {
		return ""
	}
	gt := normalizeCreazyCanvasGatewayType(work.GatewayType, work.Kind)
	switch gt {
	case CreazyCanvasGatewayVideoJob:
		return "/v1/videos/jobs/" + urlPathEscape(remoteID) + "/content"
	case CreazyCanvasGatewayImageTask:
		return "/v1/images/tasks/" + urlPathEscape(remoteID)
	default:
		return ""
	}
}

func urlPathEscape(segment string) string {
	return strings.ReplaceAll(strings.TrimSpace(segment), " ", "%20")
}

// sanitizeCreazyCanvasMediaURL drops broken placeholders such as bare "b64".
func sanitizeCreazyCanvasMediaURL(raw string) string {
	c := strings.TrimSpace(raw)
	if c == "" {
		return ""
	}
	lower := strings.ToLower(c)
	if lower == "b64" || lower == "b64_json" || lower == "null" || lower == "undefined" || lower == "-" {
		return ""
	}
	if strings.HasPrefix(c, "data:") {
		// Reject incomplete data URLs without a comma payload separator.
		if !strings.Contains(c, ",") {
			return ""
		}
		return c
	}
	if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
		return c
	}
	// Relative gateway paths are kept for storage; not treated as public media.
	if strings.HasPrefix(c, "/") {
		return c
	}
	return ""
}

func creazyCanvasIsPublicAbsoluteMediaURL(c string) bool {
	c = strings.TrimSpace(c)
	if c == "" {
		return false
	}
	if strings.HasPrefix(c, "data:") {
		return strings.Contains(c, ",")
	}
	if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
		if strings.Contains(c, "/v1/videos/jobs/") && strings.Contains(c, "/content") {
			return false
		}
		if strings.Contains(c, "/v1/images/") {
			return false
		}
		return true
	}
	return false
}

func creazyCanvasPublicMediaURL(work *CreazyCanvasWork) string {
	if work == nil {
		return ""
	}
	candidates := []string{
		strings.TrimSpace(work.ObjectURL),
		strings.TrimSpace(work.PreviewURL),
	}
	if work.ParamsJSON != nil {
		for _, key := range []string{"poster_url", "result_url", "video_url", "content_url"} {
			if v, ok := work.ParamsJSON[key].(string); ok {
				candidates = append(candidates, strings.TrimSpace(v))
			}
		}
		if arr, ok := work.ParamsJSON["result_urls"].([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					candidates = append(candidates, strings.TrimSpace(s))
				}
			}
		}
	}
	for _, c := range candidates {
		c = sanitizeCreazyCanvasMediaURL(c)
		if c == "" {
			continue
		}
		if creazyCanvasIsPublicAbsoluteMediaURL(c) {
			return c
		}
	}
	return ""
}

func creazyCanvasContentFilename(work *CreazyCanvasWork) string {
	if work == nil {
		return "creazy-content.bin"
	}
	if work.Kind == CreazyCanvasWorkKindVideo {
		return fmt.Sprintf("creazy-work-%d.mp4", work.ID)
	}
	if work.Kind == CreazyCanvasWorkKindImage {
		return fmt.Sprintf("creazy-work-%d.png", work.ID)
	}
	return fmt.Sprintf("creazy-work-%d.bin", work.ID)
}

func isCreazyCanvasJSONContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	return strings.Contains(ct, "application/json") || strings.Contains(ct, "+json")
}

func creazyCanvasExtractMediaURL(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	keys := []string{"url", "content_url", "video_url", "download_url", "mp4_url", "image_url", "result_url"}
	for _, k := range keys {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	for _, nestKey := range []string{"data", "result", "output", "video", "image"} {
		if nested, ok := payload[nestKey].(map[string]any); ok {
			if u := creazyCanvasExtractMediaURL(nested); u != "" {
				return u
			}
		}
	}
	if data, ok := payload["data"].([]any); ok {
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				if u := creazyCanvasExtractMediaURL(m); u != "" {
					return u
				}
			}
		}
	}
	return ""
}


// SaveWorkContent 可选：将生成内容转存到 COS（前缀 creazy-canvas/）。
func (s *CreazyCanvasService) SaveWorkContent(ctx context.Context, userID, workID int64, body io.Reader, contentType string, sizeBytes int64, filename string) (*CreazyCanvasWork, error) {
	work, err := s.GetWork(ctx, userID, workID)
	if err != nil {
		return nil, err
	}
	if s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	if body == nil {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_CONTENT_REQUIRED", "内容不能为空")
	}
	objectKey := buildCreazyCanvasObjectKey(work, filename)
	put, err := s.artifactStore.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        body,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
	})
	if err != nil {
		return nil, err
	}
	work.ObjectKey = put.ObjectKey
	work.StorageProvider = put.Provider
	work.Bucket = put.Bucket
	work.ObjectURL = put.ObjectURL
	work.MimeType = strings.TrimSpace(contentType)
	if sizeBytes > 0 {
		work.SizeBytes = sizeBytes
	} else {
		work.SizeBytes = put.SizeBytes
	}
	if work.Status == CreazyCanvasWorkStatusCreated || work.Status == CreazyCanvasWorkStatusRunning || work.Status == CreazyCanvasWorkStatusQueued {
		work.Status = CreazyCanvasWorkStatusSucceeded
	}
	if err := s.workRepo.UpdateContentMeta(ctx, work); err != nil {
		return nil, err
	}
	return work, nil
}

func (s *CreazyCanvasService) loadCanvasAPIKey(ctx context.Context, userID, apiKeyID int64) (*APIKey, error) {
	if apiKeyID <= 0 {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_API_KEY_REQUIRED", "请选择 API Key")
	}
	apiKey, err := s.apiKeyService.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != userID {
		return nil, infraerrors.NotFound("CREAZY_CANVAS_API_KEY_NOT_FOUND", "API Key 不存在")
	}
	if !apiKey.IsActive() {
		return nil, infraerrors.Forbidden("CREAZY_CANVAS_API_KEY_INACTIVE", "API Key 未启用")
	}
	if err := s.apiKeyService.CheckAPIKeyQuotaAndExpiry(apiKey); err != nil {
		return nil, err
	}
	if apiKey.Group == nil || !apiKey.Group.AllowCreazyCanvas {
		return nil, ErrCreazyCanvasKeyNotAllowed
	}
	return apiKey, nil
}

func creazyCanvasKeyInfoFromAPIKey(apiKey *APIKey) (CreazyCanvasKeyInfo, bool) {
	if apiKey == nil {
		return CreazyCanvasKeyInfo{}, false
	}
	info := CreazyCanvasKeyInfo{
		ID:     apiKey.ID,
		Name:   apiKey.Name,
		Status: apiKey.Status,
	}
	if apiKey.GroupID != nil {
		info.GroupID = apiKey.GroupID
	}
	if apiKey.Group != nil {
		if !apiKey.Group.AllowCreazyCanvas {
			return CreazyCanvasKeyInfo{}, false
		}
		// Only expose keys whose group can actually generate images and/or videos.
		if !groupHasCreazyGenerationCapability(apiKey.Group) {
			return CreazyCanvasKeyInfo{}, false
		}
		info.GroupName = apiKey.Group.Name
		info.Platform = apiKey.Group.Platform
		info.AllowCreazyCanvas = apiKey.Group.AllowCreazyCanvas
		info.AllowImageGeneration = apiKey.Group.AllowImageGeneration
		return info, true
	}
	// 无分组 key 默认不可用于画布（画布依赖分组能力开关）
	return CreazyCanvasKeyInfo{}, false
}

func buildCreazyCanvasVideoModels(group *Group) []CreazyCanvasVideoModel {
	if group == nil || !IsFFLinkVideoPlatform(group.Platform) {
		return []CreazyCanvasVideoModel{}
	}
	modelIDs := FFLinkVideoModelIDsForPlatform(group.Platform)
	out := make([]CreazyCanvasVideoModel, 0, len(modelIDs))
	billingUnit := group.EffectiveVideoBillingUnit()
	for _, modelID := range modelIDs {
		profile, ok := ffLinkVideoModelProfileFor(modelID)
		if !ok {
			continue
		}
		// 价格：优先 VideoModelPrices，否则回落 legacy 分组级分辨率价
		prices := map[string]*float64{}
		resolutions := sortedStringKeys(profile.AllowedResolutions)
		for _, res := range resolutions {
			prices[res] = group.GetVideoPriceForModel(modelID, res)
		}
		aspects := sortedStringKeys(profile.AllowedAspectRatios)
		durations := creazyCanvasDurationsForProfile(profile)
		// 若矩阵存在但该模型完全无价，仍返回模型（前端可提示未配置）
		h3Like := isHuiquMiniMaxH3Model(modelID)
		out = append(out, CreazyCanvasVideoModel{
			ID:                      modelID,
			Name:                    modelID,
			Platform:                profile.Platform,
			DefaultResolution:       profile.DefaultResolution,
			DefaultDuration:         profile.DefaultDuration,
			AllowedDurations:        durations,
			Durations:               append([]int(nil), durations...),
			AllowedResolutions:      resolutions,
			Resolutions:             append([]string(nil), resolutions...),
			AllowedAspectRatios:     aspects,
			AspectRatios:            append([]string(nil), aspects...),
			Prices:                  prices,
			BillingUnit:             billingUnit,
			AllowStartFrame:         profile.AllowStartFrame,
			RequireStartFrame:       profile.RequireStartFrame,
			AllowEndFrame:           profile.AllowEndFrame,
			AllowGeneratedAudio:     profile.AllowGeneratedAudio,
			MaxImageReferences:      profile.MaxImageReferences,
			MaxVideoReferences:      profile.MaxVideoReferences,
			MaxAudioReferences:      profile.MaxAudioReferences,
			MaxTotalMedia:           profile.MaxTotalMedia,
			MaxTotalImages:          profile.MaxTotalImages,
			FramesExclusiveWithRefs: h3Like,
			AudioRequiresImageRefs:  h3Like,
			ForceGeneratedAudio:     h3Like,
			PromptLimit:             profile.PromptLimit,
		})
	}
	return out
}

func buildCreazyCanvasImageModels(group *Group) []CreazyCanvasImageModel {
	if group == nil || !group.AllowImageGeneration {
		return []CreazyCanvasImageModel{}
	}
	modelIDs := defaultCreazyCanvasImageModels(group.Platform)
	out := make([]CreazyCanvasImageModel, 0, len(modelIDs))
	for _, id := range modelIDs {
		sizes, allowCustom, constraints := creazyCanvasImageSizePolicy(group.Platform, id)
		prices := map[string]*float64{}
		for _, size := range sizes {
			tier := creazyCanvasImageSizeTier(size)
			prices[size] = group.GetImagePrice(tier)
		}
		// Always surface billing-tier keys for UI estimates.
		prices["1K"] = group.GetImagePrice("1K")
		prices["2K"] = group.GetImagePrice("2K")
		prices["4K"] = group.GetImagePrice("4K")
		supportsRef, maxRefs, requireRef := creazyCanvasImageReferenceCapability(group.Platform, id)
		out = append(out, CreazyCanvasImageModel{
			ID:                 id,
			Name:               id,
			Sizes:              sizes,
			AllowCustomSize:    allowCustom,
			SizeConstraints:    constraints,
			Prices:             prices,
			Async:              true,
			MaxN:               1,
			SupportsReference:  supportsRef,
			MaxReferenceImages: maxRefs,
			RequireReference:   requireRef,
		})
	}
	return out
}

// creazyCanvasImageSizePolicy returns suggested presets, whether free-form size is
// allowed, and official geometric constraints (when applicable).
//
// OpenAI gpt-image-2 free-form size rules (Image generation guide):
//   - max edge <= 3840
//   - both edges multiples of 16
//   - long:short <= 3:1
//   - total pixels in [655360, 8294400]
// gpt-image-1 only accepts fixed presets (+ auto), not arbitrary WxH.
func creazyCanvasImageSizePolicy(platform, modelID string) (sizes []string, allowCustom bool, constraints *CreazyCanvasImageSizeConstraints) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch platform {
	case PlatformOpenAI:
		switch {
		case strings.HasPrefix(id, "gpt-image-2"):
			sizes = []string{
				"1024x1024", "1536x1024", "1024x1536",
				"2048x2048", "2048x1152", "1152x2048",
				"3840x2160", "2160x3840",
				"auto",
			}
			allowCustom = true
			constraints = &CreazyCanvasImageSizeConstraints{
				MaxEdge:        3840,
				MultipleOf:     16,
				MaxAspectRatio: 3,
				MinPixels:      655360,
				MaxPixels:      8294400,
				Aliases:        []string{"auto"},
			}
			return sizes, allowCustom, constraints
		case strings.HasPrefix(id, "gpt-image-1"):
			// gpt-image-1 / 1.5 family: fixed presets only.
			return []string{"1024x1024", "1536x1024", "1024x1536", "auto"}, false, nil
		default:
			return creazyCanvasImageSizesForPlatform(platform), false, nil
		}
	case PlatformGrok:
		// Grok image sizes are preset-only in canvas catalog.
		return []string{"1024x1024", "1536x1024", "1024x1536"}, false, nil
	case PlatformGemini, PlatformAntigravity:
		// Gemini image_size enum style.
		return []string{"1K", "2K", "4K"}, false, nil
	default:
		return creazyCanvasImageSizesForPlatform(platform), true, nil
	}
}

// ValidateCreazyCanvasImageSize checks size against a catalog model policy.
func ValidateCreazyCanvasImageSize(model *CreazyCanvasImageModel, raw string) bool {
	if model == nil {
		return false
	}
	size := strings.TrimSpace(raw)
	if size == "" {
		return false
	}
	for _, s := range model.Sizes {
		if strings.EqualFold(strings.TrimSpace(s), size) {
			return true
		}
	}
	if !model.AllowCustomSize {
		return false
	}
	c := model.SizeConstraints
	if c != nil {
		for _, alias := range c.Aliases {
			if strings.EqualFold(strings.TrimSpace(alias), size) {
				return true
			}
		}
	} else {
		// Loose legacy free-form: aliases + simple WxH bounds.
		switch strings.ToLower(size) {
		case "auto", "1k", "2k", "4k":
			return true
		}
	}
	w, h, ok := parseImageBillingDimensions(size)
	if !ok {
		w, h, ok = parseCreazyCanvasImageDimensions(size)
	}
	if !ok {
		return false
	}
	if c == nil {
		return w >= 64 && h >= 64 && w <= 8192 && h <= 8192
	}
	if c.MultipleOf > 0 {
		if w%c.MultipleOf != 0 || h%c.MultipleOf != 0 {
			return false
		}
	}
	if c.MaxEdge > 0 && (w > c.MaxEdge || h > c.MaxEdge) {
		return false
	}
	pixels := int64(w) * int64(h)
	if c.MinPixels > 0 && pixels < c.MinPixels {
		return false
	}
	if c.MaxPixels > 0 && pixels > c.MaxPixels {
		return false
	}
	if c.MaxAspectRatio > 0 {
		long, short := w, h
		if h > w {
			long, short = h, w
		}
		if short <= 0 {
			return false
		}
		if float64(long)/float64(short) > c.MaxAspectRatio+1e-9 {
			return false
		}
	}
	return true
}

func parseCreazyCanvasImageDimensions(size string) (int, int, bool) {
	s := strings.TrimSpace(strings.ToLower(size))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "×", "x")
	s = strings.ReplaceAll(s, "*", "x")
	parts := strings.Split(s, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// creazyCanvasImageReferenceCapability reports whether a public image model accepts
// reference images via /v1/images/edits (or generations for platforms that accept both).
func creazyCanvasImageReferenceCapability(platform, modelID string) (supports bool, maxRefs int, require bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch platform {
	case PlatformOpenAI, PlatformGrok, PlatformGemini, PlatformAntigravity:
		supports = true
	default:
		// Unknown image platforms still allowed if catalog listed them.
		supports = id != ""
	}
	if !supports {
		return false, 0, false
	}
	require = strings.Contains(id, "edit") || strings.HasSuffix(id, "-edit")
	switch platform {
	case PlatformGemini, PlatformAntigravity:
		maxRefs = 4
	case PlatformOpenAI:
		maxRefs = 4
	case PlatformGrok:
		maxRefs = 1
	default:
		maxRefs = 1
	}
	if require && maxRefs < 1 {
		maxRefs = 1
	}
	return supports, maxRefs, require
}

// groupHasCreazyGenerationCapability reports whether the group can power canvas
// image and/or video generation (not merely allow_creazy_canvas).
func groupHasCreazyGenerationCapability(group *Group) bool {
	if group == nil {
		return false
	}
	if group.AllowImageGeneration && len(defaultCreazyCanvasImageModels(group.Platform)) > 0 {
		return true
	}
	if len(buildCreazyCanvasVideoModels(group)) > 0 {
		return true
	}
	return false
}

func creazyCanvasImageSizesForPlatform(platform string) []string {
	switch platform {
	case PlatformOpenAI, PlatformGrok:
		return []string{"1024x1024", "1536x1024", "1024x1536"}
	case PlatformGemini, PlatformAntigravity:
		return []string{"1K", "2K", "4K"}
	default:
		return []string{"1024x1024", "1K", "2K", "4K"}
	}
}

func creazyCanvasImageSizeTier(size string) string {
	if tier, ok := ClassifyImageBillingTier(size); ok {
		return tier
	}
	s := strings.TrimSpace(strings.ToUpper(size))
	switch s {
	case "1K", "2K", "4K":
		return s
	}
	return ImageBillingSize1K
}

func defaultCreazyCanvasImageModels(platform string) []string {
	switch platform {
	case PlatformGrok:
		return []string{"grok-imagine", "grok-imagine-edit"}
	case PlatformOpenAI:
		return []string{"gpt-image-2", "gpt-image-1"}
	case PlatformGemini:
		return []string{"gemini-2.5-flash-image", "gemini-3-pro-image-preview"}
	case PlatformAntigravity:
		return []string{"gemini-3.1-flash-image", "gemini-3-pro-image", "gemini-2.5-flash-image"}
	default:
		return []string{}
	}
}

func isCreazyCanvasWorkStatus(status string) bool {
	switch status {
	case CreazyCanvasWorkStatusCreated,
		CreazyCanvasWorkStatusQueued,
		CreazyCanvasWorkStatusRunning,
		CreazyCanvasWorkStatusSucceeded,
		CreazyCanvasWorkStatusFailed,
		CreazyCanvasWorkStatusCanceled,
		CreazyCanvasWorkStatusExpired:
		return true
	default:
		return false
	}
}

func creazyCanvasGatewayContentHint(work *CreazyCanvasWork) string {
	if work == nil {
		return ""
	}
	remoteID := strings.TrimSpace(work.GatewayRemoteID)
	gt := normalizeCreazyCanvasGatewayType(work.GatewayType, work.Kind)
	switch gt {
	case CreazyCanvasGatewayVideoJob:
		if remoteID != "" {
			return fmt.Sprintf("GET /v1/videos/jobs/%s/content", remoteID)
		}
		return "GET /v1/videos/jobs/{job_id}/content"
	case CreazyCanvasGatewayImageTask:
		if remoteID != "" {
			return fmt.Sprintf("GET /v1/images/tasks/%s", remoteID)
		}
		return "GET /v1/images/tasks/{task_id}"
	case CreazyCanvasGatewayImageSync:
		return "同步生图结果请使用创建时返回的 URL；或重新生成"
	default:
		if remoteID != "" {
			return fmt.Sprintf("gateway_type=%s gateway_remote_id=%s", gt, remoteID)
		}
		return "作品尚未写入 object_key，请先通过网关获取内容"
	}
}

func normalizeCreazyCanvasGatewayType(raw, kind string) string {
	gt := strings.ToLower(strings.TrimSpace(raw))
	switch gt {
	case CreazyCanvasGatewayImageTask, CreazyCanvasGatewayImageSync, CreazyCanvasGatewayVideoJob:
		return gt
	case "images", "image", "openai_images", "image_async", "async_image":
		return CreazyCanvasGatewayImageTask
	case "sync_image":
		return CreazyCanvasGatewayImageSync
	case "seedance", "fflink", "ltx", "happyhorse", "happy-horse", "minimax", "grokimagine", "grok-imagine", "video", "videos":
		return CreazyCanvasGatewayVideoJob
	}
	if kind == CreazyCanvasWorkKindVideo {
		return CreazyCanvasGatewayVideoJob
	}
	if kind == CreazyCanvasWorkKindImage {
		return CreazyCanvasGatewayImageTask
	}
	return gt
}

func applyCreazyCanvasWorkView(work *CreazyCanvasWork) {
	if work == nil {
		return
	}
	work.GatewayType = normalizeCreazyCanvasGatewayType(work.GatewayType, work.Kind)
	if work.Status == CreazyCanvasWorkStatusExpired {
		return
	}
	if !work.ExpiresAt.IsZero() && time.Now().After(work.ExpiresAt) {
		work.Status = CreazyCanvasWorkStatusExpired
	}
}

func creazyCanvasDurationsForProfile(profile ffLinkVideoModelProfile) []int {
	candidates := []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 18, 20, 30}
	out := make([]int, 0, 8)
	res := profile.DefaultResolution
	for _, d := range candidates {
		if profile.ValidateDuration == nil || profile.ValidateDuration(d, res) {
			out = append(out, d)
		}
	}
	if len(out) == 0 && profile.DefaultDuration > 0 {
		return []int{profile.DefaultDuration}
	}
	return out
}

func buildCreazyCanvasObjectKey(work *CreazyCanvasWork, filename string) string {
	safeName := strings.TrimSpace(filename)
	if safeName == "" {
		safeName = "content.bin"
	}
	safeName = path.Base(safeName)
	safeName = strings.ReplaceAll(safeName, " ", "_")
	userID := int64(0)
	workID := int64(0)
	kind := "misc"
	if work != nil {
		userID = work.UserID
		workID = work.ID
		if work.Kind != "" {
			kind = work.Kind
		}
	}
	return fmt.Sprintf("%s/%d/%s/%d/%s-%s", creazyCanvasObjectPrefix, userID, kind, workID, uuid.NewString(), safeName)
}

func sortedStringKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	// 稳定顺序：常见分辨率优先
	priority := map[string]int{
		VideoBillingResolution480P: 1,
		VideoBillingResolution720P: 2,
		VideoBillingResolution1080P: 3,
		VideoBillingResolution1440P: 4,
		VideoBillingResolution2160P: 5,
		"16:9": 1, "9:16": 2, "1:1": 3, "4:3": 4, "3:4": 5, "21:9": 6, "9:21": 7, "3:2": 8, "2:3": 9,
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			pi, okI := priority[out[i]]
			pj, okJ := priority[out[j]]
			if !okI {
				pi = 100
			}
			if !okJ {
				pj = 100
			}
			if pi > pj || (pi == pj && out[i] > out[j]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
