package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// VideoProviderWeijin is a dedicated Seedance special-offer channel.
	// Public API stays platform-owned; this value is admin/account-only.
	VideoProviderWeijin       = "weijin"
	DefaultWeijinVideoBaseURL = "https://www.weijinapi.top"

	// Public model IDs for the face-reference special-offer group.
	// Resolution is fixed by model id; clients still use the unified public schema.
	SeedanceWeijinFaceRef480pModel = "seedance2.0-one-face-reference-480p"
	SeedanceWeijinFaceRef720pModel = "seedance2.0-one-face-reference-720p"

	weijinVideoCreatePath = "/v1/videos"
	weijinVideoTaskPath   = "/v1/videos"
)

var weijinPrivateNamePattern = regexp.MustCompile(`(?i)\b(?:weijin|weijinapi|one[\s_-]?api|oneapi)\b`)

func isWeijinVideoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceWeijinFaceRef480pModel, SeedanceWeijinFaceRef720pModel:
		return true
	default:
		return false
	}
}

func IsWeijinVideoModel(model string) bool {
	return isWeijinVideoModel(model)
}

func isWeijinFaceReferenceDurationSupported(duration int) bool {
	return duration >= 4 && duration <= 15
}

func (a *Account) IsWeijinVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderWeijin
}

func weijinUpstreamModelFor(info *SeedanceRequestInfo, mappedModel string) (string, error) {
	if info == nil {
		return "", errors.New("Seedance create request is required")
	}
	model := strings.ToLower(strings.TrimSpace(mappedModel))
	if model == "" {
		model = strings.ToLower(strings.TrimSpace(info.Model))
	}
	if !isWeijinVideoModel(model) {
		return "", fmt.Errorf("unsupported Weijin video model: %s", model)
	}
	if !isWeijinFaceReferenceDurationSupported(info.DurationSeconds) {
		return "", fmt.Errorf("duration %d is not supported by model %s", info.DurationSeconds, model)
	}
	expectedResolution := VideoBillingResolution720P
	if model == SeedanceWeijinFaceRef480pModel {
		expectedResolution = VideoBillingResolution480P
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(info.Resolution)
	if resolution != expectedResolution {
		return "", fmt.Errorf("model %s only supports resolution %s", model, expectedResolution)
	}
	return model, nil
}

func buildWeijinVideoCreateRequest(info *SeedanceRequestInfo, upstreamModel string) ([]byte, error) {
	if info == nil {
		return nil, errors.New("Seedance create request is required")
	}
	upstreamModel = strings.ToLower(strings.TrimSpace(upstreamModel))
	if !isWeijinVideoModel(upstreamModel) {
		return nil, fmt.Errorf("unsupported Weijin video model: %s", upstreamModel)
	}
	if !isWeijinFaceReferenceDurationSupported(info.DurationSeconds) {
		return nil, fmt.Errorf("duration %d is not supported by model %s", info.DurationSeconds, upstreamModel)
	}
	prompt := strings.TrimSpace(info.Prompt)
	images := weijinImageURLs(info)
	videos := make([]string, 0, len(info.VideoReferences))
	for _, media := range info.VideoReferences {
		urlValue := strings.TrimSpace(media.URL)
		if urlValue == "" {
			continue
		}
		videos = append(videos, urlValue)
	}
	audios := make([]string, 0, len(info.AudioReferences))
	for _, media := range info.AudioReferences {
		urlValue := strings.TrimSpace(media.URL)
		if urlValue == "" {
			continue
		}
		audios = append(audios, urlValue)
	}
	if prompt == "" && len(images) == 0 && len(videos) == 0 && len(audios) == 0 {
		return nil, errors.New("prompt is required when no reference media is provided")
	}

	body := map[string]any{
		"model":        upstreamModel,
		"seconds":      info.DurationSeconds,
		"aspect_ratio": info.AspectRatio,
	}
	if prompt != "" {
		body["prompt"] = prompt
	}
	// Mirror public audio=true when requested or when reference audio is provided.
	if info.GenerateAudio || len(audios) > 0 {
		body["audio"] = true
	}
	if len(images) > 0 {
		body["images"] = images
	}
	if len(videos) > 0 {
		body["videos"] = videos
	}
	if len(audios) > 0 {
		body["audios"] = audios
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode Weijin video request: %w", err)
	}
	return encoded, nil
}

func weijinImageURLs(info *SeedanceRequestInfo) []string {
	slots := ximeiOrderedImageSlots(info)
	if len(slots) == 0 {
		return nil
	}
	images := make([]string, 0, len(slots))
	for _, slot := range slots {
		if urlValue := strings.TrimSpace(slot.URL); urlValue != "" {
			images = append(images, urlValue)
		}
	}
	return images
}

func (s *OpenAIGatewayService) forwardWeijinSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsWeijinVideo() {
		return nil, errors.New("Weijin video forwarding requires a compatible API key account")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == http.MethodDelete {
		return nil, &SeedanceUpstreamError{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       []byte(`{"error":{"code":"not_supported","message":"This video provider does not support task cancellation"}}`),
		}
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	if contentRangeOverride != nil || (c != nil && c.Request != nil && strings.HasSuffix(c.Request.URL.Path, "/content")) {
		rangeHeader := ""
		if contentRangeOverride != nil {
			rangeHeader = strings.TrimSpace(*contentRangeOverride)
		} else if c != nil {
			rangeHeader = strings.TrimSpace(c.GetHeader("Range"))
		}
		return s.forwardWeijinSeedanceContent(ctx, c, account, baseURL, apiKey, taskID, rangeHeader)
	}

	path := weijinVideoCreatePath
	var requestBody []byte
	requestModel := ""
	upstreamModel := ""
	if method == http.MethodPost {
		if requestInfo == nil {
			return nil, errors.New("Seedance create request is required")
		}
		requestModel = requestInfo.Model
		mappedModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestModel))
		upstreamModel, err = weijinUpstreamModelFor(requestInfo, mappedModel)
		if err != nil {
			return nil, err
		}
		requestBody, err = buildWeijinVideoCreateRequest(requestInfo, upstreamModel)
		if err != nil {
			return nil, err
		}
	} else {
		upstreamTaskID := strings.TrimSpace(taskID)
		if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
			return nil, errors.New("invalid Seedance upstream task id")
		}
		path = weijinVideoTaskPath + "/" + url.PathEscape(upstreamTaskID)
	}

	response, err := s.doWeijinSeedanceRequest(ctx, c, account, method, buildOpenAIEndpointURL(baseURL, path), path, apiKey, requestBody, "")
	if err != nil {
		return nil, err
	}
	if method != http.MethodPost {
		return response, nil
	}

	upstreamTaskID := extractSeedanceUpstreamTaskID(response.Body)
	if upstreamTaskID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{
			Err: errors.New("Seedance upstream response did not include task id"),
		}
	}
	publicTaskID := upstreamTaskID
	duration := time.Duration(0)
	if response.Result != nil {
		duration = response.Result.Duration
	}
	response.Result = &OpenAIForwardResult{
		RequestID:            firstNonEmptyString(response.Header.Get("x-request-id"), response.Header.Get("request-id"), "seedance:"+publicTaskID),
		ResponseID:           publicTaskID,
		UpstreamResponseID:   upstreamTaskID,
		Model:                requestModel,
		BillingModel:         requestModel,
		UpstreamModel:        upstreamModel,
		UpstreamEndpoint:     path,
		ResponseHeaders:      response.Header.Clone(),
		Duration:             duration,
		VideoCount:           1,
		VideoResolution:      requestInfo.Resolution,
		VideoDurationSeconds: requestInfo.DurationSeconds,
	}
	return response, nil
}

func (s *OpenAIGatewayService) forwardWeijinSeedanceContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL, apiKey, upstreamTaskID, rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
		return nil, errors.New("invalid Seedance upstream task id")
	}
	path := weijinVideoTaskPath + "/" + url.PathEscape(upstreamTaskID) + "/content"
	return s.doWeijinSeedanceRequest(ctx, c, account, http.MethodGet, buildOpenAIEndpointURL(baseURL, path), path, apiKey, nil, rangeHeader)
}

func (s *OpenAIGatewayService) doWeijinSeedanceRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method, targetURL, endpoint, apiKey string,
	body []byte,
	rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	isContentResponse := strings.HasSuffix(endpoint, "/content")
	requestCtx := ctx
	if isContentResponse {
		requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, targetURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build Weijin video request: %w", err)
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if isContentResponse {
		request.Header.Set("Accept", "*/*")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	if c != nil && method == http.MethodPost {
		if idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key")); idempotencyKey != "" {
			request.Header.Set("Idempotency-Key", idempotencyKey)
		}
	}
	if method == http.MethodPost && request.Header.Get("Idempotency-Key") == "" {
		if idempotencyKey := seedanceIdempotencyKeyFromContext(ctx); idempotencyKey != "" {
			request.Header.Set("Idempotency-Key", idempotencyKey)
		}
	}
	SetActualOpenAIUpstreamEndpoint(c, endpoint)

	proxyURL := ""
	if account != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	accountID := int64(0)
	concurrency := 0
	if account != nil {
		accountID = account.ID
		concurrency = account.Concurrency
	}
	resp, err := s.httpUpstream.Do(request, proxyURL, accountID, concurrency)
	if err != nil {
		forwardErr := fmt.Errorf("video upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
		if method == http.MethodPost {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: forwardErr}
		}
		return nil, forwardErr
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if isContentResponse && resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		_ = resp.Body.Close()
		return nil, &SeedanceUpstreamError{
			StatusCode: http.StatusBadGateway,
			Body:       []byte(`{"error":{"code":"invalid_upstream_response","message":"Video result redirect is not allowed"}}`),
		}
	}
	if resp.StatusCode >= http.StatusBadRequest && !(isContentResponse && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		defer func() { _ = resp.Body.Close() }()
		responseBody := sanitizeWeijinSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(responseBody))
		if method == http.MethodPost && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, message, responseBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           responseBody,
				RetryableOnSameAccount: account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: responseBody}
	}

	response := &SeedanceUpstreamResponse{
		StatusCode:  resp.StatusCode,
		Header:      resp.Header.Clone(),
		ContentType: contentType,
	}
	if isContentResponse || rangeHeader != "" || strings.HasPrefix(strings.ToLower(contentType), "video/") || resp.StatusCode == http.StatusPartialContent {
		response.BodyStream = resp.Body
		return response, nil
	}
	defer func() { _ = resp.Body.Close() }()
	response.Body, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if method == http.MethodPost {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: err}
		}
		return nil, err
	}
	if method == http.MethodPost {
		response.Result = &OpenAIForwardResult{Duration: time.Since(startedAt)}
	}
	return response, nil
}

func sanitizeWeijinSeedanceUpstreamErrorBody(body []byte) []byte {
	sanitized := sanitizeHuiquSeedanceUpstreamErrorBody(body)
	return weijinPrivateNamePattern.ReplaceAll(sanitized, []byte("upstream provider"))
}