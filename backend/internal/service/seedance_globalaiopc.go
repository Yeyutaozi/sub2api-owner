package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
)

const (
	VideoProviderGlobalAIOPC          = "globalaiopc"
	DefaultGlobalAIOPCVideoBaseURL    = "https://zcbservice.aizfw.cn/kyyReactApiServer"
	SeedanceGlobalAIOPCC1Model        = "seedance-2.5-c1-03"
	seedanceGlobalAIOPCUpstreamModel  = "seedance-2.5-c1"
	globalAIOPCDefaultDurationSeconds = 5
	globalAIOPCMaxDurationSeconds     = 30
	globalAIOPCVideoTaskPath          = "/v2/model-center/tasks"
)

type globalAIOPCVideoCreateRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	ReferenceImages []string `json:"reference_images,omitempty"`
	ReferenceVideos []string `json:"reference_videos,omitempty"`
	ReferenceAudios []string `json:"reference_audios,omitempty"`
	Duration        int      `json:"duration"`
	AspectRatio     string   `json:"aspect_ratio"`
	Resolution      string   `json:"resolution"`
	FirstImage      string   `json:"first_image,omitempty"`
	LastImage       string   `json:"last_image,omitempty"`
}

func isGlobalAIOPCVideoModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == SeedanceGlobalAIOPCC1Model || model == seedanceGlobalAIOPCUpstreamModel
}

func (a *Account) IsGlobalAIOPCVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderGlobalAIOPC
}

func buildGlobalAIOPCVideoCreateRequest(info *SeedanceRequestInfo) ([]byte, error) {
	if info == nil {
		return nil, errors.New("Seedance create request is required")
	}
	if mapped := strings.TrimSpace(info.Model); mapped != SeedanceGlobalAIOPCC1Model {
		return nil, fmt.Errorf("model %s is not supported by this video provider", mapped)
	}
	request := globalAIOPCVideoCreateRequest{
		Model: seedanceGlobalAIOPCUpstreamModel, Prompt: info.Prompt, Duration: info.DurationSeconds,
		AspectRatio: info.AspectRatio, Resolution: info.Resolution,
		FirstImage: strings.TrimSpace(info.StartFrameURL), LastImage: strings.TrimSpace(info.EndFrameURL),
	}
	for _, reference := range info.References {
		request.ReferenceImages = append(request.ReferenceImages, reference.URL)
	}
	for _, reference := range info.VideoReferences {
		request.ReferenceVideos = append(request.ReferenceVideos, reference.URL)
	}
	for _, reference := range info.AudioReferences {
		request.ReferenceAudios = append(request.ReferenceAudios, reference.URL)
	}
	if len(request.ReferenceAudios) > 0 && len(request.ReferenceImages) == 0 {
		return nil, errors.New("reference audio requires at least one reference image")
	}
	return json.Marshal(request)
}

func (s *OpenAIGatewayService) forwardGlobalAIOPCSeedance(ctx context.Context, c *gin.Context, account *Account, method, taskID string, info *SeedanceRequestInfo, contentRangeOverride *string) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsGlobalAIOPCVideo() {
		return nil, errors.New("video forwarding requires a compatible API key account")
	}
	apiKey := account.GetSeedanceAPIKey()
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	if contentRangeOverride != nil {
		return s.forwardGlobalAIOPCSeedanceContent(ctx, c, account, baseURL, apiKey, taskID, strings.TrimSpace(*contentRangeOverride))
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path := globalAIOPCVideoTaskPath
	var body []byte
	if method == http.MethodPost {
		body, err = buildGlobalAIOPCVideoCreateRequest(info)
		if err != nil {
			return nil, err
		}
	} else if method == http.MethodGet {
		if !seedanceTaskIDPattern.MatchString(strings.TrimSpace(taskID)) {
			return nil, errors.New("invalid Seedance upstream task id")
		}
		path += "/" + url.PathEscape(strings.TrimSpace(taskID))
	} else {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("unsupported method")}
	}
	response, err := s.doGlobalAIOPCRequest(ctx, c, account, method, buildXimeiEndpointURL(baseURL, path), path, apiKey, body, "")
	if err != nil {
		return nil, err
	}
	if method != http.MethodPost {
		return response, nil
	}
	upstreamTaskID := extractGlobalAIOPCTaskID(response.Body)
	if upstreamTaskID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: errors.New("video provider response did not include a task id")}
	}
	publicTaskID := globalAIOPCPublicTaskID(account.ID, upstreamTaskID)
	duration := time.Duration(0)
	if response.Result != nil {
		duration = response.Result.Duration
	}
	response.Result = &OpenAIForwardResult{
		RequestID: "seedance:" + publicTaskID, ResponseID: publicTaskID, UpstreamResponseID: upstreamTaskID,
		Model: info.Model, BillingModel: info.Model, UpstreamModel: seedanceGlobalAIOPCUpstreamModel,
		UpstreamEndpoint: path, ResponseHeaders: response.Header.Clone(), Duration: duration,
		VideoCount: 1, VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds,
	}
	return response, nil
}

func (s *OpenAIGatewayService) doGlobalAIOPCRequest(ctx context.Context, c *gin.Context, account *Account, method, targetURL, endpoint, apiKey string, body []byte, rangeHeader string) (*SeedanceUpstreamResponse, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	requestCtx := ctx
	isContent := endpoint == globalAIOPCVideoTaskPath+"/{task_id}/content"
	if isContent {
		requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, targetURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build video request: %w", err)
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	request.Header.Set("Accept", "application/json")
	if isContent {
		request.Header.Set("Accept", "*/*")
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	SetActualOpenAIUpstreamEndpoint(c, endpoint)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	resp, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("video upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
	}
	if resp.StatusCode >= http.StatusBadRequest && !(isContent && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		defer resp.Body.Close()
		responseBody := sanitizeHuiquSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: responseBody}
	}
	response := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: strings.TrimSpace(resp.Header.Get("Content-Type"))}
	if isContent {
		response.BodyStream = resp.Body
		return response, nil
	}
	defer resp.Body.Close()
	response.Body, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		response.Result = &OpenAIForwardResult{Duration: time.Since(startedAt)}
	}
	return response, nil
}

func (s *OpenAIGatewayService) forwardGlobalAIOPCSeedanceContent(ctx context.Context, c *gin.Context, account *Account, baseURL, apiKey, taskID, rangeHeader string) (*SeedanceUpstreamResponse, error) {
	if !seedanceTaskIDPattern.MatchString(strings.TrimSpace(taskID)) {
		return nil, errors.New("invalid Seedance upstream task id")
	}
	queryPath := globalAIOPCVideoTaskPath + "/" + url.PathEscape(strings.TrimSpace(taskID))
	query, err := s.doGlobalAIOPCRequest(ctx, c, account, http.MethodGet, buildXimeiEndpointURL(baseURL, queryPath), queryPath, apiKey, nil, "")
	if err != nil {
		return nil, err
	}
	status, videoURL, err := parseGlobalAIOPCTaskResult(query.Body)
	if err != nil {
		return nil, err
	}
	if MapSeedanceTaskStatus(status) != SeedanceTaskStatusSucceeded {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusConflict, Body: []byte("video task is not completed")}
	}
	validatedURL, err := urlvalidator.ValidateHTTPSURL(videoURL, urlvalidator.ValidationOptions{AllowPrivate: false})
	if err != nil {
		return nil, errors.New("video provider result URL is invalid")
	}
	parsedURL, err := url.Parse(validatedURL)
	if err != nil || parsedURL.User != nil || parsedURL.Fragment != "" || (parsedURL.Port() != "" && parsedURL.Port() != "443") {
		return nil, errors.New("video provider result URL is invalid")
	}
	if err := urlvalidator.ValidateResolvedIP(parsedURL.Hostname()); err != nil {
		return nil, errors.New("video provider result URL is invalid")
	}
	return s.doGlobalAIOPCRequest(ctx, c, account, http.MethodGet, validatedURL, globalAIOPCVideoTaskPath+"/{task_id}/content", "", nil, rangeHeader)
}

func extractGlobalAIOPCTaskID(body []byte) string {
	var payload struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &payload) != nil || !seedanceTaskIDPattern.MatchString(strings.TrimSpace(payload.ID)) {
		return ""
	}
	return strings.TrimSpace(payload.ID)
}

func globalAIOPCPublicTaskID(accountID int64, taskID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("globalaiopc:%d:%s", accountID, strings.TrimSpace(taskID))))
	return "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}

func parseGlobalAIOPCTaskResult(body []byte) (string, string, error) {
	var payload struct {
		Status    string `json:"status"`
		ResultURL string `json:"result_url"`
		VideoURL  string `json:"video_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", errors.New("invalid video provider task response")
	}
	status := strings.TrimSpace(payload.Status)
	videoURL := firstNonEmptyString(strings.TrimSpace(payload.VideoURL), strings.TrimSpace(payload.ResultURL))
	if status == "" {
		return "", "", errors.New("video provider task response is missing status")
	}
	if MapSeedanceTaskStatus(status) == SeedanceTaskStatusSucceeded && videoURL == "" {
		return "", "", errors.New("completed video provider task response is missing video_url")
	}
	return status, videoURL, nil
}
