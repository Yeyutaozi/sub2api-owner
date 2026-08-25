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

	// SeedanceWeijin900Model is the platform-owned public model ID. The
	// provider's private ID must only appear in account model_mapping values.
	SeedanceWeijin900Model         = "sd-2.0-900-720p"
	SeedanceWeijin900UpstreamModel = "seedance2.0-900-3"
	seedanceWeijin900RetiredModel  = "sd-2.0-900"

	weijinVideoCreatePath = "/v1/videos"
	weijinVideoTaskPath   = "/v1/videos"
)

var (
	weijinPrivateNamePattern     = regexp.MustCompile(`(?i)\b(?:weijin|weijinapi|xmanway|one[\s_-]?api|oneapi)\b`)
	weijin900PrivateModelPattern = regexp.MustCompile(`(?i)\bseedance2\.0-900-3\b`)
)

func isWeijinVideoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceWeijinFaceRef480pModel,
		SeedanceWeijinFaceRef720pModel,
		SeedanceWeijin900Model,
		SeedanceWeijin900UpstreamModel:
		return true
	default:
		return false
	}
}

func isWeijin900PublicModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), SeedanceWeijin900Model)
}

func isWeijin900UpstreamModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), SeedanceWeijin900UpstreamModel)
}

func isRetiredWeijin900PublicModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), seedanceWeijin900RetiredModel)
}

func IsWeijinVideoModel(model string) bool {
	return isWeijinVideoModel(model)
}

func isWeijinFaceReferenceDurationSupported(duration int) bool {
	return duration >= 4 && duration <= 15
}

func isWeijin900DurationSupported(duration int) bool {
	return duration >= 5 && duration <= 15
}

func (a *Account) IsWeijinVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderWeijin
}

func weijinUpstreamModelFor(info *SeedanceRequestInfo, mappedModel string) (string, error) {
	if info == nil {
		return "", errors.New("Seedance create request is required")
	}
	publicModel := strings.ToLower(strings.TrimSpace(info.Model))
	model := strings.ToLower(strings.TrimSpace(mappedModel))
	if isWeijin900PublicModel(publicModel) {
		if err := validateFFLinkVideoRequestInfo(info); err != nil {
			return "", err
		}
		if !isWeijin900UpstreamModel(model) {
			return "", fmt.Errorf("model %s requires an explicit account mapping to its supported upstream model", SeedanceWeijin900Model)
		}
		return SeedanceWeijin900UpstreamModel, nil
	}
	if isWeijin900UpstreamModel(model) {
		return "", fmt.Errorf("upstream model %s may only be mapped from %s", SeedanceWeijin900UpstreamModel, SeedanceWeijin900Model)
	}
	if model == "" {
		model = publicModel
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

// Default quality constraints injected for public one-face-reference models.
// Applied only on the Weijin special-offer path and its Pixelle multi-modal mapping.
const (
	seedanceFaceRefQualityHintMarker   = "【画质强制约束】"
	seedanceFaceRefDefaultQualityHints = `【画质强制约束】
- 全程锐利清晰的真人电影画质：禁止远景虚化、背景发糊、浅景深 bohek 虚化主体外背景。
- 中景/全景/建立镜头必须深景深：建筑轮廓、街道、屋檐、辇车与人物前后景均清晰可辨。
- 禁止柔焦、雾化滤镜、镜头脏污感、过度磨皮、塑料油面感与发灰发虚。
- 对话段落优先中近景与特写；远景仅可短建立且必须全清晰。
- 光影自然写实，8K 级细节，真实质感；禁止生成字幕、水印、台标、花字。`
)

// composeWeijinFaceRefPrompt injects default sharpness / deep-DOF constraints for
// one-face-reference models without duplicating user-provided constraints.
func composeWeijinFaceRefPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	hints := strings.TrimSpace(seedanceFaceRefDefaultQualityHints)
	if prompt == "" {
		return hints
	}
	if strings.Contains(prompt, seedanceFaceRefQualityHintMarker) {
		return prompt
	}
	// User already wrote the same intent in free text.
	if strings.Contains(prompt, "禁止远景虚化") ||
		strings.Contains(prompt, "深景深") ||
		strings.Contains(strings.ToLower(prompt), "deep depth of field") ||
		strings.Contains(strings.ToLower(prompt), "no background blur") {
		return prompt
	}
	return prompt + "\n\n" + hints
}

const (
	// Public one-face protocol caps; Weijin 480p native video refs reuse the same limits.
	weijinMaxImageReferences = 9
	weijinMaxVideoReferences = 3
)

func buildWeijinVideoCreateRequest(info *SeedanceRequestInfo, upstreamModel string) ([]byte, error) {
	if info == nil {
		return nil, errors.New("Seedance create request is required")
	}
	upstreamModel = strings.ToLower(strings.TrimSpace(upstreamModel))
	if !isWeijinVideoModel(upstreamModel) {
		return nil, fmt.Errorf("unsupported Weijin video model: %s", upstreamModel)
	}
	if isWeijin900PublicModel(upstreamModel) {
		return nil, fmt.Errorf("model %s requires an explicit account mapping to its supported upstream model", SeedanceWeijin900Model)
	}
	is900 := isWeijin900UpstreamModel(upstreamModel)
	if is900 {
		if !isWeijin900PublicModel(info.Model) {
			return nil, fmt.Errorf("upstream model %s may only be mapped from %s", SeedanceWeijin900UpstreamModel, SeedanceWeijin900Model)
		}
		if err := validateFFLinkVideoRequestInfo(info); err != nil {
			return nil, err
		}
	}
	if is900 && !isWeijin900DurationSupported(info.DurationSeconds) {
		return nil, fmt.Errorf("duration %d is not supported by model %s", info.DurationSeconds, SeedanceWeijin900Model)
	}
	if !is900 && !isWeijinFaceReferenceDurationSupported(info.DurationSeconds) {
		return nil, fmt.Errorf("duration %d is not supported by model %s", info.DurationSeconds, upstreamModel)
	}
	prompt := strings.TrimSpace(info.Prompt)
	if !is900 {
		prompt = composeWeijinFaceRefPrompt(info.Prompt)
	}
	images := weijinImageURLs(info)
	videos := weijinVideoURLs(info)
	// 720p Weijin path is images + prompt only; reference videos/audio for 720p are
	// either mapped to Pixelle or rejected by decideWeijinSeedanceRoute.
	// 480p Weijin natively accepts reference videos (no audio) — include them below.
	if strings.TrimSpace(info.Prompt) == "" && len(images) == 0 && len(videos) == 0 {
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
	if len(images) > 0 {
		body["images"] = images
	}
	// Only the 480p face-ref model natively supports reference videos on Weijin.
	// Never send audios on the pure Weijin path.
	if upstreamModel == SeedanceWeijinFaceRef480pModel && len(videos) > 0 {
		body["videos"] = videos
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
			if len(images) >= weijinMaxImageReferences {
				break
			}
		}
	}
	return images
}

func weijinVideoURLs(info *SeedanceRequestInfo) []string {
	if info == nil || len(info.VideoReferences) == 0 {
		return nil
	}
	videos := make([]string, 0, len(info.VideoReferences))
	for _, media := range info.VideoReferences {
		if urlValue := strings.TrimSpace(media.URL); urlValue != "" {
			videos = append(videos, urlValue)
			if len(videos) >= weijinMaxVideoReferences {
				break
			}
		}
	}
	return videos
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

	// Lingdong-mapped tasks are identified by the opaque public id prefix so
	// poll/content stick to the same backend without changing video_provider.
	if IsLingdongMappedSeedanceTaskID(taskID) {
		return s.forwardLingdongMappedSeedance(ctx, c, account, method, taskID, requestInfo, contentRangeOverride)
	}
	if method == http.MethodPost {
		route, routeErr := decideWeijinSeedanceRoute(account, requestInfo)
		if routeErr != nil {
			return nil, routeErr
		}
		if route == "pixelle" || route == "lingdong" {
			return s.forwardLingdongMappedSeedance(ctx, c, account, method, taskID, requestInfo, contentRangeOverride)
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
		if method == http.MethodPost && shouldFallbackWeijin720pCreateToMapped(account, requestInfo, err) {
			return s.forwardLingdongMappedSeedance(ctx, c, account, method, taskID, requestInfo, contentRangeOverride)
		}
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

func shouldFallbackWeijin720pCreateToMapped(account *Account, info *SeedanceRequestInfo, err error) bool {
	if account == nil || info == nil || err == nil || !account.IsLingdongMappingReady() {
		return false
	}
	if strings.ToLower(strings.TrimSpace(info.Model)) != SeedanceWeijinFaceRef720pModel ||
		seedanceRequestHasVideoReferences(info) || seedanceRequestHasAudioReferences(info) {
		return false
	}
	if _, ok := account.ResolveLingdongMappedUpstreamModel(info.Model); !ok {
		return false
	}

	statusCode := 0
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		statusCode = failoverErr.StatusCode
	} else {
		var upstreamErr *SeedanceUpstreamError
		if errors.As(err, &upstreamErr) {
			statusCode = upstreamErr.StatusCode
		}
	}
	return statusCode >= http.StatusInternalServerError && statusCode <= 599
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
	targetURL := buildOpenAIEndpointURL(baseURL, path)
	response, err := s.doWeijinSeedanceRequest(ctx, c, account, http.MethodGet, targetURL, path, apiKey, nil, rangeHeader)
	if err != nil || response == nil || response.BodyStream == nil {
		return response, err
	}
	// Weijin may terminate a large HTTP/1.1 response before Content-Length.
	// Keep the initial status/headers visible to the caller while transparently
	// resuming the body from the exact byte offset on a subsequent Range request.
	response.BodyStream = newWeijinContentResumeReader(
		s, ctx, c, account, targetURL, path, apiKey, rangeHeader, response,
	)
	return response, nil
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
	var contentCancel context.CancelFunc
	if isContentResponse {
		requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
		requestCtx, contentCancel = context.WithTimeout(requestCtx, weijinContentSegmentTimeout)
		defer func() {
			if contentCancel != nil {
				contentCancel()
			}
		}()
	}
	request, err := http.NewRequestWithContext(requestCtx, method, targetURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build Weijin video request: %w", err)
	}
	// Weijin's video object endpoint intermittently resets HTTP/2 streams while
	// a large MP4 is being read. Keep create/status calls on the OpenAI profile,
	// but use the default transport profile for content so it negotiates
	// HTTP/1.1 and the client can receive the object without an H2 INTERNAL_ERROR.
	profile := HTTPUpstreamProfileOpenAI
	if isContentResponse {
		profile = HTTPUpstreamProfileDefault
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), profile))
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
		if contentCancel != nil {
			response.BodyStream = &weijinContentTimedBody{body: resp.Body, cancel: contentCancel}
			contentCancel = nil
		}
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
	sanitized = weijin900PrivateModelPattern.ReplaceAll(sanitized, []byte(SeedanceWeijin900Model))
	sanitized = weijinPrivateNamePattern.ReplaceAll(sanitized, []byte("upstream provider"))
	return lingdongPrivateNamePattern.ReplaceAll(sanitized, []byte("upstream provider"))
}
