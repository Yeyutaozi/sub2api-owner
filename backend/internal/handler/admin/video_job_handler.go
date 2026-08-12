package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// VideoJobHandler provides admin operations for video generation jobs.
type VideoJobHandler struct {
	gateway *service.OpenAIGatewayService
}

func NewVideoJobHandler(gateway *service.OpenAIGatewayService) *VideoJobHandler {
	return &VideoJobHandler{gateway: gateway}
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

// List GET /admin/video-jobs
func (h *VideoJobHandler) List(c *gin.Context) {
	if h == nil || h.gateway == nil {
		response.Error(c, http.StatusServiceUnavailable, "video job admin is unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filters := service.SeedanceTaskAdminFilters{
		JobID:    strings.TrimSpace(c.Query("job_id")),
		Status:   strings.TrimSpace(c.Query("status")),
		Model:    strings.TrimSpace(c.Query("model")),
		Search:   strings.TrimSpace(c.Query("search")),
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
