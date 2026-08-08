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
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
)

const (
	VideoProviderXimei       = "ximei"
	DefaultXimeiVideoBaseURL = "https://liantongyidong.ximeiedu.org"

	SeedanceXimeiSD20Model = "sd-2.0-mx933"
	SeedanceXimeiSD25Model = "sd-2.5-mx"
	// Unofficial / non-official Ximei channel for Seedance 2.5 style generation.
	SeedanceXimeiSD25UnofficialModel = "sd-2.5-mx-2000"

	seedanceXimeiSD25DefaultDurationSeconds = 5
	seedanceXimeiSD25MaxDurationSeconds     = 30

	ximeiVideoCreatePath = "/api/v3/contents/generations/tasks"

	ximeiPlatformIdempotencyKeyPrefix = "sub2api-ximei-"
)

var ximeiVideoResultAllowedHosts = []string{
	"liantongyidong.ximeiedu.org",
	"tdown1.ximeiedu.org",
	"tdown2.ximeiedu.org",
}

var validateXimeiResultResolvedIP = urlvalidator.ValidateResolvedIP

type ximeiDurationMode string

const (
	ximeiDurationParameter ximeiDurationMode = "parameter"
	ximeiDurationPrompt    ximeiDurationMode = "prompt"
)

type ximeiVideoProduct struct {
	Route                 string
	Resolution            string
	DurationMode          ximeiDurationMode
	MaxAudioSeconds       float64
	MaxVideoSeconds       float64
	MaxImages             int
	MaxVideos             int
	MaxAudios             int
	RequireMediaDurations bool
}

var ximeiPrivateNamePattern = regexp.MustCompile(`(?i)\b(?:ximei|canseedream|liantongyidong|ximeiedu|kele_pool|tc_pool|fenda_pool|nangua_pool|lajiao_pool)\b`)

type ximeiTimedMedia struct {
	URL             string  `json:"url"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type ximeiVideoCreateRequest struct {
	Model         string            `json:"model"`
	ProviderRoute string            `json:"provider_route"`
	Prompt        string            `json:"prompt"`
	Duration      string            `json:"duration"`
	ImageURLs     []string          `json:"image_urls,omitempty"`
	VideoURLs     []ximeiTimedMedia `json:"video_urls,omitempty"`
	AudioURLs     []ximeiTimedMedia `json:"audio_urls,omitempty"`
	AspectRatio   string            `json:"aspect_ratio"`
	GenerateAudio bool              `json:"generate_audio"`
	NumberOfRuns  int               `json:"number_of_runs"`
}

func isXimeiVideoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceXimeiSD20Model, SeedanceXimeiSD25Model, SeedanceXimeiSD25UnofficialModel:
		return true
	default:
		return false
	}
}

func IsXimeiVideoModel(model string) bool {
	return isXimeiVideoModel(model)
}

func isSeedanceMixedImageModel(model string) bool {
	// 所有 seedance 系模型均允许首尾帧与参考图同时使用。
	profile, ok := ffLinkVideoModelProfileFor(model)
	if ok && profile.Platform == PlatformSeedance {
		return true
	}
	return isHuiquVideoModel(model) || isXimeiVideoModel(model)
}

func IsOpaqueSeedanceVideoProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case VideoProviderHuiqu, VideoProviderXimei:
		return true
	default:
		return false
	}
}

func (a *Account) IsXimeiVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderXimei
}

func ximeiVideoProductFor(model, resolution string) (ximeiVideoProduct, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	resolution = NormalizeVideoBillingResolutionOrDefault(resolution)
	switch model {
	case SeedanceXimeiSD20Model:
		switch resolution {
		case VideoBillingResolution480P:
			return ximeiVideoProduct{
				Route: "kele_pool", Resolution: resolution, DurationMode: ximeiDurationParameter,
				MaxAudioSeconds: 15, MaxVideoSeconds: 15, RequireMediaDurations: true,
			}, nil
		case VideoBillingResolution720P:
			return ximeiVideoProduct{
				Route: "tc_pool", Resolution: resolution, DurationMode: ximeiDurationPrompt,
				MaxAudioSeconds: 15, MaxVideoSeconds: 15, RequireMediaDurations: true,
			}, nil
		}
	case SeedanceXimeiSD25Model:
		if resolution == VideoBillingResolution720P {
			return ximeiVideoProduct{
				Route: "nangua_pool", Resolution: resolution, DurationMode: ximeiDurationParameter,
				// Official Seedance 2.5 channel on Ximei (nangua_pool).
				// Supports up to 30 image / 10 video / 10 audio references (50 total), 30s media.
				MaxImages: 30, MaxVideos: 10, MaxAudios: 10,
				MaxAudioSeconds: 30, MaxVideoSeconds: 30,
				RequireMediaDurations: true,
			}, nil
		}
	case SeedanceXimeiSD25UnofficialModel:
		if resolution == VideoBillingResolution720P {
			return ximeiVideoProduct{
				Route: "lajiao_pool", Resolution: resolution, DurationMode: ximeiDurationParameter,
				// Unofficial Seedance 2.5 channel on Ximei (lajiao_pool / chili full).
				// Upstream health: maxImages=30, maxVideos=10, maxAudio=10, maxAssets=50,
				// maxAudioSeconds=30, maxVideoSeconds=30; platform exposes duration 5/10/15/30.
				MaxImages: 30, MaxVideos: 10, MaxAudios: 10,
				MaxAudioSeconds: 30, MaxVideoSeconds: 30,
				RequireMediaDurations: true,
			}, nil
		}
	}
	return ximeiVideoProduct{}, fmt.Errorf("model %s does not support resolution %s on this video provider", model, resolution)
}

func buildXimeiVideoCreateRequest(info *SeedanceRequestInfo) ([]byte, string, error) {
	if info == nil {
		return nil, "", errors.New("Seedance create request is required")
	}
	product, err := ximeiVideoProductFor(info.Model, info.Resolution)
	if err != nil {
		return nil, "", err
	}
	if !isXimeiVideoDurationSupported(info.Model, info.DurationSeconds) {
		return nil, "", fmt.Errorf("duration %d is not supported by model %s", info.DurationSeconds, info.Model)
	}
	if err := validateXimeiReferenceDurations(info, product); err != nil {
		return nil, "", err
	}

	prompt := compileXimeiPrompt(info)
	profile, _ := ffLinkVideoModelProfileFor(info.Model)
	if profile.PromptLimit > 0 && len([]rune(prompt)) > profile.PromptLimit {
		return nil, "", fmt.Errorf("compiled prompt exceeds the %d character limit for model %s", profile.PromptLimit, info.Model)
	}
	duration := fmt.Sprintf("%d", info.DurationSeconds)
	if product.DurationMode == ximeiDurationPrompt {
		duration = "auto"
	}
	request := ximeiVideoCreateRequest{
		Model: "video", ProviderRoute: product.Route, Prompt: prompt, Duration: duration,
		AspectRatio: info.AspectRatio, GenerateAudio: info.GenerateAudio, NumberOfRuns: 1,
	}
	request.ImageURLs = ximeiImageURLs(info)
	for _, media := range info.VideoReferences {
		request.VideoURLs = append(request.VideoURLs, ximeiTimedMedia{URL: media.URL, DurationSeconds: media.DurationSeconds})
	}
	for _, media := range info.AudioReferences {
		request.AudioURLs = append(request.AudioURLs, ximeiTimedMedia{URL: media.URL, DurationSeconds: media.DurationSeconds})
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encode Ximei video request: %w", err)
	}
	return body, product.Route, nil
}

func isXimeiVideoDurationSupported(model string, duration int) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceXimeiSD25Model, SeedanceXimeiSD25UnofficialModel:
		return isSeedanceDurationSupported(duration) || duration == seedanceXimeiSD25MaxDurationSeconds
	default:
		return isSeedanceDurationSupported(duration)
	}
}

func ximeiImageURLs(info *SeedanceRequestInfo) []string {
	// 与提示词 @ImageN 同一顺序：参考图 → 首帧 → 尾帧。
	// 首尾帧追加在数组末尾，提示词按实际下标动态注入（如 1 张参考图时首帧=@Image2）。
	slots := ximeiOrderedImageSlots(info)
	if len(slots) == 0 {
		return nil
	}
	images := make([]string, 0, len(slots))
	for _, slot := range slots {
		images = append(images, slot.URL)
	}
	return images
}

func validateXimeiReferenceDurations(info *SeedanceRequestInfo, product ximeiVideoProduct) error {
	if product.MaxImages > 0 {
		imageCount := len(info.References)
		if strings.TrimSpace(info.StartFrameURL) != "" {
			imageCount++
		}
		if strings.TrimSpace(info.EndFrameURL) != "" {
			imageCount++
		}
		if imageCount > product.MaxImages {
			return fmt.Errorf("model %s supports at most %d images including reference images and first/last frames", info.Model, product.MaxImages)
		}
	}
	if product.MaxVideos > 0 && len(info.VideoReferences) > product.MaxVideos {
		return fmt.Errorf("model %s supports at most %d reference videos", info.Model, product.MaxVideos)
	}
	if product.MaxAudios > 0 && len(info.AudioReferences) > product.MaxAudios {
		return fmt.Errorf("model %s supports at most %d reference audio files", info.Model, product.MaxAudios)
	}

	var totalVideo float64
	for index, media := range info.VideoReferences {
		if product.RequireMediaDurations && media.DurationSeconds <= 0 {
			return fmt.Errorf("guidances.video_reference_base[%d].video.duration_seconds is required for model %s", index, info.Model)
		}
		totalVideo += media.DurationSeconds
	}
	var totalAudio float64
	for index, media := range info.AudioReferences {
		if product.RequireMediaDurations && media.DurationSeconds <= 0 {
			return fmt.Errorf("guidances.audio_reference[%d].audio.duration_seconds is required for model %s", index, info.Model)
		}
		totalAudio += media.DurationSeconds
	}
	if product.MaxVideoSeconds > 0 && totalVideo > product.MaxVideoSeconds+0.001 {
		return fmt.Errorf("reference video duration must not exceed %.0f seconds in total for model %s", product.MaxVideoSeconds, info.Model)
	}
	if product.MaxAudioSeconds > 0 && totalAudio > product.MaxAudioSeconds+0.001 {
		return fmt.Errorf("reference audio duration must not exceed %.0f seconds in total for model %s", product.MaxAudioSeconds, info.Model)
	}
	return nil
}

func compileXimeiPrompt(info *SeedanceRequestInfo) string {
	if info == nil {
		return ""
	}
	// 按 image_urls 实际序号动态注入；用户占用编号时不写 @ImageN 映射。
	base := composeSeedancePromptWithMediaHints(info)
	// ximei 额外要求严格时长约束
	durationHint := fmt.Sprintf("- 生成视频的总时长必须严格为 %d 秒，动作在规定时长内完整结束。", info.DurationSeconds)
	if strings.Contains(base, "[平台参考约束，请严格执行]") {
		return strings.Replace(base, "[平台参考约束，请严格执行]\n", "[平台参考约束，请严格执行]\n"+durationHint+"\n", 1)
	}
	prompt := strings.TrimSpace(base)
	if prompt == "" {
		return "[平台参考约束，请严格执行]\n" + durationHint
	}
	return prompt + "\n\n[平台参考约束，请严格执行]\n" + durationHint
}

func (s *OpenAIGatewayService) forwardXimeiSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsXimeiVideo() {
		return nil, errors.New("Ximei video forwarding requires a compatible API key account")
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == http.MethodDelete {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("this video provider does not support task cancellation")}
	}
	if contentRangeOverride != nil {
		return s.forwardXimeiSeedanceContent(ctx, c, account, baseURL, apiKey, taskID, strings.TrimSpace(*contentRangeOverride))
	}

	path := ximeiVideoCreatePath
	var requestBody []byte
	providerRoute := ""
	requestModel := ""
	if method == http.MethodPost {
		if requestInfo == nil {
			return nil, errors.New("Seedance create request is required")
		}
		requestModel = requestInfo.Model
		requestBody, providerRoute, err = buildXimeiVideoCreateRequest(requestInfo)
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

	response, err := s.doXimeiSeedanceRequest(ctx, c, account, method, buildXimeiEndpointURL(baseURL, path), path, apiKey, requestBody, "")
	if err != nil {
		if method == http.MethodPost {
			var upstreamErr *SeedanceUpstreamError
			var failoverErr *UpstreamFailoverError
			if !errors.As(err, &upstreamErr) && !errors.As(err, &failoverErr) {
				return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: err}
			}
		}
		return nil, err
	}
	if method != http.MethodPost {
		return response, nil
	}
	upstreamTaskID := extractXimeiTaskID(response.Body)
	if upstreamTaskID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: errors.New("video provider response did not include a task id")}
	}
	publicTaskID := ximeiPublicTaskID(account.ID, upstreamTaskID)
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
		UpstreamModel:        providerRoute,
		UpstreamEndpoint:     path,
		ResponseHeaders:      response.Header.Clone(),
		Duration:             duration,
		VideoCount:           1,
		VideoResolution:      requestInfo.Resolution,
		VideoDurationSeconds: requestInfo.DurationSeconds,
	}
	return response, nil
}

func (s *OpenAIGatewayService) doXimeiSeedanceRequest(
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
		return nil, fmt.Errorf("build Ximei video request: %w", err)
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
		request.Header.Set("Idempotency-Key", ximeiPlatformIdempotencyKey(ctx, body))
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
		return nil, fmt.Errorf("Ximei video upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if isContentResponse && resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		_ = resp.Body.Close()
		return nil, &SeedanceUpstreamError{
			StatusCode: http.StatusBadGateway,
			Body:       []byte("video provider result redirect is not allowed"),
		}
	}
	if resp.StatusCode >= http.StatusBadRequest && !(isContentResponse && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		defer func() { _ = resp.Body.Close() }()
		responseBody := sanitizeXimeiSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(responseBody))
		if method == http.MethodPost && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, message, responseBody) {
			return nil, &UpstreamFailoverError{
				StatusCode: resp.StatusCode, ResponseBody: responseBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: responseBody}
	}
	response := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: contentType}
	if isContentResponse || rangeHeader != "" || strings.HasPrefix(strings.ToLower(contentType), "video/") || resp.StatusCode == http.StatusPartialContent {
		response.BodyStream = resp.Body
		return response, nil
	}
	defer func() { _ = resp.Body.Close() }()
	response.Body, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		response.Result = &OpenAIForwardResult{Duration: time.Since(startedAt)}
	}
	return response, nil
}

func (s *OpenAIGatewayService) forwardXimeiSeedanceContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL, apiKey, upstreamTaskID, rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
		return nil, errors.New("invalid Seedance upstream task id")
	}
	queryPath := ximeiVideoCreatePath + "/" + url.PathEscape(upstreamTaskID)
	query, err := s.doXimeiSeedanceRequest(ctx, c, account, http.MethodGet, buildXimeiEndpointURL(baseURL, queryPath), queryPath, apiKey, nil, "")
	if err != nil {
		return nil, err
	}
	status, videoURL, err := parseXimeiTaskResult(query.Body)
	if err != nil {
		return nil, err
	}
	if MapSeedanceTaskStatus(status) != SeedanceTaskStatusSucceeded {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusConflict, Body: []byte("video task is not completed")}
	}
	validatedURL, err := validateXimeiVideoResultURL(videoURL)
	if err != nil {
		return nil, errors.New("video provider result URL is invalid")
	}
	return s.doXimeiSeedanceRequest(ctx, c, account, http.MethodGet, validatedURL, ximeiVideoCreatePath+"/{task_id}/content", "", nil, rangeHeader)
}

func ximeiPlatformIdempotencyKey(ctx context.Context, body []byte) string {
	scope := seedanceIdempotencyKeyFromContext(ctx)
	if scope != "" {
		digest := sha256.Sum256([]byte("ximei-video-create:task:v1\n" + scope))
		return ximeiPlatformIdempotencyKeyPrefix + base64.RawURLEncoding.EncodeToString(digest[:24])
	}
	if scope == "" && ctx != nil {
		scope, _ = ctx.Value(ctxkey.RequestID).(string)
		scope = strings.TrimSpace(scope)
	}
	if scope == "" && ctx != nil {
		scope, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		scope = strings.TrimSpace(scope)
	}

	bodyDigest := sha256.Sum256(body)
	material := "ximei-video-create:v1\n" + scope + "\n" + base64.RawURLEncoding.EncodeToString(bodyDigest[:])
	digest := sha256.Sum256([]byte(material))
	return ximeiPlatformIdempotencyKeyPrefix + base64.RawURLEncoding.EncodeToString(digest[:24])
}

func validateXimeiVideoResultURL(raw string) (string, error) {
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     ximeiVideoResultAllowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     false,
	})
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", errors.New("invalid video result URL")
	}
	if parsed.User != nil {
		return "", errors.New("video result URL must not contain user info")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", errors.New("video result URL must use the standard HTTPS port")
	}
	if parsed.Fragment != "" {
		return "", errors.New("video result URL must not contain a fragment")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if err := validateXimeiResultResolvedIP(host); err != nil {
		return "", fmt.Errorf("video result host resolution is unsafe: %w", err)
	}
	return normalized, nil
}

func buildXimeiEndpointURL(baseURL, endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + strings.TrimLeft(endpoint, "/")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	endpointPath := "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(basePath, "/api/v3") && strings.HasPrefix(endpointPath, "/api/v3/") {
		endpointPath = strings.TrimPrefix(endpointPath, "/api/v3")
	}
	if !strings.HasSuffix(basePath, endpointPath) {
		basePath += endpointPath
	}
	parsed.Path = basePath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func extractXimeiTaskID(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return findXimeiTaskID(payload, 0)
}

func findXimeiTaskID(payload map[string]any, depth int) string {
	if depth > 2 || payload == nil {
		return ""
	}
	for _, key := range []string{"task_id", "id", "job_id"} {
		if value, ok := payload[key].(string); ok {
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "cstask_") && seedanceTaskIDPattern.MatchString(value) {
				return value
			}
		}
	}
	for _, key := range []string{"task", "data"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if taskID := findXimeiTaskID(nested, depth+1); taskID != "" {
				return taskID
			}
		}
	}
	return ""
}

func ximeiPublicTaskID(accountID int64, upstreamTaskID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("ximei:%d:%s", accountID, strings.TrimSpace(upstreamTaskID))))
	return "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}

func parseXimeiTaskResult(body []byte) (status string, videoURL string, err error) {
	var payload struct {
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
	}
	if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil {
		return "", "", errors.New("invalid video provider task response")
	}
	status = strings.TrimSpace(payload.Status)
	if status == "" {
		return "", "", errors.New("video provider task response is missing status")
	}
	videoURL = strings.TrimSpace(payload.Content.VideoURL)
	if MapSeedanceTaskStatus(status) == SeedanceTaskStatusSucceeded && videoURL == "" {
		return "", "", errors.New("completed video provider task response is missing content.video_url")
	}
	return status, videoURL, nil
}

func sanitizeXimeiSeedanceUpstreamErrorBody(body []byte) []byte {
	sanitized := sanitizeHuiquSeedanceUpstreamErrorBody(body)
	return ximeiPrivateNamePattern.ReplaceAll(sanitized, []byte("upstream provider"))
}
