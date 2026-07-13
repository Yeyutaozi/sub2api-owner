package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AgentAppHandler struct {
	appService *service.AgentAppService
}

func NewAgentAppHandler(appService *service.AgentAppService) *AgentAppHandler {
	return &AgentAppHandler{appService: appService}
}

type createAgentAppRequest struct {
	Name        string `json:"name" binding:"required,max=120"`
	Slug        string `json:"slug" binding:"required,max=140"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
	Category    string `json:"category"`
	AppType     string `json:"app_type"`
	Visibility  string `json:"visibility"`
	Status      string `json:"status"`
}

type createAgentAppVersionRequest struct {
	Version                string         `json:"version" binding:"required,max=64"`
	Status                 string         `json:"status"`
	RuntimeType            string         `json:"runtime_type"`
	WorkerHostID           *int64         `json:"worker_host_id"`
	WorkerRoute            string         `json:"worker_route"`
	WorkerHealthRoute      string         `json:"worker_health_route"`
	ImageRef               string         `json:"image_ref"`
	SourceRef              string         `json:"source_ref"`
	InputSchemaJSON        map[string]any `json:"input_schema_json"`
	OutputSchemaJSON       map[string]any `json:"output_schema_json"`
	CapabilitiesJSON       map[string]any `json:"capabilities_json"`
	DefaultModelConfigJSON map[string]any `json:"default_model_config_json"`
	NodeModelPolicyJSON    map[string]any `json:"node_model_policy_json"`
	ArtifactPolicyJSON     map[string]any `json:"artifact_policy_json"`
	Changelog              string         `json:"changelog"`
}

type createAgentAppWithVersionRequest struct {
	App     createAgentAppRequest        `json:"app" binding:"required"`
	Version createAgentAppVersionRequest `json:"version" binding:"required"`
}

type updateAgentAppVersionStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type uploadAgentAppIconResponse struct {
	URL             string `json:"url"`
	PreviewURL      string `json:"preview_url,omitempty"`
	ObjectKey       string `json:"object_key,omitempty"`
	StorageProvider string `json:"storage_provider,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
}

type agentAppResponse struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Description        string `json:"description,omitempty"`
	IconURL            string `json:"icon_url,omitempty"`
	Category           string `json:"category,omitempty"`
	AppType            string `json:"app_type"`
	Visibility         string `json:"visibility"`
	Status             string `json:"status"`
	PublishedVersionID *int64 `json:"published_version_id,omitempty"`
	CreatedBy          *int64 `json:"created_by,omitempty"`
	UpdatedBy          *int64 `json:"updated_by,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type agentAppVersionResponse struct {
	ID                     int64                    `json:"id"`
	AppID                  int64                    `json:"app_id"`
	Version                string                   `json:"version"`
	Status                 string                   `json:"status"`
	RuntimeType            string                   `json:"runtime_type"`
	WorkerHostID           *int64                   `json:"worker_host_id,omitempty"`
	WorkerRoute            string                   `json:"worker_route,omitempty"`
	WorkerHealthRoute      string                   `json:"worker_health_route,omitempty"`
	ImageRef               string                   `json:"image_ref,omitempty"`
	SourceRef              string                   `json:"source_ref,omitempty"`
	InputSchemaJSON        map[string]any           `json:"input_schema_json"`
	OutputSchemaJSON       map[string]any           `json:"output_schema_json"`
	CapabilitiesJSON       map[string]any           `json:"capabilities_json"`
	DefaultModelConfigJSON map[string]any           `json:"default_model_config_json"`
	NodeModelPolicyJSON    map[string]any           `json:"node_model_policy_json"`
	ArtifactPolicyJSON     map[string]any           `json:"artifact_policy_json"`
	Changelog              string                   `json:"changelog,omitempty"`
	CreatedBy              *int64                   `json:"created_by,omitempty"`
	PublishedAt            *string                  `json:"published_at,omitempty"`
	CreatedAt              string                   `json:"created_at"`
	UpdatedAt              string                   `json:"updated_at"`
	WorkerHost             *agentWorkerHostResponse `json:"worker_host,omitempty"`
}

func (h *AgentAppHandler) ListApps(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.appService.ListApps(c.Request.Context(), pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "id"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentAppListFilters{
		Status:  c.Query("status"),
		AppType: c.Query("app_type"),
		Search:  c.Query("search"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentAppResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentAppToResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentAppHandler) CreateApp(c *gin.Context) {
	var req createAgentAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	createdBy := currentAdminID(c)
	app, err := h.appService.CreateApp(c.Request.Context(), service.CreateAgentAppInput{
		Name:        strings.TrimSpace(req.Name),
		Slug:        strings.TrimSpace(req.Slug),
		Description: strings.TrimSpace(req.Description),
		IconURL:     strings.TrimSpace(req.IconURL),
		Category:    strings.TrimSpace(req.Category),
		AppType:     strings.TrimSpace(req.AppType),
		Visibility:  strings.TrimSpace(req.Visibility),
		Status:      strings.TrimSpace(req.Status),
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, agentAppToResponse(app))
}

func (h *AgentAppHandler) UpdateApp(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	var req createAgentAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updatedBy := currentAdminID(c)
	app, err := h.appService.UpdateApp(c.Request.Context(), appID, service.CreateAgentAppInput{
		Name:        strings.TrimSpace(req.Name),
		Slug:        strings.TrimSpace(req.Slug),
		Description: strings.TrimSpace(req.Description),
		IconURL:     strings.TrimSpace(req.IconURL),
		Category:    strings.TrimSpace(req.Category),
		AppType:     strings.TrimSpace(req.AppType),
		Visibility:  strings.TrimSpace(req.Visibility),
		Status:      strings.TrimSpace(req.Status),
		UpdatedBy:   updatedBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentAppToResponse(app))
}

func (h *AgentAppHandler) DeleteApp(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	if err := h.appService.DeleteApp(c.Request.Context(), appID, currentAdminID(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *AgentAppHandler) CreateAppWithVersion(c *gin.Context) {
	var req createAgentAppWithVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	createdBy := currentAdminID(c)
	app, version, err := h.appService.CreateAppWithVersion(c.Request.Context(),
		service.CreateAgentAppInput{
			Name:        strings.TrimSpace(req.App.Name),
			Slug:        strings.TrimSpace(req.App.Slug),
			Description: strings.TrimSpace(req.App.Description),
			IconURL:     strings.TrimSpace(req.App.IconURL),
			Category:    strings.TrimSpace(req.App.Category),
			AppType:     strings.TrimSpace(req.App.AppType),
			Visibility:  strings.TrimSpace(req.App.Visibility),
			Status:      strings.TrimSpace(req.App.Status),
			CreatedBy:   createdBy,
			UpdatedBy:   createdBy,
		},
		agentAppVersionInputFromRequest(0, req.Version, createdBy),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, gin.H{
		"app":     agentAppToResponse(app),
		"version": agentAppVersionToResponse(version),
	})
}

func (h *AgentAppHandler) UploadIcon(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invalid icon file: "+err.Error())
		return
	}
	src, err := file.Open()
	if err != nil {
		response.BadRequest(c, "Invalid icon file: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	result, err := h.appService.UploadIcon(c.Request.Context(), service.UploadAgentAppIconInput{
		FileName:    file.Filename,
		ContentType: strings.TrimSpace(file.Header.Get("Content-Type")),
		SizeBytes:   file.Size,
		Body:        src,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, uploadAgentAppIconResponse{
		URL:             result.URL,
		PreviewURL:      result.PreviewURL,
		ObjectKey:       result.ObjectKey,
		StorageProvider: result.StorageProvider,
		Bucket:          result.Bucket,
		SizeBytes:       result.SizeBytes,
		SHA256:          result.SHA256,
	})
}

func (h *AgentAppHandler) GetApp(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	app, err := h.appService.GetAppByID(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentAppToResponse(app))
}

func (h *AgentAppHandler) ListVersions(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	items, err := h.appService.ListVersions(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentAppVersionResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentAppVersionToResponse(&items[i]))
	}
	response.Success(c, out)
}

func (h *AgentAppHandler) CreateVersion(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	var req createAgentAppVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	version, err := h.appService.CreateVersion(c.Request.Context(), agentAppVersionInputFromRequest(appID, req, currentAdminID(c)))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, agentAppVersionToResponse(version))
}

func (h *AgentAppHandler) PublishVersion(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	versionID, ok := parseAgentIDParam(c, "version_id", "Invalid version ID")
	if !ok {
		return
	}
	version, err := h.appService.PublishVersion(c.Request.Context(), appID, versionID, currentAdminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentAppVersionToResponse(version))
}

func (h *AgentAppHandler) UpdateVersionStatus(c *gin.Context) {
	appID, ok := parseAgentIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	versionID, ok := parseAgentIDParam(c, "version_id", "Invalid version ID")
	if !ok {
		return
	}
	var req updateAgentAppVersionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	version, err := h.appService.SetVersionStatus(c.Request.Context(), appID, versionID, strings.TrimSpace(req.Status), currentAdminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentAppVersionToResponse(version))
}

func agentAppVersionInputFromRequest(appID int64, req createAgentAppVersionRequest, createdBy *int64) service.CreateAgentAppVersionInput {
	return service.CreateAgentAppVersionInput{
		AppID:                  appID,
		Version:                strings.TrimSpace(req.Version),
		Status:                 strings.TrimSpace(req.Status),
		RuntimeType:            strings.TrimSpace(req.RuntimeType),
		WorkerHostID:           req.WorkerHostID,
		WorkerRoute:            strings.TrimSpace(req.WorkerRoute),
		WorkerHealthRoute:      strings.TrimSpace(req.WorkerHealthRoute),
		ImageRef:               strings.TrimSpace(req.ImageRef),
		SourceRef:              strings.TrimSpace(req.SourceRef),
		InputSchemaJSON:        req.InputSchemaJSON,
		OutputSchemaJSON:       req.OutputSchemaJSON,
		CapabilitiesJSON:       req.CapabilitiesJSON,
		DefaultModelConfigJSON: req.DefaultModelConfigJSON,
		NodeModelPolicyJSON:    req.NodeModelPolicyJSON,
		ArtifactPolicyJSON:     req.ArtifactPolicyJSON,
		Changelog:              strings.TrimSpace(req.Changelog),
		CreatedBy:              createdBy,
	}
}

func currentAdminID(c *gin.Context) *int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return nil
	}
	return &subject.UserID
}

func agentAppToResponse(app *service.AgentApp) *agentAppResponse {
	if app == nil {
		return nil
	}
	return &agentAppResponse{
		ID:                 app.ID,
		Name:               app.Name,
		Slug:               app.Slug,
		Description:        app.Description,
		IconURL:            app.IconURL,
		Category:           app.Category,
		AppType:            app.AppType,
		Visibility:         app.Visibility,
		Status:             app.Status,
		PublishedVersionID: app.PublishedVersionID,
		CreatedBy:          app.CreatedBy,
		UpdatedBy:          app.UpdatedBy,
		CreatedAt:          app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          app.UpdatedAt.Format(time.RFC3339),
	}
}

func agentAppVersionToResponse(version *service.AgentAppVersion) *agentAppVersionResponse {
	if version == nil {
		return nil
	}
	return &agentAppVersionResponse{
		ID:                     version.ID,
		AppID:                  version.AppID,
		Version:                version.Version,
		Status:                 version.Status,
		RuntimeType:            version.RuntimeType,
		WorkerHostID:           version.WorkerHostID,
		WorkerRoute:            version.WorkerRoute,
		WorkerHealthRoute:      version.WorkerHealthRoute,
		ImageRef:               version.ImageRef,
		SourceRef:              version.SourceRef,
		InputSchemaJSON:        version.InputSchemaJSON,
		OutputSchemaJSON:       version.OutputSchemaJSON,
		CapabilitiesJSON:       version.CapabilitiesJSON,
		DefaultModelConfigJSON: version.DefaultModelConfigJSON,
		NodeModelPolicyJSON:    version.NodeModelPolicyJSON,
		ArtifactPolicyJSON:     version.ArtifactPolicyJSON,
		Changelog:              version.Changelog,
		CreatedBy:              version.CreatedBy,
		PublishedAt:            formatTimePtr(version.PublishedAt),
		CreatedAt:              version.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              version.UpdatedAt.Format(time.RFC3339),
		WorkerHost:             agentWorkerHostToResponse(version.WorkerHost),
	}
}
