package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var agentAppSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,138}[a-z0-9]$`)

type AgentAppService struct {
	repo              AgentAppRepository
	workerHostRepo    AgentWorkerHostRepository
	workerHostService *AgentWorkerHostService
	artifactStore     AgentArtifactStore
}

func NewAgentAppService(repo AgentAppRepository, workerHostRepo AgentWorkerHostRepository, workerHostService *AgentWorkerHostService, artifactStore AgentArtifactStore) *AgentAppService {
	if artifactStore == nil {
		artifactStore = disabledAgentArtifactStore{}
	}
	return &AgentAppService{
		repo:              repo,
		workerHostRepo:    workerHostRepo,
		workerHostService: workerHostService,
		artifactStore:     artifactStore,
	}
}

type CreateAgentAppInput struct {
	Name        string
	Slug        string
	Description string
	IconURL     string
	Category    string
	AppType     string
	Visibility  string
	Status      string
	CreatedBy   *int64
	UpdatedBy   *int64
}

type CreateAgentAppVersionInput struct {
	AppID                  int64
	Version                string
	Status                 string
	RuntimeType            string
	WorkerHostID           *int64
	WorkerRoute            string
	WorkerHealthRoute      string
	ImageRef               string
	SourceRef              string
	InputSchemaJSON        map[string]any
	OutputSchemaJSON       map[string]any
	CapabilitiesJSON       map[string]any
	DefaultModelConfigJSON map[string]any
	NodeModelPolicyJSON    map[string]any
	ArtifactPolicyJSON     map[string]any
	Changelog              string
	CreatedBy              *int64
}

type UploadAgentAppIconInput struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	Body        io.Reader
}

type UploadAgentAppIconResult struct {
	URL             string `json:"url"`
	PreviewURL      string `json:"preview_url,omitempty"`
	ObjectKey       string `json:"object_key,omitempty"`
	StorageProvider string `json:"storage_provider,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
}

func (s *AgentAppService) CreateApp(ctx context.Context, input CreateAgentAppInput) (*AgentApp, error) {
	app, err := buildAgentApp(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateApp(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *AgentAppService) UpdateApp(ctx context.Context, id int64, input CreateAgentAppInput) (*AgentApp, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("AGENT_APP_ID_INVALID", "应用 ID 无效")
	}
	current, err := s.repo.GetAppByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next, err := buildAgentApp(input)
	if err != nil {
		return nil, err
	}
	next.ID = current.ID
	next.CreatedBy = current.CreatedBy
	next.CreatedAt = current.CreatedAt
	next.PublishedVersionID = current.PublishedVersionID
	if err := s.repo.UpdateApp(ctx, next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *AgentAppService) DeleteApp(ctx context.Context, id int64, updatedBy *int64) error {
	if id <= 0 {
		return infraerrors.BadRequest("AGENT_APP_ID_INVALID", "应用 ID 无效")
	}
	return s.repo.DeleteApp(ctx, id, updatedBy)
}

func (s *AgentAppService) CreateAppWithVersion(ctx context.Context, appInput CreateAgentAppInput, versionInput CreateAgentAppVersionInput) (*AgentApp, *AgentAppVersion, error) {
	app, err := buildAgentApp(appInput)
	if err != nil {
		return nil, nil, err
	}
	versionInput.AppID = 0
	version, err := s.buildAgentAppVersion(ctx, versionInput, false)
	if err != nil {
		return nil, nil, err
	}
	if version.CreatedBy == nil {
		version.CreatedBy = app.CreatedBy
	}
	if err := s.repo.CreateAppWithVersion(ctx, app, version); err != nil {
		return nil, nil, err
	}
	return app, version, nil
}

func (s *AgentAppService) ListApps(ctx context.Context, params pagination.PaginationParams, filters AgentAppListFilters) ([]AgentApp, *pagination.PaginationResult, error) {
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}
	return s.repo.ListApps(ctx, params, filters)
}

func (s *AgentAppService) GetAppByID(ctx context.Context, id int64) (*AgentApp, error) {
	return s.repo.GetAppByID(ctx, id)
}

func (s *AgentAppService) CreateVersion(ctx context.Context, input CreateAgentAppVersionInput) (*AgentAppVersion, error) {
	if _, err := s.repo.GetAppByID(ctx, input.AppID); err != nil {
		return nil, err
	}
	version, err := s.buildAgentAppVersion(ctx, input, true)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateVersion(ctx, version); err != nil {
		return nil, err
	}
	if version.Status == AgentAppStatusPublished {
		if err := s.repo.PublishVersion(ctx, version.AppID, version.ID, version.CreatedBy); err != nil {
			return nil, err
		}
		publishedAt := timeNowUTC()
		version.PublishedAt = &publishedAt
	}
	return version, nil
}

func (s *AgentAppService) PublishVersion(ctx context.Context, appID, versionID int64, updatedBy *int64) (*AgentAppVersion, error) {
	if appID <= 0 || versionID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_APP_VERSION_ID_INVALID", "应用版本 ID 无效")
	}
	version, err := s.repo.GetVersionByID(ctx, versionID)
	if err != nil || version.AppID != appID {
		return nil, ErrAgentAppVersionNotFound
	}
	if version.RuntimeType != AgentRuntimeTypeWorker || version.WorkerHostID == nil {
		return nil, infraerrors.BadRequest("AGENT_APP_RUNTIME_UNSUPPORTED", "当前应用中心仅支持 Worker 运行方式")
	}
	if err := validateAgentAppModelPolicies(version.NodeModelPolicyJSON); err != nil {
		return nil, err
	}
	if s.workerHostService != nil {
		if _, err := s.workerHostService.ValidateRunRoute(ctx, *version.WorkerHostID, version.WorkerHealthRoute, version.WorkerRoute); err != nil {
			return nil, err
		}
	}
	if err := s.repo.PublishVersion(ctx, appID, versionID, updatedBy); err != nil {
		return nil, err
	}
	return s.repo.GetVersionByID(ctx, versionID)
}

func (s *AgentAppService) SetVersionStatus(ctx context.Context, appID, versionID int64, status string, updatedBy *int64) (*AgentAppVersion, error) {
	status = strings.TrimSpace(status)
	switch status {
	case AgentAppStatusDraft, AgentAppStatusDisabled, AgentAppStatusArchived:
	case AgentAppStatusPublished:
		return s.PublishVersion(ctx, appID, versionID, updatedBy)
	default:
		return nil, infraerrors.BadRequest("AGENT_APP_VERSION_STATUS_INVALID", "应用版本状态无效")
	}
	if err := s.repo.SetVersionStatus(ctx, appID, versionID, status, updatedBy); err != nil {
		return nil, err
	}
	return s.repo.GetVersionByID(ctx, versionID)
}

func (s *AgentAppService) ListVersions(ctx context.Context, appID int64) ([]AgentAppVersion, error) {
	return s.repo.ListVersions(ctx, appID)
}

func (s *AgentAppService) UploadIcon(ctx context.Context, input UploadAgentAppIconInput) (*UploadAgentAppIconResult, error) {
	if s == nil || s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	if input.Body == nil {
		return nil, infraerrors.BadRequest("AGENT_APP_ICON_FILE_REQUIRED", "请选择应用图标")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if !isSupportedAgentAppIconContentType(contentType) {
		return nil, infraerrors.BadRequest("AGENT_APP_ICON_TYPE_INVALID", "应用图标只支持 PNG、JPEG 或 WebP")
	}
	const maxIconBytes = 2 * 1024 * 1024
	if input.SizeBytes > maxIconBytes {
		return nil, infraerrors.BadRequest("AGENT_APP_ICON_TOO_LARGE", "应用图标不能超过 2MB")
	}

	hasher := sha256.New()
	limited := io.LimitReader(input.Body, maxIconBytes+1)
	data, err := io.ReadAll(io.TeeReader(limited, hasher))
	if err != nil {
		return nil, fmt.Errorf("read app icon: %w", err)
	}
	if int64(len(data)) > maxIconBytes {
		return nil, infraerrors.BadRequest("AGENT_APP_ICON_TOO_LARGE", "应用图标不能超过 2MB")
	}

	objectKey := "app-icons/" + uuid.NewString() + iconExtension(input.FileName, contentType)
	putResult, err := s.artifactStore.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        bytes.NewReader(data),
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		Metadata: map[string]string{
			"asset-kind": "agent-app-icon",
			"sha256":     hex.EncodeToString(hasher.Sum(nil)),
		},
	})
	if err != nil {
		return nil, err
	}
	previewURL, _ := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{
		StorageProvider: putResult.Provider,
		Bucket:          putResult.Bucket,
		ObjectKey:       putResult.ObjectKey,
	}, time.Hour)
	return &UploadAgentAppIconResult{
		URL:             putResult.ObjectURL,
		PreviewURL:      previewURL,
		ObjectKey:       putResult.ObjectKey,
		StorageProvider: putResult.Provider,
		Bucket:          putResult.Bucket,
		SizeBytes:       putResult.SizeBytes,
		SHA256:          hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func buildAgentApp(input CreateAgentAppInput) (*AgentApp, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("AGENT_APP_NAME_REQUIRED", "应用名称不能为空")
	}
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	if !agentAppSlugPattern.MatchString(slug) {
		return nil, infraerrors.BadRequest("AGENT_APP_SLUG_INVALID", "应用标识只能包含小写字母、数字、下划线和中划线，长度 3-140")
	}
	appType := strings.TrimSpace(input.AppType)
	if appType == "" {
		appType = AgentAppTypeExternal
	}
	switch appType {
	case AgentAppTypePrompt, AgentAppTypeWorkflow, AgentAppTypeAgent, AgentAppTypeExternal:
	default:
		return nil, infraerrors.BadRequest("AGENT_APP_TYPE_INVALID", "应用类型无效")
	}
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = AgentAppVisibilityPrivate
	}
	switch visibility {
	case AgentAppVisibilityPublic, AgentAppVisibilityPrivate:
	default:
		return nil, infraerrors.BadRequest("AGENT_APP_VISIBILITY_INVALID", "应用可见性无效")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AgentAppStatusDraft
	}
	switch status {
	case AgentAppStatusDraft, AgentAppStatusPublished, AgentAppStatusDisabled, AgentAppStatusArchived:
	default:
		return nil, infraerrors.BadRequest("AGENT_APP_STATUS_INVALID", "应用状态无效")
	}

	return &AgentApp{
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(input.Description),
		IconURL:     strings.TrimSpace(input.IconURL),
		Category:    strings.TrimSpace(input.Category),
		AppType:     appType,
		Visibility:  visibility,
		Status:      status,
		CreatedBy:   input.CreatedBy,
		UpdatedBy:   input.UpdatedBy,
	}, nil
}

func (s *AgentAppService) buildAgentAppVersion(ctx context.Context, input CreateAgentAppVersionInput, requireAppID bool) (*AgentAppVersion, error) {
	if requireAppID && input.AppID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_APP_ID_INVALID", "应用 ID 无效")
	}
	versionName := strings.TrimSpace(input.Version)
	if versionName == "" {
		return nil, infraerrors.BadRequest("AGENT_APP_VERSION_REQUIRED", "版本号不能为空")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AgentAppStatusDraft
	}
	switch status {
	case AgentAppStatusDraft, AgentAppStatusPublished, AgentAppStatusDisabled, AgentAppStatusArchived:
	default:
		return nil, infraerrors.BadRequest("AGENT_APP_VERSION_STATUS_INVALID", "应用版本状态无效")
	}
	runtimeType := strings.TrimSpace(input.RuntimeType)
	if runtimeType == "" {
		runtimeType = AgentRuntimeTypeWorker
	}
	if runtimeType != AgentRuntimeTypeWorker {
		return nil, infraerrors.BadRequest("AGENT_APP_RUNTIME_UNSUPPORTED", "当前应用中心仅支持 Worker 运行方式")
	}

	workerRoute := strings.TrimSpace(input.WorkerRoute)
	workerHealthRoute := strings.TrimSpace(input.WorkerHealthRoute)
	if runtimeType == AgentRuntimeTypeWorker {
		if input.WorkerHostID == nil || *input.WorkerHostID <= 0 {
			return nil, infraerrors.BadRequest("AGENT_APP_WORKER_HOST_REQUIRED", "Worker 运行类型必须选择 Worker Host")
		}
		host, err := s.workerHostRepo.GetByID(ctx, *input.WorkerHostID)
		if err != nil {
			return nil, err
		}
		if host.Status == AgentWorkerHostStatusDisabled {
			return nil, infraerrors.BadRequest("AGENT_APP_WORKER_HOST_DISABLED", "所选 Worker Host 已禁用")
		}
		workerRoute, err = normalizeWorkerPath(workerRoute, "", "Worker 运行路径")
		if err != nil {
			return nil, err
		}
		if workerRoute == "" {
			return nil, infraerrors.BadRequest("AGENT_APP_WORKER_ROUTE_REQUIRED", "Worker 运行路径不能为空")
		}
		if workerHealthRoute != "" {
			workerHealthRoute, err = normalizeWorkerPath(workerHealthRoute, "", "Worker 健康检查路径")
			if err != nil {
				return nil, err
			}
		}
		if status == AgentAppStatusPublished && s.workerHostService != nil {
			if _, err := s.workerHostService.ValidateRunRoute(ctx, host.ID, workerHealthRoute, workerRoute); err != nil {
				return nil, err
			}
		}
	}
	if err := validateAgentAppModelPolicies(input.NodeModelPolicyJSON); err != nil {
		return nil, err
	}

	return &AgentAppVersion{
		AppID:                  input.AppID,
		Version:                versionName,
		Status:                 status,
		RuntimeType:            runtimeType,
		WorkerHostID:           input.WorkerHostID,
		WorkerRoute:            workerRoute,
		WorkerHealthRoute:      workerHealthRoute,
		ImageRef:               strings.TrimSpace(input.ImageRef),
		SourceRef:              strings.TrimSpace(input.SourceRef),
		InputSchemaJSON:        ensureMap(input.InputSchemaJSON),
		OutputSchemaJSON:       ensureMap(input.OutputSchemaJSON),
		CapabilitiesJSON:       ensureMap(input.CapabilitiesJSON),
		DefaultModelConfigJSON: ensureMap(input.DefaultModelConfigJSON),
		NodeModelPolicyJSON:    ensureMap(input.NodeModelPolicyJSON),
		ArtifactPolicyJSON:     ensureMap(input.ArtifactPolicyJSON),
		Changelog:              strings.TrimSpace(input.Changelog),
		CreatedBy:              input.CreatedBy,
	}, nil
}

func validateAgentAppModelPolicies(raw map[string]any) error {
	if len(raw) == 0 {
		return infraerrors.BadRequest("AGENT_APP_MODEL_POLICY_REQUIRED", "至少需要配置一个模型能力")
	}
	requiredCount := 0
	for policyKey, value := range raw {
		policy, ok := value.(map[string]any)
		if !ok {
			return infraerrors.BadRequest("AGENT_APP_MODEL_POLICY_INVALID", fmt.Sprintf("模型能力 %s 配置无效", policyKey))
		}
		model := strings.TrimSpace(agentAppStringFromAny(policy["model"]))
		provider := normalizeAgentModelProvider(agentAppStringFromAny(policy["provider"]))
		if provider == "" {
			provider = normalizeAgentModelProvider(agentAppStringFromAny(policy["platform"]))
		}
		if model == "" {
			return infraerrors.BadRequest("AGENT_APP_MODEL_REQUIRED", fmt.Sprintf("模型能力 %s 必须填写模型", policyKey))
		}
		if provider == "" {
			return infraerrors.BadRequest("AGENT_APP_MODEL_PROVIDER_REQUIRED", fmt.Sprintf("模型能力 %s 必须选择模型厂商", policyKey))
		}
		if optional, _ := policy["optional"].(bool); !optional {
			requiredCount++
		}
	}
	if requiredCount == 0 {
		return infraerrors.BadRequest("AGENT_APP_REQUIRED_MODEL_POLICY_MISSING", "至少需要一个运行前必须选择 Key 的模型能力")
	}
	return nil
}

func agentAppStringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func ensureMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func isSupportedAgentAppIconContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func iconExtension(fileName, contentType string) string {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(fileName)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
