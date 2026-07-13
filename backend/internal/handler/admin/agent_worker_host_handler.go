package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AgentWorkerHostHandler struct {
	workerHostService *service.AgentWorkerHostService
}

func NewAgentWorkerHostHandler(workerHostService *service.AgentWorkerHostService) *AgentWorkerHostHandler {
	return &AgentWorkerHostHandler{workerHostService: workerHostService}
}

type agentWorkerHostRequest struct {
	Name           string         `json:"name" binding:"required,max=100"`
	BaseURL        string         `json:"base_url" binding:"required"`
	Protocol       string         `json:"protocol"`
	AuthType       string         `json:"auth_type"`
	SecretRef      string         `json:"secret_ref"`
	HealthPath     string         `json:"health_path"`
	RunPath        string         `json:"run_path"`
	CancelPath     string         `json:"cancel_path"`
	MaxConcurrency int            `json:"max_concurrency" binding:"omitempty,min=1"`
	TimeoutSeconds int            `json:"timeout_seconds" binding:"omitempty,min=1"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata"`
}

type agentWorkerHostResponse struct {
	ID                  int64          `json:"id"`
	Name                string         `json:"name"`
	BaseURL             string         `json:"base_url"`
	Protocol            string         `json:"protocol"`
	AuthType            string         `json:"auth_type"`
	SecretRef           string         `json:"secret_ref,omitempty"`
	HealthPath          string         `json:"health_path"`
	RunPath             string         `json:"run_path"`
	CancelPath          string         `json:"cancel_path,omitempty"`
	MaxConcurrency      int            `json:"max_concurrency"`
	TimeoutSeconds      int            `json:"timeout_seconds"`
	Status              string         `json:"status"`
	LastHealthStatus    string         `json:"last_health_status"`
	LastHealthMessage   string         `json:"last_health_message,omitempty"`
	LastHealthLatencyMS *int           `json:"last_health_latency_ms,omitempty"`
	LastCheckedAt       *string        `json:"last_checked_at,omitempty"`
	Metadata            map[string]any `json:"metadata"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
}

type agentWorkerHostHealthResponse struct {
	Success    bool                          `json:"success"`
	Status     string                        `json:"status"`
	StatusCode int                           `json:"status_code,omitempty"`
	LatencyMS  int                           `json:"latency_ms"`
	Message    string                        `json:"message"`
	CheckedAt  string                        `json:"checked_at"`
	Response   *service.WorkerHealthResponse `json:"response,omitempty"`
	Host       *agentWorkerHostResponse      `json:"host"`
}

func (h *AgentWorkerHostHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.workerHostService.List(c.Request.Context(), pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "id"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, service.AgentWorkerHostListFilters{
		Status: c.Query("status"),
		Search: c.Query("search"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentWorkerHostResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentWorkerHostToResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *AgentWorkerHostHandler) ListAll(c *gin.Context) {
	items, err := h.workerHostService.ListAll(c.Request.Context(), c.Query("status"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]agentWorkerHostResponse, 0, len(items))
	for i := range items {
		out = append(out, *agentWorkerHostToResponse(&items[i]))
	}
	response.Success(c, out)
}

func (h *AgentWorkerHostHandler) GetByID(c *gin.Context) {
	id, ok := parseAgentIDParam(c, "id", "Invalid Worker Host ID")
	if !ok {
		return
	}
	host, err := h.workerHostService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentWorkerHostToResponse(host))
}

func (h *AgentWorkerHostHandler) Create(c *gin.Context) {
	var req agentWorkerHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	host, err := h.workerHostService.Create(c.Request.Context(), service.CreateAgentWorkerHostInput{
		Name:           strings.TrimSpace(req.Name),
		BaseURL:        strings.TrimSpace(req.BaseURL),
		Protocol:       strings.TrimSpace(req.Protocol),
		AuthType:       strings.TrimSpace(req.AuthType),
		SecretRef:      strings.TrimSpace(req.SecretRef),
		HealthPath:     strings.TrimSpace(req.HealthPath),
		RunPath:        strings.TrimSpace(req.RunPath),
		CancelPath:     strings.TrimSpace(req.CancelPath),
		MaxConcurrency: req.MaxConcurrency,
		TimeoutSeconds: req.TimeoutSeconds,
		Status:         strings.TrimSpace(req.Status),
		Metadata:       req.Metadata,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, agentWorkerHostToResponse(host))
}

func (h *AgentWorkerHostHandler) Update(c *gin.Context) {
	id, ok := parseAgentIDParam(c, "id", "Invalid Worker Host ID")
	if !ok {
		return
	}
	var req agentWorkerHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	host, err := h.workerHostService.Update(c.Request.Context(), id, service.UpdateAgentWorkerHostInput{
		Name:           strings.TrimSpace(req.Name),
		BaseURL:        strings.TrimSpace(req.BaseURL),
		Protocol:       strings.TrimSpace(req.Protocol),
		AuthType:       strings.TrimSpace(req.AuthType),
		SecretRef:      strings.TrimSpace(req.SecretRef),
		HealthPath:     strings.TrimSpace(req.HealthPath),
		RunPath:        strings.TrimSpace(req.RunPath),
		CancelPath:     strings.TrimSpace(req.CancelPath),
		MaxConcurrency: req.MaxConcurrency,
		TimeoutSeconds: req.TimeoutSeconds,
		Status:         strings.TrimSpace(req.Status),
		Metadata:       req.Metadata,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentWorkerHostToResponse(host))
}

func (h *AgentWorkerHostHandler) Delete(c *gin.Context) {
	id, ok := parseAgentIDParam(c, "id", "Invalid Worker Host ID")
	if !ok {
		return
	}
	if err := h.workerHostService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Worker Host deleted successfully"})
}

func (h *AgentWorkerHostHandler) HealthCheck(c *gin.Context) {
	id, ok := parseAgentIDParam(c, "id", "Invalid Worker Host ID")
	if !ok {
		return
	}
	result, err := h.workerHostService.HealthCheck(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentWorkerHostHealthToResponse(result))
}

func parseAgentIDParam(c *gin.Context, key, message string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, message)
		return 0, false
	}
	return id, true
}

func agentWorkerHostToResponse(host *service.AgentWorkerHost) *agentWorkerHostResponse {
	if host == nil {
		return nil
	}
	return &agentWorkerHostResponse{
		ID:                  host.ID,
		Name:                host.Name,
		BaseURL:             host.BaseURL,
		Protocol:            host.Protocol,
		AuthType:            host.AuthType,
		SecretRef:           host.SecretRef,
		HealthPath:          host.HealthPath,
		RunPath:             host.RunPath,
		CancelPath:          host.CancelPath,
		MaxConcurrency:      host.MaxConcurrency,
		TimeoutSeconds:      host.TimeoutSeconds,
		Status:              host.Status,
		LastHealthStatus:    host.LastHealthStatus,
		LastHealthMessage:   host.LastHealthMessage,
		LastHealthLatencyMS: host.LastHealthLatencyMS,
		LastCheckedAt:       formatTimePtr(host.LastCheckedAt),
		Metadata:            host.Metadata,
		CreatedAt:           host.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           host.UpdatedAt.Format(time.RFC3339),
	}
}

func agentWorkerHostHealthToResponse(result *service.AgentWorkerHostHealthResult) *agentWorkerHostHealthResponse {
	if result == nil {
		return nil
	}
	return &agentWorkerHostHealthResponse{
		Success:    result.Success,
		Status:     result.Status,
		StatusCode: result.StatusCode,
		LatencyMS:  result.LatencyMS,
		Message:    result.Message,
		CheckedAt:  result.CheckedAt.Format(time.RFC3339),
		Response:   result.Response,
		Host:       agentWorkerHostToResponse(result.Host),
	}
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}
