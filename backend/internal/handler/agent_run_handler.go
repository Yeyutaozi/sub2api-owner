package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AgentRunHandler struct {
	runService     *service.AgentRunService
	settingService *service.SettingService
}

func NewAgentRunHandler(runService *service.AgentRunService, settingService *service.SettingService) *AgentRunHandler {
	return &AgentRunHandler{runService: runService, settingService: settingService}
}

type createAgentRunRequest struct {
	AppVersionID   int64                             `json:"app_version_id"`
	APIKeyID       int64                             `json:"api_key_id" binding:"required"`
	APIKeyBindings []createAgentRunKeyBindingRequest `json:"api_key_bindings"`
	InputAssetIDs  []int64                           `json:"input_asset_ids"`
	Input          map[string]any                    `json:"input"`
	InputRefURL    string                            `json:"input_ref_url"`
}

type createAgentRunKeyBindingRequest struct {
	PolicyKey string `json:"policy_key"`
	NodeID    string `json:"node_id"`
	Role      string `json:"role"`
	APIKeyID  int64  `json:"api_key_id" binding:"required"`
}

type modelProxyHandlerRequest struct {
	RunID       int64                                  `json:"run_id"`
	NodeID      string                                 `json:"node_id"`
	Role        string                                 `json:"role"`
	Model       string                                 `json:"model" binding:"required"`
	GroupID     *int64                                 `json:"group_id"`
	Platform    string                                 `json:"platform"`
	Endpoint    string                                 `json:"endpoint" binding:"required"`
	Method      string                                 `json:"method"`
	ContentType string                                 `json:"content_type"`
	BodyBase64  string                                 `json:"body_base64"`
	Multipart   []service.ModelProxyMultipartReference `json:"multipart"`
	Request     map[string]any                         `json:"request"`
	Metadata    map[string]any                         `json:"metadata"`
}

type artifactRegisterHandlerRequest struct {
	RunID           int64          `json:"run_id"`
	RunToken        string         `json:"run_token"`
	Type            string         `json:"type" binding:"required"`
	Name            string         `json:"name" binding:"required"`
	MimeType        string         `json:"mime_type"`
	SizeBytes       int64          `json:"size_bytes"`
	SHA256          string         `json:"sha256"`
	StorageProvider string         `json:"storage_provider"`
	ObjectKey       string         `json:"object_key"`
	ObjectURL       string         `json:"object_url"`
	Metadata        map[string]any `json:"metadata"`
}

type agentRunResponse struct {
	ID                int64              `json:"id"`
	AppID             int64              `json:"app_id"`
	AppVersionID      int64              `json:"app_version_id"`
	UserID            int64              `json:"user_id"`
	APIKeyID          int64              `json:"api_key_id"`
	WorkerHostID      *int64             `json:"worker_host_id,omitempty"`
	Status            string             `json:"status"`
	InputRefURL       string             `json:"input_ref_url,omitempty"`
	InputSummaryJSON  map[string]any     `json:"input_summary_json"`
	OutputRefURL      string             `json:"output_ref_url,omitempty"`
	OutputSummaryJSON map[string]any     `json:"output_summary_json"`
	ErrorCode         string             `json:"error_code,omitempty"`
	ErrorMessage      string             `json:"error_message,omitempty"`
	UsageJSON         map[string]any     `json:"usage_json"`
	StartedAt         *string            `json:"started_at,omitempty"`
	CompletedAt       *string            `json:"completed_at,omitempty"`
	ExpiresAt         *string            `json:"expires_at,omitempty"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
	Artifacts         []artifactResponse `json:"artifacts,omitempty"`
}

// adminAgentRunAuditResponse intentionally excludes all input, output, artifact,
// object-storage and free-form error fields.
type adminAgentRunAuditResponse struct {
	ID             int64   `json:"id"`
	AppID          int64   `json:"app_id"`
	AppName        string  `json:"app_name"`
	AppVersionID   int64   `json:"app_version_id"`
	AppVersion     string  `json:"app_version"`
	UserID         int64   `json:"user_id"`
	UserEmail      string  `json:"user_email"`
	Username       string  `json:"username"`
	APIKeyID       int64   `json:"api_key_id"`
	APIKeyName     string  `json:"api_key_name"`
	WorkerHostID   *int64  `json:"worker_host_id,omitempty"`
	WorkerHostName string  `json:"worker_host_name,omitempty"`
	Status         string  `json:"status"`
	DurationMs     *int64  `json:"duration_ms,omitempty"`
	StartedAt      *string `json:"started_at,omitempty"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type artifactResponse struct {
	ID              int64          `json:"id"`
	RunID           int64          `json:"run_id"`
	ArtifactType    string         `json:"artifact_type"`
	Name            string         `json:"name"`
	MimeType        string         `json:"mime_type,omitempty"`
	StorageProvider string         `json:"storage_provider"`
	Bucket          string         `json:"bucket,omitempty"`
	ObjectKey       string         `json:"object_key"`
	ObjectURL       string         `json:"object_url"`
	SizeBytes       int64          `json:"size_bytes"`
	SHA256          string         `json:"sha256,omitempty"`
	MetadataJSON    map[string]any `json:"metadata_json"`
	ExpiresAt       *string        `json:"expires_at,omitempty"`
	CreatedAt       string         `json:"created_at"`
}

type agentRunEventResponse struct {
	ID           int64          `json:"id"`
	RunID        int64          `json:"run_id"`
	EventType    string         `json:"event_type"`
	Status       string         `json:"status,omitempty"`
	NodeID       string         `json:"node_id,omitempty"`
	Role         string         `json:"role,omitempty"`
	Message      string         `json:"message,omitempty"`
	Progress     *float64       `json:"progress,omitempty"`
	MetadataJSON map[string]any `json:"metadata_json"`
	CreatedAt    string         `json:"created_at"`
}

type inputAssetResponse struct {
	ID              int64          `json:"id"`
	RunID           *int64         `json:"run_id,omitempty"`
	UserID          int64          `json:"user_id"`
	AppID           *int64         `json:"app_id,omitempty"`
	FieldName       string         `json:"field_name,omitempty"`
	AssetType       string         `json:"asset_type"`
	AssetRole       string         `json:"asset_role,omitempty"`
	Name            string         `json:"name"`
	MimeType        string         `json:"mime_type,omitempty"`
	StorageProvider string         `json:"storage_provider"`
	Bucket          string         `json:"bucket,omitempty"`
	ObjectKey       string         `json:"object_key"`
	ObjectURL       string         `json:"object_url"`
	SizeBytes       int64          `json:"size_bytes"`
	SHA256          string         `json:"sha256,omitempty"`
	MetadataJSON    map[string]any `json:"metadata_json"`
	ExpiresAt       *string        `json:"expires_at,omitempty"`
	CreatedAt       string         `json:"created_at"`
}

type agentAppCatalogResponse struct {
	ID                 int64                          `json:"id"`
	Name               string                         `json:"name"`
	Slug               string                         `json:"slug"`
	Description        string                         `json:"description,omitempty"`
	IconURL            string                         `json:"icon_url,omitempty"`
	Category           string                         `json:"category,omitempty"`
	AppType            string                         `json:"app_type"`
	Visibility         string                         `json:"visibility"`
	Status             string                         `json:"status"`
	PublishedVersionID *int64                         `json:"published_version_id,omitempty"`
	CreatedAt          string                         `json:"created_at"`
	UpdatedAt          string                         `json:"updated_at"`
	PublishedVersion   *agentAppVersionPublicResponse `json:"published_version,omitempty"`
}

type agentAppVersionPublicResponse struct {
	ID                     int64          `json:"id"`
	AppID                  int64          `json:"app_id"`
	Version                string         `json:"version"`
	RuntimeType            string         `json:"runtime_type"`
	InputSchemaJSON        map[string]any `json:"input_schema_json"`
	OutputSchemaJSON       map[string]any `json:"output_schema_json"`
	CapabilitiesJSON       map[string]any `json:"capabilities_json"`
	DefaultModelConfigJSON map[string]any `json:"default_model_config_json"`
	NodeModelPolicyJSON    map[string]any `json:"node_model_policy_json"`
	ArtifactPolicyJSON     map[string]any `json:"artifact_policy_json"`
	PublishedAt            *string        `json:"published_at,omitempty"`
	CreatedAt              string         `json:"created_at"`
}

func (h *AgentRunHandler) ListPublishedApps(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.runService.ListPublishedApps(c.Request.Context(), pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentAppListFilters{
		AppType: c.Query("app_type"),
		Search:  c.Query("search"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentAppCatalogResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentAppToCatalogResponse(&items[i], nil))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentRunHandler) GetPublishedApp(c *gin.Context) {
	appID, ok := parseIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	app, version, err := h.runService.GetPublishedApp(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentAppToCatalogResponse(app, version))
}

func (h *AgentRunHandler) GetPublishedAppIconURL(c *gin.Context) {
	if _, ok := middleware.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	appID, ok := parseIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	result, err := h.runService.GetPublishedAppIconURL(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AgentRunHandler) CreateRun(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	appID, ok := parseIDParam(c, "id", "Invalid app ID")
	if !ok {
		return
	}
	var req createAgentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	run, err := h.runService.CreateRun(c.Request.Context(), service.CreateAgentRunInput{
		AppID:           appID,
		VersionID:       req.AppVersionID,
		UserID:          subject.UserID,
		APIKeyID:        req.APIKeyID,
		APIKeyBindings:  agentRunKeyBindingInputs(req.APIKeyBindings),
		InputAssetIDs:   req.InputAssetIDs,
		Input:           req.Input,
		InputRefURL:     strings.TrimSpace(req.InputRefURL),
		CallbackBaseURL: h.callbackBaseURL(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, agentRunToResponse(run, nil))
}

func (h *AgentRunHandler) callbackBaseURL(c *gin.Context) string {
	if h != nil && h.settingService != nil {
		if configured := h.settingService.GetAPIBaseURL(c.Request.Context()); configured != "" {
			return configured
		}
	}
	return requestBaseURL(c)
}

func (h *AgentRunHandler) ListRuns(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	var appID *int64
	if raw := strings.TrimSpace(c.Query("app_id")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			appID = &parsed
		}
	}
	items, result, err := h.runService.ListRuns(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentRunListFilters{
		AppID:  appID,
		Status: c.Query("status"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentRunResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentRunToResponse(&items[i], nil))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentRunHandler) ListRunsAdmin(c *gin.Context) {
	var appID *int64
	if raw := strings.TrimSpace(c.Query("app_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid app ID")
			return
		}
		appID = &parsed
	}
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.runService.ListRunsForAdmin(c.Request.Context(), pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentRunListFilters{
		AppID:  appID,
		Status: c.Query("status"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]adminAgentRunAuditResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentRunToAdminAuditResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentRunHandler) GetRun(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	run, artifacts, err := h.runService.GetRunForUser(c.Request.Context(), runID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentRunToResponse(run, artifacts))
}

func (h *AgentRunHandler) CancelRun(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	run, err := h.runService.CancelRun(c.Request.Context(), runID, subject.UserID, "user canceled run")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentRunToResponse(run, nil))
}

func (h *AgentRunHandler) ListEvents(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.runService.ListRunEventsForUser(c.Request.Context(), runID, subject.UserID, pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    "created_at",
		SortOrder: "asc",
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentRunEventResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentRunEventToResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentRunHandler) Callback(c *gin.Context) {
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	var req service.WorkerCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.RunID == 0 {
		req.RunID = runID
	}
	run, err := h.runService.HandleCallback(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentRunToResponse(run, nil))
}

func (h *AgentRunHandler) ModelProxy(c *gin.Context) {
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	var req modelProxyHandlerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	runToken := workerRunTokenFromRequest(c)
	if req.RunID == 0 {
		req.RunID = runID
	}
	proxyRequest := service.ModelProxyRequest{
		RunID:       req.RunID,
		NodeID:      strings.TrimSpace(req.NodeID),
		Role:        strings.TrimSpace(req.Role),
		Model:       strings.TrimSpace(req.Model),
		GroupID:     req.GroupID,
		Platform:    strings.TrimSpace(req.Platform),
		Endpoint:    strings.TrimSpace(req.Endpoint),
		Method:      strings.TrimSpace(req.Method),
		ContentType: strings.TrimSpace(req.ContentType),
		BodyBase64:  strings.TrimSpace(req.BodyBase64),
		Multipart:   req.Multipart,
		Request:     req.Request,
		Metadata:    req.Metadata,
	}
	if stream, _ := proxyRequest.Request["stream"].(bool); stream {
		h.streamModelProxy(c, runID, proxyRequest, runToken)
		return
	}
	result, err := h.runService.HandleModelProxy(c.Request.Context(), runID, proxyRequest, runToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AgentRunHandler) streamModelProxy(c *gin.Context, runID int64, req service.ModelProxyRequest, runToken string) {
	stream, err := h.runService.OpenModelProxyStream(c.Request.Context(), runID, req, runToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if stream == nil || stream.Response == nil || stream.Response.Body == nil {
		response.InternalError(c, "Streaming model proxy is unavailable")
		return
	}
	defer func() { _ = stream.Response.Body.Close() }()
	streamErr := writeAgentModelProxyStream(c, stream.Response)
	h.runService.FinishModelProxyStream(context.WithoutCancel(c.Request.Context()), stream, streamErr)
}

func writeAgentModelProxyStream(c *gin.Context, stream *service.ModelProxyStreamResponse) error {
	for name, value := range stream.Headers {
		if strings.EqualFold(name, "Content-Type") || strings.EqualFold(name, "Content-Length") {
			continue
		}
		c.Header(name, value)
	}
	contentType := strings.TrimSpace(stream.ContentType)
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	status := stream.Status
	if status <= 0 {
		status = http.StatusOK
	}
	c.Status(status)
	c.Writer.Flush()

	buffer := make([]byte, 32*1024)
	for {
		readCount, readErr := stream.Body.Read(buffer)
		if readCount > 0 {
			if _, writeErr := c.Writer.Write(buffer[:readCount]); writeErr != nil {
				return writeErr
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				return readErr
			}
			return nil
		}
	}
}

func (h *AgentRunHandler) RegisterArtifact(c *gin.Context) {
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	var req artifactRegisterHandlerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	runToken := workerRunTokenFromRequest(c)
	result, err := h.runService.RegisterArtifact(c.Request.Context(), runID, service.ArtifactCreateRequest{
		RunID:           req.RunID,
		RunToken:        req.RunToken,
		Type:            strings.TrimSpace(req.Type),
		Name:            strings.TrimSpace(req.Name),
		MimeType:        strings.TrimSpace(req.MimeType),
		SizeBytes:       req.SizeBytes,
		SHA256:          strings.TrimSpace(req.SHA256),
		StorageProvider: strings.TrimSpace(req.StorageProvider),
		ObjectKey:       strings.TrimSpace(req.ObjectKey),
		ObjectURL:       strings.TrimSpace(req.ObjectURL),
		Metadata:        req.Metadata,
	}, runToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AgentRunHandler) UploadArtifact(c *gin.Context) {
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invalid artifact file: "+err.Error())
		return
	}
	src, err := file.Open()
	if err != nil {
		response.BadRequest(c, "Invalid artifact file: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	metadata := map[string]any{}
	if raw := strings.TrimSpace(c.PostForm("metadata")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			response.BadRequest(c, "Invalid metadata JSON: "+err.Error())
			return
		}
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = file.Filename
	}
	result, err := h.runService.UploadArtifact(c.Request.Context(), service.ArtifactUploadInput{
		RunID:     runID,
		RunToken:  workerRunTokenFromRequest(c),
		Type:      strings.TrimSpace(c.DefaultPostForm("type", service.AgentArtifactTypeOutput)),
		Name:      name,
		MimeType:  strings.TrimSpace(c.PostForm("mime_type")),
		SizeBytes: file.Size,
		SHA256:    strings.TrimSpace(c.PostForm("sha256")),
		Metadata:  metadata,
		Body:      src,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AgentRunHandler) ListArtifacts(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	runID, ok := parseIDParam(c, "id", "Invalid run ID")
	if !ok {
		return
	}
	artifacts, err := h.runService.ListArtifactsForUserRun(c.Request.Context(), runID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]artifactResponse, 0, len(artifacts))
	for i := range artifacts {
		out = append(out, *artifactToResponse(&artifacts[i]))
	}
	response.Success(c, out)
}

func (h *AgentRunHandler) GetArtifactDownloadURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	artifactID, ok := parseIDParam(c, "id", "Invalid artifact ID")
	if !ok {
		return
	}
	result, err := h.runService.GetArtifactDownloadURL(c.Request.Context(), artifactID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AgentRunHandler) GetArtifactPreviewURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	artifactID, ok := parseIDParam(c, "id", "Invalid artifact ID")
	if !ok {
		return
	}
	result, err := h.runService.GetArtifactPreviewURL(c.Request.Context(), artifactID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, result)
}

func (h *AgentRunHandler) StreamArtifactPreview(c *gin.Context) {
	artifactID, ok := parseIDParam(c, "id", "Invalid artifact ID")
	if !ok {
		return
	}
	content, err := h.runService.OpenArtifactPreview(
		c.Request.Context(),
		artifactID,
		strings.TrimSpace(c.Query("token")),
		c.GetHeader("Range"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer func() { _ = content.Body.Close() }()
	writeArtifactPreviewContent(c, content)
}

func writeArtifactPreviewContent(c *gin.Context, content *service.ArtifactPreviewContent) {
	if content.ContentType != "" {
		c.Header("Content-Type", content.ContentType)
	}
	c.Header("Content-Disposition", "inline")
	c.Header("Accept-Ranges", content.AcceptRanges)
	c.Header("Cache-Control", "private, no-store")
	if content.ContentRange != "" {
		c.Header("Content-Range", content.ContentRange)
	}
	if content.ETag != "" {
		c.Header("ETag", content.ETag)
	}
	if content.LastModified != "" {
		c.Header("Last-Modified", content.LastModified)
	}
	if content.StatusCode != http.StatusRequestedRangeNotSatisfiable && content.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(content.ContentLength, 10))
	}
	c.Status(content.StatusCode)
	c.Writer.WriteHeaderNow()
	if content.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		_, _ = io.Copy(c.Writer, content.Body)
	}
}

func (h *AgentRunHandler) UploadInputAsset(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invalid input file: "+err.Error())
		return
	}
	src, err := file.Open()
	if err != nil {
		response.BadRequest(c, "Invalid input file: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	metadata := map[string]any{}
	if raw := strings.TrimSpace(c.PostForm("metadata")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			response.BadRequest(c, "Invalid metadata JSON: "+err.Error())
			return
		}
	}
	var appID *int64
	if raw := strings.TrimSpace(c.PostForm("app_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid app ID")
			return
		}
		appID = &parsed
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = file.Filename
	}
	mimeType := strings.TrimSpace(c.PostForm("mime_type"))
	if mimeType == "" {
		mimeType = strings.TrimSpace(file.Header.Get("Content-Type"))
	}
	asset, err := h.runService.UploadInputAsset(c.Request.Context(), service.InputAssetUploadInput{
		UserID:    subject.UserID,
		AppID:     appID,
		FieldName: strings.TrimSpace(c.PostForm("field_name")),
		AssetType: strings.TrimSpace(c.PostForm("asset_type")),
		AssetRole: strings.TrimSpace(c.PostForm("asset_role")),
		Name:      name,
		MimeType:  mimeType,
		SizeBytes: file.Size,
		SHA256:    strings.TrimSpace(c.PostForm("sha256")),
		Metadata:  metadata,
		Body:      src,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, inputAssetToResponse(asset))
}

func (h *AgentRunHandler) ListInputAssets(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	var appID *int64
	if raw := strings.TrimSpace(c.Query("app_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid app ID")
			return
		}
		appID = &parsed
	}
	items, result, err := h.runService.ListInputAssets(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentInputAssetListFilters{
		AppID:     appID,
		AssetType: c.Query("asset_type"),
		Search:    c.Query("search"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]inputAssetResponse, 0, len(items))
	for i := range items {
		out = append(out, *inputAssetToResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentRunHandler) GetInputAssetDownloadURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	assetID, ok := parseIDParam(c, "id", "Invalid input asset ID")
	if !ok {
		return
	}
	result, err := h.runService.GetInputAssetDownloadURL(c.Request.Context(), assetID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func workerRunTokenFromRequest(c *gin.Context) string {
	runToken := strings.TrimSpace(c.GetHeader("X-Sub2API-Run-Token"))
	if runToken == "" {
		runToken = strings.TrimSpace(c.GetHeader("X-Sub2API-Agent-Run-Token"))
	}
	if runToken == "" {
		runToken = strings.TrimSpace(c.GetHeader("Authorization"))
		runToken = strings.TrimPrefix(runToken, "Bearer ")
	}
	return strings.TrimSpace(runToken)
}

func agentRunKeyBindingInputs(items []createAgentRunKeyBindingRequest) []service.CreateAgentRunKeyBindingInput {
	if len(items) == 0 {
		return nil
	}
	out := make([]service.CreateAgentRunKeyBindingInput, 0, len(items))
	for _, item := range items {
		out = append(out, service.CreateAgentRunKeyBindingInput{
			PolicyKey: strings.TrimSpace(item.PolicyKey),
			NodeID:    strings.TrimSpace(item.NodeID),
			Role:      strings.TrimSpace(item.Role),
			APIKeyID:  item.APIKeyID,
		})
	}
	return out
}

func parseIDParam(c *gin.Context, key, message string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, message)
		return 0, false
	}
	return id, true
}

func requestBaseURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if c.Request != nil && c.Request.TLS != nil {
			scheme = "https"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" && c.Request != nil {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func agentAppToCatalogResponse(app *service.AgentApp, version *service.AgentAppVersion) *agentAppCatalogResponse {
	if app == nil {
		return nil
	}
	return &agentAppCatalogResponse{
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
		CreatedAt:          app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          app.UpdatedAt.Format(time.RFC3339),
		PublishedVersion:   agentAppVersionToPublicResponse(version),
	}
}

func agentAppVersionToPublicResponse(version *service.AgentAppVersion) *agentAppVersionPublicResponse {
	if version == nil {
		return nil
	}
	return &agentAppVersionPublicResponse{
		ID:                     version.ID,
		AppID:                  version.AppID,
		Version:                version.Version,
		RuntimeType:            version.RuntimeType,
		InputSchemaJSON:        version.InputSchemaJSON,
		OutputSchemaJSON:       version.OutputSchemaJSON,
		CapabilitiesJSON:       version.CapabilitiesJSON,
		DefaultModelConfigJSON: version.DefaultModelConfigJSON,
		NodeModelPolicyJSON:    version.NodeModelPolicyJSON,
		ArtifactPolicyJSON:     version.ArtifactPolicyJSON,
		PublishedAt:            formatOptionalTime(version.PublishedAt),
		CreatedAt:              version.CreatedAt.Format(time.RFC3339),
	}
}

func agentRunToResponse(run *service.AgentRun, artifacts []service.AgentArtifact) *agentRunResponse {
	if run == nil {
		return nil
	}
	out := &agentRunResponse{
		ID:                run.ID,
		AppID:             run.AppID,
		AppVersionID:      run.AppVersionID,
		UserID:            run.UserID,
		APIKeyID:          run.APIKeyID,
		WorkerHostID:      run.WorkerHostID,
		Status:            run.Status,
		InputRefURL:       run.InputRefURL,
		InputSummaryJSON:  run.InputSummaryJSON,
		OutputRefURL:      run.OutputRefURL,
		OutputSummaryJSON: run.OutputSummaryJSON,
		ErrorCode:         run.ErrorCode,
		ErrorMessage:      run.ErrorMessage,
		UsageJSON:         run.UsageJSON,
		StartedAt:         formatOptionalTime(run.StartedAt),
		CompletedAt:       formatOptionalTime(run.CompletedAt),
		ExpiresAt:         formatOptionalTime(run.ExpiresAt),
		CreatedAt:         run.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         run.UpdatedAt.Format(time.RFC3339),
	}
	if len(artifacts) > 0 {
		out.Artifacts = make([]artifactResponse, 0, len(artifacts))
		for i := range artifacts {
			out.Artifacts = append(out.Artifacts, *artifactToResponse(&artifacts[i]))
		}
	}
	return out
}

func agentRunToAdminAuditResponse(run *service.AgentRun) *adminAgentRunAuditResponse {
	if run == nil {
		return nil
	}
	var durationMs *int64
	if run.StartedAt != nil && run.CompletedAt != nil {
		value := run.CompletedAt.Sub(*run.StartedAt).Milliseconds()
		if value < 0 {
			value = 0
		}
		durationMs = &value
	}
	return &adminAgentRunAuditResponse{
		ID:             run.ID,
		AppID:          run.AppID,
		AppName:        run.AppName,
		AppVersionID:   run.AppVersionID,
		AppVersion:     run.AppVersion,
		UserID:         run.UserID,
		UserEmail:      run.UserEmail,
		Username:       run.Username,
		APIKeyID:       run.APIKeyID,
		APIKeyName:     run.APIKeyName,
		WorkerHostID:   run.WorkerHostID,
		WorkerHostName: run.WorkerHostName,
		Status:         run.Status,
		DurationMs:     durationMs,
		StartedAt:      formatOptionalTime(run.StartedAt),
		CompletedAt:    formatOptionalTime(run.CompletedAt),
		CreatedAt:      run.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      run.UpdatedAt.Format(time.RFC3339),
	}
}

func artifactToResponse(artifact *service.AgentArtifact) *artifactResponse {
	if artifact == nil {
		return nil
	}
	return &artifactResponse{
		ID:              artifact.ID,
		RunID:           artifact.RunID,
		ArtifactType:    artifact.ArtifactType,
		Name:            artifact.Name,
		MimeType:        artifact.MimeType,
		StorageProvider: artifact.StorageProvider,
		Bucket:          artifact.Bucket,
		ObjectKey:       artifact.ObjectKey,
		ObjectURL:       artifact.ObjectURL,
		SizeBytes:       artifact.SizeBytes,
		SHA256:          artifact.SHA256,
		MetadataJSON:    artifact.MetadataJSON,
		ExpiresAt:       formatOptionalTime(artifact.ExpiresAt),
		CreatedAt:       artifact.CreatedAt.Format(time.RFC3339),
	}
}

func agentRunEventToResponse(event *service.AgentRunEvent) *agentRunEventResponse {
	if event == nil {
		return nil
	}
	return &agentRunEventResponse{
		ID:           event.ID,
		RunID:        event.RunID,
		EventType:    event.EventType,
		Status:       event.Status,
		NodeID:       event.NodeID,
		Role:         event.Role,
		Message:      event.Message,
		Progress:     event.Progress,
		MetadataJSON: event.MetadataJSON,
		CreatedAt:    event.CreatedAt.Format(time.RFC3339),
	}
}

func inputAssetToResponse(asset *service.AgentInputAsset) *inputAssetResponse {
	if asset == nil {
		return nil
	}
	return &inputAssetResponse{
		ID:              asset.ID,
		RunID:           asset.RunID,
		UserID:          asset.UserID,
		AppID:           asset.AppID,
		FieldName:       asset.FieldName,
		AssetType:       asset.AssetType,
		AssetRole:       asset.AssetRole,
		Name:            asset.Name,
		MimeType:        asset.MimeType,
		StorageProvider: asset.StorageProvider,
		Bucket:          asset.Bucket,
		ObjectKey:       asset.ObjectKey,
		ObjectURL:       asset.ObjectURL,
		SizeBytes:       asset.SizeBytes,
		SHA256:          asset.SHA256,
		MetadataJSON:    asset.MetadataJSON,
		ExpiresAt:       formatOptionalTime(asset.ExpiresAt),
		CreatedAt:       asset.CreatedAt.Format(time.RFC3339),
	}
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}
