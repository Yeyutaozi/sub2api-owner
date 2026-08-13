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
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// Admin-only credentials on a Weijin video account that enable silent
	// multi-modal mapping onto Pixelle sora-v3-pro for reference-video requests.
	// Legacy lingdong_* credential keys remain accepted as aliases.
	// End users never see these fields or the upstream vendor name.
	credentialLingdongMappingEnabled = "lingdong_mapping_enabled"
	credentialLingdongAPIKey         = "lingdong_api_key"
	credentialLingdongBaseURL        = "lingdong_base_url"
	credentialLingdongUpstreamModel  = "lingdong_upstream_model"
	credentialPixelleMappingEnabled  = "pixelle_mapping_enabled"
	credentialPixelleAPIKey          = "pixelle_api_key"
	credentialPixelleBaseURL         = "pixelle_base_url"
	credentialPixelleUpstreamModel   = "pixelle_upstream_model"

	DefaultLingdongVideoBaseURL     = "https://api.pixellelabs.com"
	DefaultLingdongUpstreamModel    = "sora-v3-pro"
	lingdongPublicTaskPrefix        = "pxv1_"
	lingdongLegacyPublicTaskPrefix  = "ldv1_"
	lingdongVideoCreatePath         = "/v1/videos"
	lingdongVideoTaskPath           = "/v1/videos"
	lingdongVideoContentPath        = "/v1/videos"
	lingdongMaxImageReferences      = 9
	lingdongMaxVideoReferences      = 3
	lingdongMaxAudioReferences      = 3
	lingdongMaxTotalReferences      = 12
	lingdongAudioRequiresImageMsg   = "使用音频参考时必须同时提供至少一张参考图"
	lingdongAudioComplianceMessage  = "音频涉嫌侵权，或音频格式/大小不合规，请移除参考音频后重试"
	lingdongVideoUnavailableMessage = "当前渠道暂不支持参考视频/音频，请移除后重试，或联系管理员开通扩展能力"
	lingdongMappingMisconfiguredMsg = "扩展参考视频/音频能力未正确配置，请联系管理员"
)

var lingdongPrivateNamePattern = regexp.MustCompile(`(?i)\b(?:lingdong|lingdongapi|cvk[\s_-]?s|pixelle|pixellelabs|sora[\s_-]?v3[\s_-]?pro|sora[\s_-]?v3)\b`)

func IsLingdongMappedSeedanceTaskID(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	return strings.HasPrefix(taskID, lingdongPublicTaskPrefix) || strings.HasPrefix(taskID, lingdongLegacyPublicTaskPrefix)
}

func publicLingdongMappedTaskID(upstreamTaskID string) (string, error) {
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
		return "", errors.New("invalid Seedance upstream task id")
	}
	publicID := lingdongPublicTaskPrefix + upstreamTaskID
	if !seedanceTaskIDPattern.MatchString(publicID) {
		return "", errors.New("Seedance upstream task id is too long")
	}
	return publicID, nil
}

func upstreamLingdongMappedTaskID(publicTaskID string) (string, error) {
	publicTaskID = strings.TrimSpace(publicTaskID)
	if !IsLingdongMappedSeedanceTaskID(publicTaskID) {
		return "", errors.New("Seedance task does not belong to the mapped video provider")
	}
	upstream := publicTaskID
	switch {
	case strings.HasPrefix(publicTaskID, lingdongPublicTaskPrefix):
		upstream = strings.TrimPrefix(publicTaskID, lingdongPublicTaskPrefix)
	case strings.HasPrefix(publicTaskID, lingdongLegacyPublicTaskPrefix):
		upstream = strings.TrimPrefix(publicTaskID, lingdongLegacyPublicTaskPrefix)
	}
	if !seedanceTaskIDPattern.MatchString(upstream) {
		return "", errors.New("invalid Seedance upstream task id")
	}
	return upstream, nil
}

func credentialTruthy(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	case json.Number:
		parsed, err := strconv.ParseBool(value.String())
		return err == nil && parsed
	case float64:
		return value != 0
	case int:
		return value != 0
	case int64:
		return value != 0
	default:
		return false
	}
}

func (a *Account) IsLingdongMappingEnabled() bool {
	if a == nil || !a.IsWeijinVideo() || a.Credentials == nil {
		return false
	}
	for _, key := range []string{credentialPixelleMappingEnabled, credentialLingdongMappingEnabled} {
		raw, ok := a.Credentials[key]
		if !ok || raw == nil {
			continue
		}
		if credentialTruthy(raw) {
			return true
		}
	}
	return false
}

func (a *Account) GetLingdongAPIKey() string {
	if a == nil || !a.IsWeijinVideo() {
		return ""
	}
	if key := strings.TrimSpace(a.GetCredential(credentialPixelleAPIKey)); key != "" {
		return key
	}
	return strings.TrimSpace(a.GetCredential(credentialLingdongAPIKey))
}

func (a *Account) GetLingdongBaseURL() string {
	if a == nil || !a.IsWeijinVideo() {
		return ""
	}
	if baseURL := strings.TrimSpace(a.GetCredential(credentialPixelleBaseURL)); baseURL != "" {
		return baseURL
	}
	if baseURL := strings.TrimSpace(a.GetCredential(credentialLingdongBaseURL)); baseURL != "" {
		return baseURL
	}
	return DefaultLingdongVideoBaseURL
}

func (a *Account) GetLingdongUpstreamModel() string {
	if a == nil || !a.IsWeijinVideo() {
		return DefaultLingdongUpstreamModel
	}
	if model := strings.TrimSpace(a.GetCredential(credentialPixelleUpstreamModel)); model != "" {
		return model
	}
	if model := strings.TrimSpace(a.GetCredential(credentialLingdongUpstreamModel)); model != "" {
		return model
	}
	return DefaultLingdongUpstreamModel
}

// IsLingdongMappingReady reports whether the admin enabled mapping and supplied
// a usable API key. Base URL falls back to the default when omitted.
func (a *Account) IsLingdongMappingReady() bool {
	return a != nil && a.IsLingdongMappingEnabled() && a.GetLingdongAPIKey() != ""
}

func SeedanceRequestHasVideoReferences(info *SeedanceRequestInfo) bool {
	return seedanceRequestHasVideoReferences(info)
}

func seedanceRequestHasVideoReferences(info *SeedanceRequestInfo) bool {
	if info == nil {
		return false
	}
	for _, media := range info.VideoReferences {
		if strings.TrimSpace(media.URL) != "" {
			return true
		}
	}
	return false
}

func SeedanceRequestHasAudioReferences(info *SeedanceRequestInfo) bool {
	return seedanceRequestHasAudioReferences(info)
}

func seedanceRequestHasAudioReferences(info *SeedanceRequestInfo) bool {
	if info == nil {
		return false
	}
	for _, media := range info.AudioReferences {
		if strings.TrimSpace(media.URL) != "" {
			return true
		}
	}
	return false
}

func seedanceAudioComplianceUpstreamError() *SeedanceUpstreamError {
	return &SeedanceUpstreamError{
		StatusCode: http.StatusBadRequest,
		Body: []byte(`{"error":{"code":"invalid_request","message":"` +
			lingdongAudioComplianceMessage +
			`"}}`),
	}
}

func seedanceVideoReferenceUnavailableUpstreamError(message string) *SeedanceUpstreamError {
	if strings.TrimSpace(message) == "" {
		message = lingdongVideoUnavailableMessage
	}
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    "invalid_request",
			"message": message,
		},
	})
	return &SeedanceUpstreamError{StatusCode: http.StatusBadRequest, Body: payload}
}

// decideWeijinSeedanceRoute chooses pure Weijin (images/prompt) vs Pixelle-mapped
// multi-modal (reference videos and/or audio) or a client-facing rejection.
// Account stays video_provider=weijin. Pixelle replaces the former Lingdong slot.
func decideWeijinSeedanceRoute(account *Account, info *SeedanceRequestInfo) (route string, err error) {
	if info == nil {
		return "weijin", nil
	}
	hasVideo := seedanceRequestHasVideoReferences(info)
	hasAudio := seedanceRequestHasAudioReferences(info)
	if !hasVideo && !hasAudio {
		return "weijin", nil
	}
	if account == nil || !account.IsLingdongMappingReady() {
		if account != nil && account.IsLingdongMappingEnabled() && account.GetLingdongAPIKey() == "" {
			return "", seedanceVideoReferenceUnavailableUpstreamError(lingdongMappingMisconfiguredMsg)
		}
		return "", seedanceVideoReferenceUnavailableUpstreamError(lingdongVideoUnavailableMessage)
	}
	// Pixelle requires at least one image when audio references are present.
	if hasAudio && len(weijinImageURLs(info)) == 0 {
		return "", seedanceVideoReferenceUnavailableUpstreamError(lingdongAudioRequiresImageMsg)
	}
	return "pixelle", nil
}

func lingdongResolutionForPublicModel(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceWeijinFaceRef480pModel:
		return VideoBillingResolution480P
	default:
		return VideoBillingResolution720P
	}
}

func pixelleSecondsForDuration(duration int) string {
	// Pixelle sora-v3-pro accepts 10 or 15 seconds only.
	if duration <= 10 {
		return "10"
	}
	return "15"
}

func buildLingdongVideoCreateRequest(info *SeedanceRequestInfo, upstreamModel string) ([]byte, error) {
	if info == nil {
		return nil, errors.New("Seedance create request is required")
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		upstreamModel = DefaultLingdongUpstreamModel
	}
	if !isWeijinFaceReferenceDurationSupported(info.DurationSeconds) {
		return nil, fmt.Errorf("duration %d is not supported", info.DurationSeconds)
	}
	prompt := composeWeijinFaceRefPrompt(info.Prompt)
	images := weijinImageURLs(info)
	if len(images) > lingdongMaxImageReferences {
		images = images[:lingdongMaxImageReferences]
	}
	videos := make([]string, 0, lingdongMaxVideoReferences)
	for _, media := range info.VideoReferences {
		urlValue := strings.TrimSpace(media.URL)
		if urlValue == "" {
			continue
		}
		videos = append(videos, urlValue)
		if len(videos) >= lingdongMaxVideoReferences {
			break
		}
	}
	audios := make([]string, 0, lingdongMaxAudioReferences)
	for _, media := range info.AudioReferences {
		urlValue := strings.TrimSpace(media.URL)
		if urlValue == "" {
			continue
		}
		audios = append(audios, urlValue)
		if len(audios) >= lingdongMaxAudioReferences {
			break
		}
	}
	// Hard cap: images + videos + audios <= 12. Drop audios first, then videos.
	remaining := lingdongMaxTotalReferences - len(images)
	if remaining < 0 {
		remaining = 0
	}
	if len(videos) > remaining {
		videos = videos[:remaining]
	}
	remaining -= len(videos)
	if remaining < 0 {
		remaining = 0
	}
	if len(audios) > remaining {
		audios = audios[:remaining]
	}
	if strings.TrimSpace(info.Prompt) == "" && len(images) == 0 && len(videos) == 0 && len(audios) == 0 {
		return nil, errors.New("prompt is required when no reference media is provided")
	}
	if len(audios) > 0 && len(images) == 0 {
		return nil, errors.New(lingdongAudioRequiresImageMsg)
	}

	publicModel := strings.ToLower(strings.TrimSpace(info.Model))
	resolution := lingdongResolutionForPublicModel(publicModel)
	if explicit := NormalizeVideoBillingResolutionOrDefault(info.Resolution); explicit != "" {
		// Keep the public model resolution as the source of truth; only honor
		// request resolution when it matches the model tier.
		if publicModel == SeedanceWeijinFaceRef480pModel || publicModel == SeedanceWeijinFaceRef720pModel {
			resolution = lingdongResolutionForPublicModel(publicModel)
		} else {
			resolution = explicit
		}
	}
	// Pixelle public docs currently advertise 720p for sora-v3-pro.
	if resolution != VideoBillingResolution720P && resolution != VideoBillingResolution480P {
		resolution = VideoBillingResolution720P
	}
	// Prefer 720p for mapped path even when public model is 480p tier if upstream
	// only lists 720p; still pass model-tier resolution when it is 480p/720p.
	ratio := strings.TrimSpace(info.AspectRatio)
	if ratio == "" {
		ratio = "16:9"
	}

	body := map[string]any{
		"model":        upstreamModel,
		"prompt":       prompt,
		"aspect_ratio": ratio,
		"resolution":   resolution,
		"seconds":      pixelleSecondsForDuration(info.DurationSeconds),
	}
	if len(images) == 1 {
		body["image_url"] = images[0]
	} else if len(images) > 1 {
		body["image_url"] = images[0]
		body["reference_image_urls"] = images[1:]
	}
	if len(videos) == 1 {
		body["reference_video"] = videos[0]
	} else if len(videos) > 1 {
		body["reference_videos"] = videos
	}
	if len(audios) == 1 {
		body["audio_url"] = audios[0]
	} else if len(audios) > 1 {
		// Pixelle rejects mixing audio_url + audio_urls ("aliases must contain identical items").
		// Prefer plural form only for multi-audio requests.
		body["audio_urls"] = audios
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode mapped video request: %w", err)
	}
	return encoded, nil
}

func (s *OpenAIGatewayService) forwardLingdongMappedSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsWeijinVideo() {
		return nil, errors.New("mapped video forwarding requires a Weijin special-offer account")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == http.MethodDelete {
		return nil, &SeedanceUpstreamError{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       []byte(`{"error":{"code":"not_supported","message":"This video provider does not support task cancellation"}}`),
		}
	}
	apiKey := account.GetLingdongAPIKey()
	if apiKey == "" {
		return nil, seedanceVideoReferenceUnavailableUpstreamError(lingdongMappingMisconfiguredMsg)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetLingdongBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid mapped base_url: %w", err)
	}

	if contentRangeOverride != nil || (c != nil && c.Request != nil && strings.HasSuffix(c.Request.URL.Path, "/content")) {
		rangeHeader := ""
		if contentRangeOverride != nil {
			rangeHeader = strings.TrimSpace(*contentRangeOverride)
		} else if c != nil {
			rangeHeader = strings.TrimSpace(c.GetHeader("Range"))
		}
		upstreamTaskID, err := upstreamLingdongMappedTaskID(taskID)
		if err != nil {
			// Content paths always receive the public (prefixed) task id.
			return nil, err
		}
		path := lingdongVideoContentPath + "/" + url.PathEscape(upstreamTaskID) + "/content"
		return s.doLingdongMappedSeedanceRequest(ctx, c, account, http.MethodGet, buildOpenAIEndpointURL(baseURL, path), path, apiKey, nil, rangeHeader)
	}

	path := lingdongVideoCreatePath
	var requestBody []byte
	requestModel := ""
	upstreamModel := account.GetLingdongUpstreamModel()
	if method == http.MethodPost {
		if requestInfo == nil {
			return nil, errors.New("Seedance create request is required")
		}
		requestModel = requestInfo.Model
		// Validate public model constraints via the same weijin model checker.
		if _, err := weijinUpstreamModelFor(requestInfo, normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestModel))); err != nil {
			return nil, err
		}
		requestBody, err = buildLingdongVideoCreateRequest(requestInfo, upstreamModel)
		if err != nil {
			return nil, err
		}
	} else {
		upstreamTaskID, err := upstreamLingdongMappedTaskID(taskID)
		if err != nil {
			return nil, err
		}
		path = lingdongVideoTaskPath + "/" + url.PathEscape(upstreamTaskID)
	}

	response, err := s.doLingdongMappedSeedanceRequest(ctx, c, account, method, buildOpenAIEndpointURL(baseURL, path), path, apiKey, requestBody, "")
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
	publicTaskID, err := publicLingdongMappedTaskID(upstreamTaskID)
	if err != nil {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: err}
	}
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
	// Rewrite create body ids so clients bind the opaque public task id.
	if rewritten, rewriteErr := rewriteSeedanceCreateTaskIDs(response.Body, publicTaskID); rewriteErr == nil && len(rewritten) > 0 {
		response.Body = rewritten
	}
	return response, nil
}

func rewriteSeedanceCreateTaskIDs(body []byte, publicTaskID string) ([]byte, error) {
	publicTaskID = strings.TrimSpace(publicTaskID)
	if publicTaskID == "" || len(body) == 0 {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, err
	}
	payload["id"] = publicTaskID
	payload["job_id"] = publicTaskID
	payload["task_id"] = publicTaskID
	return json.Marshal(payload)
}

func (s *OpenAIGatewayService) doLingdongMappedSeedanceRequest(
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
		return nil, fmt.Errorf("build mapped video request: %w", err)
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
		responseBody := sanitizeLingdongSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
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

func sanitizeLingdongSeedanceUpstreamErrorBody(body []byte) []byte {
	sanitized := sanitizeWeijinSeedanceUpstreamErrorBody(body)
	return lingdongPrivateNamePattern.ReplaceAll(sanitized, []byte("upstream provider"))
}
