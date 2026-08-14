package admin

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// VideoJobHandler provides admin operations for video generation jobs.
type VideoJobHandler struct {
	gateway    *service.OpenAIGatewayService
	canvas     *service.CreazyCanvasService
	imageTasks *service.ImageTaskService
}

func NewVideoJobHandler(gateway *service.OpenAIGatewayService, canvas *service.CreazyCanvasService, imageTasks *service.ImageTaskService) *VideoJobHandler {
	return &VideoJobHandler{gateway: gateway, canvas: canvas, imageTasks: imageTasks}
}

type adminVideoJobDTO struct {
	ID                 int64          `json:"id"`
	JobID              string         `json:"job_id"`
	UpstreamJobID      string         `json:"upstream_job_id"`
	UserID             int64          `json:"user_id"`
	UserEmail          string         `json:"user_email"`
	Username           string         `json:"username"`
	APIKeyID           int64          `json:"api_key_id"`
	APIKeyName         string         `json:"api_key_name"`
	GroupID            int64          `json:"group_id"`
	GroupName          string         `json:"group_name"`
	AccountID          int64          `json:"account_id"`
	Model              string         `json:"model"`
	FallbackModel      string         `json:"fallback_model,omitempty"`
	FallbackStatus     string         `json:"fallback_status,omitempty"`
	TaskStatus         string         `json:"task_status"`
	RefundStatus       string         `json:"refund_status"`
	RefundAttempts     int            `json:"refund_attempts"`
	SettlementAttempts int            `json:"settlement_attempts"`
	LastError          string         `json:"last_error,omitempty"`
	Prompt             string         `json:"prompt,omitempty"`
	RequestSnapshot    map[string]any `json:"request_snapshot,omitempty"`
	ResultPath         string         `json:"result_path,omitempty"`
	NextPollAt         *string        `json:"next_poll_at,omitempty"`
	LastPolledAt       *string        `json:"last_polled_at,omitempty"`
	SettledAt          *string        `json:"settled_at,omitempty"`
	RefundedAt         *string        `json:"refunded_at,omitempty"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type killVideoJobRequest struct {
	Reason string `json:"reason"`
}

type adminImageJobDTO struct {
	ID               int64          `json:"id"`
	TaskID           string         `json:"task_id"`
	UserID           int64          `json:"user_id"`
	UserEmail        string         `json:"user_email"`
	Username         string         `json:"username"`
	APIKeyID         int64          `json:"api_key_id"`
	APIKeyName       string         `json:"api_key_name"`
	GroupID          *int64         `json:"group_id,omitempty"`
	GroupName        string         `json:"group_name"`
	Model            string         `json:"model"`
	Status           string         `json:"status"`
	GatewayType      string         `json:"gateway_type"`
	GatewayRemoteID  string         `json:"gateway_remote_id"`
	Prompt           string         `json:"prompt"`
	Params           map[string]any `json:"params"`
	PreviewURL       string         `json:"preview_url,omitempty"`
	ObjectURL        string         `json:"object_url,omitempty"`
	MimeType         string         `json:"mime_type,omitempty"`
	SizeBytes        int64          `json:"size_bytes"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	CanTerminate     bool           `json:"can_terminate"`
	TerminationScope string         `json:"termination_scope"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	ExpiresAt        string         `json:"expires_at"`
}

type terminateImageJobRequest struct {
	Reason string `json:"reason"`
}

// ListImages GET /admin/image-jobs
func (h *VideoJobHandler) ListImages(c *gin.Context) {
	if h == nil || h.canvas == nil {
		response.Error(c, http.StatusServiceUnavailable, "image job admin is unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filters := service.CreazyCanvasAdminWorkFilters{
		Status:      strings.TrimSpace(c.Query("status")),
		GatewayType: strings.TrimSpace(c.Query("gateway_type")),
		Search:      strings.TrimSpace(c.Query("search")),
	}
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}
	if raw := strings.TrimSpace(c.Query("active_only")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid active_only")
			return
		}
		filters.ActiveOnly = value
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    strings.TrimSpace(c.DefaultQuery("sort_by", "created_at")),
		SortOrder: strings.TrimSpace(c.DefaultQuery("sort_order", pagination.SortOrderDesc)),
	}
	items, result, err := h.canvas.AdminListImageWorks(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]adminImageJobDTO, 0, len(items))
	for i := range items {
		out = append(out, toAdminImageJobDTO(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

// GetImage GET /admin/image-jobs/:id
func (h *VideoJobHandler) GetImage(c *gin.Context) {
	if h == nil || h.canvas == nil {
		response.Error(c, http.StatusServiceUnavailable, "image job admin is unavailable")
		return
	}
	workID, ok := parseAdminImageWorkID(c)
	if !ok {
		return
	}
	work, err := h.canvas.AdminGetImageWork(c.Request.Context(), workID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminImageJobDTO(work))
}

// TerminateImage POST /admin/image-jobs/:id/terminate
func (h *VideoJobHandler) TerminateImage(c *gin.Context) {
	if h == nil || h.canvas == nil {
		response.Error(c, http.StatusServiceUnavailable, "image job admin is unavailable")
		return
	}
	workID, ok := parseAdminImageWorkID(c)
	if !ok {
		return
	}
	var req terminateImageJobRequest
	_ = c.ShouldBindJSON(&req)
	work, err := h.canvas.AdminGetImageWork(c.Request.Context(), workID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if work.GatewayType == service.CreazyCanvasGatewayImageTask && work.GatewayRemoteID != "" && h.imageTasks != nil {
		// The Redis task may have expired or completed between list and action. The
		// canvas record remains authoritative for the admin desk, so cancellation
		// is best-effort and local termination still proceeds.
		_, _ = h.imageTasks.Cancel(c.Request.Context(), work.GatewayRemoteID, req.Reason)
	}
	work, err = h.canvas.AdminTerminateImageWork(c.Request.Context(), workID, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminImageJobDTO(work))
}

// ImageContent GET /admin/image-jobs/:id/content
func (h *VideoJobHandler) ImageContent(c *gin.Context) {
	if h == nil || h.canvas == nil {
		response.Error(c, http.StatusServiceUnavailable, "image job admin is unavailable")
		return
	}
	workID, ok := parseAdminImageWorkID(c)
	if !ok {
		return
	}
	content, err := h.canvas.OpenAdminImageWorkContent(c.Request.Context(), workID, c.GetHeader("Range"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if content.RedirectURL != "" {
		if strings.HasPrefix(content.RedirectURL, "data:") {
			response.Success(c, gin.H{"url": content.RedirectURL, "source": "inline"})
			return
		}
		c.Redirect(http.StatusFound, content.RedirectURL)
		return
	}
	if content.Body != nil {
		defer func() { _ = content.Body.Close() }()
	}
	status := content.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	if content.ContentType != "" {
		c.Header("Content-Type", content.ContentType)
	}
	if content.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(content.ContentLength, 10))
	}
	if content.Filename != "" {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", content.Filename))
	}
	if content.Header != nil {
		for _, key := range []string{"Accept-Ranges", "Content-Range", "ETag", "Last-Modified", "Cache-Control"} {
			if value := content.Header.Get(key); value != "" {
				c.Header(key, value)
			}
		}
	}
	c.Status(status)
	if content.Body != nil {
		_, _ = io.Copy(c.Writer, content.Body)
	}
}

func parseAdminImageWorkID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid image job id")
		return 0, false
	}
	return id, true
}

func toAdminImageJobDTO(item *service.CreazyCanvasAdminWork) adminImageJobDTO {
	if item == nil {
		return adminImageJobDTO{}
	}
	taskID := strings.TrimSpace(item.GatewayRemoteID)
	if taskID == "" {
		taskID = fmt.Sprintf("canvas-%d", item.ID)
	}
	status := strings.ToLower(strings.TrimSpace(item.Status))
	canTerminate := status == service.CreazyCanvasWorkStatusCreated || status == service.CreazyCanvasWorkStatusQueued || status == service.CreazyCanvasWorkStatusRunning
	scope := "local_record"
	if item.GatewayType == service.CreazyCanvasGatewayImageTask && item.GatewayRemoteID != "" {
		scope = "async_execution"
	}
	params := item.ParamsJSON
	if params == nil {
		params = map[string]any{}
	}
	return adminImageJobDTO{
		ID:               item.ID,
		TaskID:           taskID,
		UserID:           item.UserID,
		UserEmail:        item.UserEmail,
		Username:         item.Username,
		APIKeyID:         item.APIKeyID,
		APIKeyName:       item.APIKeyName,
		GroupID:          item.GroupID,
		GroupName:        item.GroupName,
		Model:            item.PublicModel,
		Status:           item.Status,
		GatewayType:      item.GatewayType,
		GatewayRemoteID:  item.GatewayRemoteID,
		Prompt:           item.Prompt,
		Params:           params,
		PreviewURL:       item.PreviewURL,
		ObjectURL:        item.ObjectURL,
		MimeType:         item.MimeType,
		SizeBytes:        item.SizeBytes,
		ErrorMessage:     item.ErrorMessage,
		CanTerminate:     canTerminate,
		TerminationScope: scope,
		CreatedAt:        item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.UTC().Format(time.RFC3339),
		ExpiresAt:        item.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

// List GET /admin/video-jobs
func (h *VideoJobHandler) List(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filters := service.SeedanceTaskAdminFilters{
		JobID:  strings.TrimSpace(c.Query("job_id")),
		Status: strings.TrimSpace(c.Query("status")),
		Model:  strings.TrimSpace(c.Query("model")),
		Search: strings.TrimSpace(c.Query("search")),
	}
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filters.UserID = id
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filters.GroupID = id
	}
	if raw := strings.TrimSpace(c.Query("api_key_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		filters.APIKeyID = id
	}
	if raw := strings.TrimSpace(c.Query("unsettled_only")); raw != "" {
		val, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid unsettled_only")
			return
		}
		filters.UnsettledOnly = val
	}

	items, total, err := h.gateway.AdminListSeedanceVideoJobs(c.Request.Context(), filters, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]adminVideoJobDTO, 0, len(items))
	for i := range items {
		out = append(out, toAdminVideoJobDTO(&items[i]))
	}
	response.Paginated(c, out, total, page, pageSize)
}

// Get GET /admin/video-jobs/:job_id
func (h *VideoJobHandler) Get(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "job_id is required")
		return
	}
	item, err := h.gateway.AdminGetSeedanceVideoJob(c.Request.Context(), jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			response.Error(c, http.StatusNotFound, "video job not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	dto := toAdminVideoJobDTO(item)
	response.Success(c, dto)
}

// Sync POST /admin/video-jobs/:job_id/sync
func (h *VideoJobHandler) Sync(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "job_id is required")
		return
	}
	item, err := h.gateway.AdminSyncSeedanceVideoJob(c.Request.Context(), jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			response.Error(c, http.StatusNotFound, "video job not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminVideoJobDTO(item))
}

// Kill POST /admin/video-jobs/:job_id/kill
func (h *VideoJobHandler) Kill(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "job_id is required")
		return
	}
	var req killVideoJobRequest
	_ = c.ShouldBindJSON(&req)
	item, err := h.gateway.AdminKillSeedanceVideoJob(c.Request.Context(), jobID, req.Reason)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			response.Error(c, http.StatusNotFound, "video job not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminVideoJobDTO(item))
}

// ForceFail POST /admin/video-jobs/:job_id/force-fail
func (h *VideoJobHandler) ForceFail(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "job_id is required")
		return
	}
	var req killVideoJobRequest
	_ = c.ShouldBindJSON(&req)
	item, err := h.gateway.AdminForceFailSeedanceVideoJob(c.Request.Context(), jobID, req.Reason)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			response.Error(c, http.StatusNotFound, "video job not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toAdminVideoJobDTO(item))
}

func toAdminVideoJobDTO(item *service.SeedanceTaskAdminItem) adminVideoJobDTO {
	if item == nil {
		return adminVideoJobDTO{}
	}
	snapshot := service.ParseSeedanceRequestSnapshot(item.RequestSnapshot)
	prompt, _ := snapshot["prompt"].(string)
	dto := adminVideoJobDTO{
		ID:                 item.ID,
		JobID:              item.JobID,
		UpstreamJobID:      item.UpstreamJobID,
		UserID:             item.UserID,
		UserEmail:          item.UserEmail,
		Username:           item.Username,
		APIKeyID:           item.APIKeyID,
		APIKeyName:         item.APIKeyName,
		GroupID:            item.GroupID,
		GroupName:          item.GroupName,
		AccountID:          item.AccountID,
		Model:              item.Model,
		FallbackModel:      item.FallbackModel,
		FallbackStatus:     item.FallbackStatus,
		TaskStatus:         item.TaskStatus,
		RefundStatus:       item.RefundStatus,
		RefundAttempts:     item.RefundAttempts,
		SettlementAttempts: item.SettlementAttempts,
		LastError:          item.LastError,
		Prompt:             prompt,
		RequestSnapshot:    snapshot,
		CreatedAt:          item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          item.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if item.TaskStatus == service.SeedanceTaskStatusSucceeded {
		dto.ResultPath = service.SeedancePublicResultPath(item.JobID)
	}
	if !item.NextPollAt.IsZero() {
		v := item.NextPollAt.UTC().Format(time.RFC3339)
		dto.NextPollAt = &v
	}
	if !item.LastPolledAt.IsZero() {
		v := item.LastPolledAt.UTC().Format(time.RFC3339)
		dto.LastPolledAt = &v
	}
	if !item.SettledAt.IsZero() {
		v := item.SettledAt.UTC().Format(time.RFC3339)
		dto.SettledAt = &v
	}
	if !item.RefundedAt.IsZero() {
		v := item.RefundedAt.UTC().Format(time.RFC3339)
		dto.RefundedAt = &v
	}
	return dto
}
