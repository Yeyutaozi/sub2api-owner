package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type AgentWorkerHostService struct {
	repo       AgentWorkerHostRepository
	httpClient *http.Client
}

func NewAgentWorkerHostService(repo AgentWorkerHostRepository) *AgentWorkerHostService {
	return &AgentWorkerHostService{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type CreateAgentWorkerHostInput struct {
	Name           string
	BaseURL        string
	Protocol       string
	AuthType       string
	SecretRef      string
	HealthPath     string
	RunPath        string
	CancelPath     string
	MaxConcurrency int
	TimeoutSeconds int
	Status         string
	Metadata       map[string]any
}

type UpdateAgentWorkerHostInput struct {
	Name           string
	BaseURL        string
	Protocol       string
	AuthType       string
	SecretRef      string
	HealthPath     string
	RunPath        string
	CancelPath     string
	MaxConcurrency int
	TimeoutSeconds int
	Status         string
	Metadata       map[string]any
}

func (s *AgentWorkerHostService) Create(ctx context.Context, input CreateAgentWorkerHostInput) (*AgentWorkerHost, error) {
	host, err := buildWorkerHostFromInput(input, nil)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, host); err != nil {
		return nil, fmt.Errorf("create agent worker host: %w", err)
	}
	return host, nil
}

func (s *AgentWorkerHostService) GetByID(ctx context.Context, id int64) (*AgentWorkerHost, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AgentWorkerHostService) List(ctx context.Context, params pagination.PaginationParams, filters AgentWorkerHostListFilters) ([]AgentWorkerHost, *pagination.PaginationResult, error) {
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 100 {
		filters.Search = filters.Search[:100]
	}
	filters.Status = strings.TrimSpace(filters.Status)
	return s.repo.List(ctx, params, filters)
}

func (s *AgentWorkerHostService) ListAll(ctx context.Context, status string) ([]AgentWorkerHost, error) {
	return s.repo.ListAll(ctx, strings.TrimSpace(status))
}

func (s *AgentWorkerHostService) Update(ctx context.Context, id int64, input UpdateAgentWorkerHostInput) (*AgentWorkerHost, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	updated, err := buildWorkerHostFromInput(CreateAgentWorkerHostInput{
		Name:           agentFirstNonEmpty(input.Name, current.Name),
		BaseURL:        agentFirstNonEmpty(input.BaseURL, current.BaseURL),
		Protocol:       agentFirstNonEmpty(input.Protocol, current.Protocol),
		AuthType:       agentFirstNonEmpty(input.AuthType, current.AuthType),
		SecretRef:      input.SecretRef,
		HealthPath:     agentFirstNonEmpty(input.HealthPath, current.HealthPath),
		RunPath:        agentFirstNonEmpty(input.RunPath, current.RunPath),
		CancelPath:     input.CancelPath,
		MaxConcurrency: agentFirstNonZero(input.MaxConcurrency, current.MaxConcurrency),
		TimeoutSeconds: agentFirstNonZero(input.TimeoutSeconds, current.TimeoutSeconds),
		Status:         agentFirstNonEmpty(input.Status, current.Status),
		Metadata:       input.Metadata,
	}, current)
	if err != nil {
		return nil, err
	}
	updated.ID = id
	if err := s.repo.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("update agent worker host: %w", err)
	}
	return updated, nil
}

func (s *AgentWorkerHostService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete agent worker host: %w", err)
	}
	return nil
}

func (s *AgentWorkerHostService) HealthCheck(ctx context.Context, id int64) (*AgentWorkerHostHealthResult, error) {
	host, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	healthURL, err := joinWorkerURL(host.BaseURL, host.HealthPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_WORKER_HEALTH_REQUEST_INVALID", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Sub2API-Worker-Protocol", host.Protocol)

	started := time.Now()
	resp, err := s.httpClient.Do(req)
	latencyMS := int(time.Since(started).Milliseconds())
	checkedAt := time.Now().UTC()

	result := &AgentWorkerHostHealthResult{
		Success:   false,
		Status:    AgentWorkerHostHealthUnhealthy,
		LatencyMS: latencyMS,
		CheckedAt: checkedAt,
		Host:      host,
	}
	if err != nil {
		result.Message = err.Error()
		return s.persistHealthResult(ctx, host, result)
	}
	defer func() { _ = resp.Body.Close() }()

	result.StatusCode = resp.StatusCode
	var workerResp WorkerHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&workerResp); err == nil {
		result.Response = &workerResp
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		result.Status = AgentWorkerHostHealthHealthy
		if result.Response != nil && strings.TrimSpace(result.Response.Message) != "" {
			result.Message = strings.TrimSpace(result.Response.Message)
		} else {
			result.Message = "Worker Host 健康检查通过"
		}
		return s.persistHealthResult(ctx, host, result)
	}

	result.Message = fmt.Sprintf("Worker Host 健康检查失败，HTTP %d", resp.StatusCode)
	if result.Response != nil && strings.TrimSpace(result.Response.Message) != "" {
		result.Message = result.Response.Message
	}
	return s.persistHealthResult(ctx, host, result)
}

func (s *AgentWorkerHostService) ValidateRunRoute(ctx context.Context, id int64, healthPath, runPath string) (*WorkerHealthResponse, error) {
	host, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if host.Status == AgentWorkerHostStatusDisabled {
		return nil, infraerrors.BadRequest("AGENT_WORKER_HOST_DISABLED", "Worker Host 已禁用")
	}
	healthPath, err = normalizeWorkerPath(healthPath, host.HealthPath, "Worker 健康检查路径")
	if err != nil {
		return nil, err
	}
	runPath, err = normalizeWorkerPath(runPath, host.RunPath, "Worker 运行路径")
	if err != nil {
		return nil, err
	}
	healthURL, err := joinWorkerURL(host.BaseURL, healthPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PREFLIGHT_REQUEST_INVALID", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Sub2API-Worker-Protocol", host.Protocol)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PREFLIGHT_UNREACHABLE", "无法连接 Worker，请检查服务地址和防火墙："+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PREFLIGHT_UNHEALTHY", fmt.Sprintf("Worker 健康检查失败，HTTP %d", resp.StatusCode))
	}
	var workerResp WorkerHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&workerResp); err != nil {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PREFLIGHT_INVALID_RESPONSE", "Worker 健康检查响应不是有效 JSON")
	}
	if protocol := strings.TrimSpace(workerResp.Protocol); protocol != "" && protocol != host.Protocol {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PROTOCOL_MISMATCH", fmt.Sprintf("Worker 协议不匹配：期望 %s，实际 %s", host.Protocol, protocol))
	}
	routes := workerRunRoutes(workerResp.Routes)
	if len(routes) == 0 {
		return nil, infraerrors.BadRequest("AGENT_WORKER_ROUTES_MISSING", "Worker 未声明可用运行路径，请升级 Worker 后再发布")
	}
	for _, route := range routes {
		if route == runPath {
			return &workerResp, nil
		}
	}
	return nil, infraerrors.BadRequest("AGENT_WORKER_ROUTE_NOT_FOUND", fmt.Sprintf("Worker 未声明运行路径 %s，可用路径：%s", runPath, strings.Join(routes, "、")))
}

func workerRunRoutes(routes map[string]any) []string {
	if len(routes) == 0 {
		return nil
	}
	value, ok := routes["runs"]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	appendRoute := func(raw string) {
		route, err := normalizeWorkerPath(raw, "", "Worker 运行路径")
		if err == nil && route != "" && !seen[route] {
			seen[route] = true
			out = append(out, route)
		}
	}
	switch typed := value.(type) {
	case string:
		appendRoute(typed)
	case []string:
		for _, route := range typed {
			appendRoute(route)
		}
	case []any:
		for _, route := range typed {
			if text, ok := route.(string); ok {
				appendRoute(text)
			}
		}
	}
	return out
}

func (s *AgentWorkerHostService) persistHealthResult(ctx context.Context, host *AgentWorkerHost, result *AgentWorkerHostHealthResult) (*AgentWorkerHostHealthResult, error) {
	nextStatus := host.Status
	if host.Status != AgentWorkerHostStatusDisabled {
		if result.Success {
			nextStatus = AgentWorkerHostStatusActive
		} else {
			nextStatus = AgentWorkerHostStatusUnhealthy
		}
	}
	latency := result.LatencyMS
	if err := s.repo.UpdateHealth(ctx, host.ID, nextStatus, result.Status, result.Message, &latency, result.CheckedAt); err != nil {
		return nil, fmt.Errorf("update worker host health: %w", err)
	}
	refreshed, err := s.repo.GetByID(ctx, host.ID)
	if err != nil {
		return nil, err
	}
	result.Host = refreshed
	return result, nil
}

func buildWorkerHostFromInput(input CreateAgentWorkerHostInput, current *AgentWorkerHost) (*AgentWorkerHost, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("AGENT_WORKER_HOST_NAME_REQUIRED", "Worker Host 名称不能为空")
	}

	baseURL, err := normalizeWorkerBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}

	protocol := strings.TrimSpace(input.Protocol)
	if protocol == "" {
		protocol = AgentWorkerProtocolV1
	}
	if protocol != AgentWorkerProtocolV1 {
		return nil, infraerrors.BadRequest("AGENT_WORKER_PROTOCOL_INVALID", "暂只支持 sub2api-worker-v1 协议")
	}

	authType := strings.TrimSpace(input.AuthType)
	if authType == "" {
		authType = AgentWorkerAuthHMACRunToken
	}
	switch authType {
	case AgentWorkerAuthNone, AgentWorkerAuthHMACRunToken, AgentWorkerAuthBearer:
	default:
		return nil, infraerrors.BadRequest("AGENT_WORKER_AUTH_TYPE_INVALID", "Worker Host 鉴权方式无效")
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AgentWorkerHostStatusActive
	}
	switch status {
	case AgentWorkerHostStatusActive, AgentWorkerHostStatusDisabled, AgentWorkerHostStatusUnhealthy:
	default:
		return nil, infraerrors.BadRequest("AGENT_WORKER_HOST_STATUS_INVALID", "Worker Host 状态无效")
	}

	healthPath, err := normalizeWorkerPath(input.HealthPath, "/health", "健康检查路径")
	if err != nil {
		return nil, err
	}
	runPath, err := normalizeWorkerPath(input.RunPath, "/runs", "运行路径")
	if err != nil {
		return nil, err
	}
	cancelPath := strings.TrimSpace(input.CancelPath)
	if cancelPath != "" {
		cancelPath, err = normalizeWorkerPath(cancelPath, "", "取消路径")
		if err != nil {
			return nil, err
		}
	}

	maxConcurrency := input.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 600
	}

	metadata := input.Metadata
	if metadata == nil && current != nil {
		metadata = current.Metadata
	}
	if metadata == nil {
		metadata = map[string]any{}
	}

	host := &AgentWorkerHost{
		Name:           name,
		BaseURL:        baseURL,
		Protocol:       protocol,
		AuthType:       authType,
		SecretRef:      strings.TrimSpace(input.SecretRef),
		HealthPath:     healthPath,
		RunPath:        runPath,
		CancelPath:     cancelPath,
		MaxConcurrency: maxConcurrency,
		TimeoutSeconds: timeoutSeconds,
		Status:         status,
		Metadata:       metadata,
	}
	if current != nil {
		host.LastHealthStatus = current.LastHealthStatus
		host.LastHealthMessage = current.LastHealthMessage
		host.LastHealthLatencyMS = current.LastHealthLatencyMS
		host.LastCheckedAt = current.LastCheckedAt
		host.CreatedAt = current.CreatedAt
	}
	return host, nil
}

func normalizeWorkerBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", infraerrors.BadRequest("AGENT_WORKER_BASE_URL_REQUIRED", "Worker Host Base URL 不能为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", infraerrors.BadRequest("AGENT_WORKER_BASE_URL_INVALID", "Worker Host Base URL 必须是完整的 HTTP/HTTPS 地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("AGENT_WORKER_BASE_URL_SCHEME_INVALID", "Worker Host Base URL 仅支持 http 或 https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizeWorkerPath(raw, fallback, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "/") {
		return "", infraerrors.BadRequest("AGENT_WORKER_PATH_INVALID", label+"必须以 / 开头")
	}
	return raw, nil
}

func joinWorkerURL(base, path string) (string, error) {
	baseURL, err := normalizeWorkerBaseURL(base)
	if err != nil {
		return "", err
	}
	workerPath, err := normalizeWorkerPath(path, "", "Worker 路径")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/") + workerPath, nil
}

func agentFirstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func agentFirstNonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
