package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

const (
	SeedanceOfficialTasksEndpoint   = "/api/v3/contents/generations/tasks"
	SeedanceOfficialUploadsEndpoint = "/api/v3/contents/generations/uploads"
	DefaultSeedanceBaseURL          = "https://api.fflink.top"
	seedanceUpstreamCreatePath      = "/v1/videos/generations"
	seedanceUpstreamJobsPath        = "/v1/videos/jobs"
	seedanceTaskBindingTTL          = 7 * 24 * time.Hour
)

var seedanceTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

var seedanceSensitiveQueryParamPattern = regexp.MustCompile(`(?i)((?:[?&]|\\u0026)(?:key|client_secret|access_token|refresh_token|token|sig|signature|credential|policy|ossaccesskeyid|x-amz-[a-z0-9-]+|x-goog-[a-z0-9-]+|q-[a-z0-9-]+|x-oss-[a-z0-9-]+)=)[^&"'\s\\},]+`)

func ValidateSeedanceAccountConfiguration(platform, accountType string, credentials map[string]any) error {
	if platform != PlatformSeedance {
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return infraerrors.BadRequest("SEEDANCE_ACCOUNT_TYPE_INVALID", "Seedance accounts must use the apikey account type")
	}
	apiKey, _ := credentials["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		return infraerrors.BadRequest("SEEDANCE_API_KEY_REQUIRED", "Seedance accounts require an upstream API key")
	}
	return nil
}

// SeedanceCreateRequest is the Volcengine Ark-compatible video task request.
type SeedanceCreateRequest struct {
	Model         string                `json:"model"`
	Content       []SeedanceContentItem `json:"content"`
	Ratio         string                `json:"ratio,omitempty"`
	Duration      int                   `json:"duration,omitempty"`
	Resolution    string                `json:"resolution,omitempty"`
	GenerateAudio *bool                 `json:"generate_audio,omitempty"`
	Watermark     *bool                 `json:"watermark,omitempty"`
	Seed          *int64                `json:"seed,omitempty"`
	CameraFixed   *bool                 `json:"camera_fixed,omitempty"`
}

type SeedanceContentItem struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL json.RawMessage `json:"image_url,omitempty"`
	Role     string          `json:"role,omitempty"`
	Strength string          `json:"strength,omitempty"`
}

type seedanceImageInput struct {
	URL       string `json:"url"`
	Base64    string `json:"base64,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Role      string `json:"role,omitempty"`
	Strength  string `json:"strength,omitempty"`
}

type SeedanceRequestInfo struct {
	Model           string
	Prompt          string
	Resolution      string
	DurationSeconds int
	AspectRatio     string
	GenerateAudio   bool
	StartFrameURL   string
	EndFrameURL     string
	References      []SeedanceReferenceImage
}

type SeedanceReferenceImage struct {
	URL      string
	Strength string
}

func (i *SeedanceRequestInfo) HasInlineImages() bool {
	if i == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.StartFrameURL)), "data:") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.EndFrameURL)), "data:") {
		return true
	}
	for _, reference := range i.References {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(reference.URL)), "data:") {
			return true
		}
	}
	return false
}

type SeedanceUpstreamResponse struct {
	StatusCode  int
	Header      http.Header
	Body        []byte
	BodyStream  io.ReadCloser
	ContentType string
	Streamed    bool
	Result      *OpenAIForwardResult
}

type SeedanceUpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *SeedanceUpstreamError) Error() string {
	if e == nil {
		return "seedance upstream request failed"
	}
	return fmt.Sprintf("seedance upstream returned status %d", e.StatusCode)
}

func ParseSeedanceCreateRequest(body []byte) (*SeedanceRequestInfo, error) {
	if len(body) == 0 {
		return nil, errors.New("request body is empty")
	}
	var request SeedanceCreateRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must contain exactly one JSON object")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("model is required")
	}
	if len(request.Content) == 0 {
		return nil, errors.New("content is required")
	}
	if request.Seed != nil {
		return nil, errors.New("seed is not supported by the configured Seedance upstream")
	}
	if request.CameraFixed != nil && *request.CameraFixed {
		return nil, errors.New("camera_fixed is not supported by the configured Seedance upstream")
	}
	if request.Watermark != nil && *request.Watermark {
		return nil, errors.New("watermark=true is not supported by the configured Seedance upstream")
	}

	info := &SeedanceRequestInfo{
		Model:           strings.TrimSpace(request.Model),
		Resolution:      strings.ToLower(strings.TrimSpace(request.Resolution)),
		DurationSeconds: request.Duration,
		AspectRatio:     strings.TrimSpace(request.Ratio),
	}
	if request.GenerateAudio != nil {
		info.GenerateAudio = *request.GenerateAudio
	}
	if info.Resolution == "" {
		info.Resolution = VideoBillingResolution720P
	}
	switch info.Resolution {
	case VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P:
	default:
		return nil, errors.New("resolution must be one of 480p, 720p, or 1080p")
	}
	if info.DurationSeconds == 0 {
		info.DurationSeconds = VideoBillingDefaultDurationSeconds
	}
	if info.DurationSeconds < 4 || info.DurationSeconds > VideoBillingMaxDurationSeconds {
		return nil, errors.New("duration must be between 4 and 15 seconds")
	}
	if err := validateSeedanceAspectRatio(info.AspectRatio); err != nil {
		return nil, err
	}

	var unroledImageSeen bool
	for _, item := range request.Content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "text":
			text := strings.TrimSpace(item.Text)
			if text != "" {
				if info.Prompt != "" {
					info.Prompt += "\n"
				}
				info.Prompt += text
			}
		case "image_url":
			imageInput, err := parseSeedanceImageInput(item)
			if err != nil {
				return nil, err
			}
			switch normalizeSeedanceImageRole(imageInput.Role) {
			case "first_frame":
				if info.StartFrameURL != "" {
					return nil, errors.New("only one first-frame image is allowed")
				}
				info.StartFrameURL = imageInput.URL
			case "last_frame":
				if info.EndFrameURL != "" {
					return nil, errors.New("only one last-frame image is allowed")
				}
				info.EndFrameURL = imageInput.URL
			case "reference_image":
				info.References = append(info.References, SeedanceReferenceImage{URL: imageInput.URL, Strength: normalizeSeedanceStrength(imageInput.Strength)})
			default:
				if unroledImageSeen || info.StartFrameURL != "" {
					return nil, errors.New("multiple image_url items require explicit roles")
				}
				unroledImageSeen = true
				info.StartFrameURL = imageInput.URL
			}
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	if info.Prompt == "" {
		return nil, errors.New("content must include a non-empty text item")
	}
	if info.EndFrameURL != "" && info.StartFrameURL == "" {
		return nil, errors.New("a last-frame image requires a first-frame image")
	}
	if len(info.References) > 4 {
		return nil, errors.New("at most 4 reference images are allowed")
	}
	if len(info.References) > 0 && (info.StartFrameURL != "" || info.EndFrameURL != "") {
		return nil, errors.New("reference images cannot be combined with first/last frames")
	}
	return info, nil
}

func parseSeedanceImageInput(item SeedanceContentItem) (*seedanceImageInput, error) {
	if len(item.ImageURL) == 0 || string(item.ImageURL) == "null" {
		return nil, errors.New("image_url is required for image content")
	}
	input := &seedanceImageInput{Role: item.Role, Strength: item.Strength}
	var directURL string
	if err := json.Unmarshal(item.ImageURL, &directURL); err == nil {
		input.URL = directURL
	} else {
		decoder := json.NewDecoder(bytes.NewReader(item.ImageURL))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(input); err != nil {
			return nil, errors.New("image_url must be a URL/data URI string or an object containing url or base64")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, errors.New("image_url must contain exactly one object")
		}
	}
	input.URL = strings.TrimSpace(input.URL)
	input.Base64 = strings.TrimSpace(input.Base64)
	if input.URL != "" && input.Base64 != "" {
		return nil, errors.New("image_url.url and image_url.base64 are mutually exclusive")
	}
	if input.Base64 != "" {
		mediaType := normalizeSeedanceInlineImageMediaType(input.MediaType)
		if mediaType == "" {
			return nil, errors.New("image_url.media_type must be image/png, image/jpeg, or image/webp when base64 is used")
		}
		input.URL = "data:" + mediaType + ";base64," + input.Base64
	}
	if input.URL == "" {
		return nil, errors.New("image_url.url or image_url.base64 is required")
	}
	if strings.HasPrefix(strings.ToLower(input.URL), "data:") {
		if _, _, err := splitSeedanceImageDataURI(input.URL); err != nil {
			return nil, err
		}
		return input, nil
	}
	parsed, err := url.Parse(input.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("image_url.url must be an absolute HTTP(S) URL")
	}
	return input, nil
}

func normalizeSeedanceInlineImageMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func splitSeedanceImageDataURI(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return "", "", errors.New("image data URI must start with data:")
	}
	comma := strings.IndexByte(value, ',')
	if comma <= len("data:") || comma == len(value)-1 {
		return "", "", errors.New("image data URI is invalid")
	}
	header := value[len("data:"):comma]
	parts := strings.Split(header, ";")
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[1]), "base64") {
		return "", "", errors.New("image data URI must use base64 encoding")
	}
	mediaType := normalizeSeedanceInlineImageMediaType(parts[0])
	if mediaType == "" {
		return "", "", errors.New("image data URI media type must be image/png, image/jpeg, or image/webp")
	}
	return mediaType, value[comma+1:], nil
}

func normalizeSeedanceImageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "first_frame", "start_frame", "first":
		return "first_frame"
	case "last_frame", "end_frame", "last":
		return "last_frame"
	case "reference_image", "reference", "ref":
		return "reference_image"
	default:
		return ""
	}
}

func normalizeSeedanceStrength(strength string) string {
	switch strings.ToUpper(strings.TrimSpace(strength)) {
	case "LOW", "MID", "HIGH":
		return strings.ToUpper(strings.TrimSpace(strength))
	default:
		return "MID"
	}
}

func validateSeedanceAspectRatio(ratio string) error {
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "", "adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21":
		return nil
	default:
		return errors.New("ratio must be adaptive, 16:9, 9:16, 1:1, 4:3, 3:4, 21:9, or 9:21")
	}
}

func (i *SeedanceRequestInfo) UpstreamBody(upstreamModel string) ([]byte, error) {
	if i == nil {
		return nil, errors.New("seedance request info is required")
	}
	body := map[string]any{
		"model":      strings.TrimSpace(upstreamModel),
		"prompt":     i.Prompt,
		"resolution": i.Resolution,
		"duration":   i.DurationSeconds,
		"audio":      i.GenerateAudio,
	}
	if ratio := strings.TrimSpace(i.AspectRatio); ratio != "" && !strings.EqualFold(ratio, "adaptive") {
		body["aspect_ratio"] = ratio
	}
	if len(i.References) > 0 {
		references := make([]map[string]any, 0, len(i.References))
		for order, reference := range i.References {
			if !isSeedanceHTTPImageURL(reference.URL) {
				return nil, errors.New("inline/reference image must be uploaded before forwarding")
			}
			references = append(references, map[string]any{
				"image":    map[string]any{"url": reference.URL, "type": "UPLOADED"},
				"strength": reference.Strength,
				"order":    order,
			})
		}
		body["guidances"] = map[string]any{"image_reference": references}
	} else if i.EndFrameURL != "" {
		if !isSeedanceHTTPImageURL(i.StartFrameURL) || !isSeedanceHTTPImageURL(i.EndFrameURL) {
			return nil, errors.New("inline first/last frame must be uploaded before forwarding")
		}
		body["start_frame_url"] = i.StartFrameURL
		body["end_frame_url"] = i.EndFrameURL
	} else if i.StartFrameURL != "" {
		if !isSeedanceHTTPImageURL(i.StartFrameURL) {
			return nil, errors.New("inline first-frame image must be uploaded before forwarding")
		}
		body["image_url"] = i.StartFrameURL
	}
	return json.Marshal(body)
}

func isSeedanceHTTPImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func SeedanceTaskSessionHash(taskID string, userID, apiKeyID int64) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return "seedance-task:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d:%d:%s", userID, apiKeyID, taskID))
}

func SeedanceUsageRequestID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return "seedance:" + taskID
}

func (s *OpenAIGatewayService) BindSeedanceTaskAccount(ctx context.Context, groupID *int64, taskID string, userID, apiKeyID, accountID int64) error {
	if s == nil || s.cache == nil {
		return errors.New("seedance task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(SeedanceTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" || accountID <= 0 {
		return errors.New("seedance task binding is invalid")
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, seedanceTaskBindingTTL)
}

func (s *OpenAIGatewayService) ResolveSeedanceTaskAccount(ctx context.Context, groupID *int64, taskID string, userID, apiKeyID int64) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, errors.New("seedance task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(SeedanceTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" {
		return 0, errors.New("seedance task binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func (s *OpenAIGatewayService) ForwardSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
) (*SeedanceUpstreamResponse, error) {
	return s.forwardSeedance(ctx, c, account, method, taskID, requestInfo, nil)
}

func (s *OpenAIGatewayService) ForwardSeedanceContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	taskID string,
	rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	return s.forwardSeedance(ctx, c, account, http.MethodGet, taskID, nil, &rangeHeader)
}

func (s *OpenAIGatewayService) forwardSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsSeedance() || account.Type != AccountTypeAPIKey {
		return nil, errors.New("Seedance forwarding requires a Seedance API key account")
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	path := seedanceUpstreamCreatePath
	var requestBody []byte
	requestModel := ""
	upstreamModel := ""
	if method == http.MethodPost {
		if requestInfo == nil {
			return nil, errors.New("Seedance create request is required")
		}
		requestModel = requestInfo.Model
		upstreamModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestModel))
		var err error
		requestBody, err = requestInfo.UpstreamBody(upstreamModel)
		if err != nil {
			return nil, err
		}
	} else {
		if !seedanceTaskIDPattern.MatchString(strings.TrimSpace(taskID)) {
			return nil, errors.New("invalid Seedance task id")
		}
		path = seedanceUpstreamJobsPath + "/" + url.PathEscape(strings.TrimSpace(taskID))
		if c != nil && strings.HasSuffix(c.Request.URL.Path, "/content") {
			path += "/content"
		}
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEndpointURL(baseURL, path)
	SetActualOpenAIUpstreamEndpoint(c, path)

	var bodyReader io.Reader
	if len(requestBody) > 0 {
		bodyReader = bytes.NewReader(requestBody)
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build Seedance upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	upstreamReq.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Prefer", "respond-async")
		if c != nil {
			if idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key")); idempotencyKey != "" {
				upstreamReq.Header.Set("Idempotency-Key", idempotencyKey)
			}
		}
	}
	if c != nil && strings.HasSuffix(path, "/content") {
		rangeHeader := strings.TrimSpace(c.GetHeader("Range"))
		if contentRangeOverride != nil {
			rangeHeader = strings.TrimSpace(*contentRangeOverride)
		}
		if rangeHeader != "" {
			upstreamReq.Header.Set("Range", rangeHeader)
		}
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("Seedance upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	isContentResponse := strings.HasSuffix(path, "/content")
	if resp.StatusCode >= http.StatusBadRequest && !(isContentResponse && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		defer func() { _ = resp.Body.Close() }()
		responseBody := sanitizeSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(responseBody))
		if method == http.MethodPost && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, message, responseBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           responseBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: responseBody}
	}

	response := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: contentType}
	if isContentResponse {
		response.BodyStream = resp.Body
		return response, nil
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	response.Body = responseBody
	if method == http.MethodPost {
		taskID := extractSeedanceUpstreamTaskID(responseBody)
		if taskID == "" {
			return nil, errors.New("Seedance upstream response did not include job_id")
		}
		response.Result = &OpenAIForwardResult{
			RequestID:            firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id"), "seedance:"+taskID),
			ResponseID:           taskID,
			Model:                requestModel,
			BillingModel:         requestModel,
			UpstreamModel:        upstreamModel,
			UpstreamEndpoint:     path,
			ResponseHeaders:      resp.Header.Clone(),
			Duration:             time.Since(startedAt),
			VideoCount:           1,
			VideoResolution:      requestInfo.Resolution,
			VideoDurationSeconds: requestInfo.DurationSeconds,
		}
	}
	return response, nil
}

func extractSeedanceUpstreamTaskID(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"job_id", "id", "task_id"} {
		if value, ok := payload[key].(string); ok && seedanceTaskIDPattern.MatchString(strings.TrimSpace(value)) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func BuildSeedanceOfficialTaskResponse(taskID string, upstreamBody []byte, contentURL string) (map[string]any, error) {
	var upstream map[string]any
	if err := json.Unmarshal(upstreamBody, &upstream); err != nil {
		return nil, errors.New("invalid Seedance upstream task response")
	}
	status, _ := upstream["status"].(string)
	officialStatus := MapSeedanceTaskStatus(status)
	response := map[string]any{"id": taskID, "status": officialStatus}
	for _, key := range []string{"model", "created_at", "updated_at", "completed_at", "seed", "resolution", "duration", "ratio"} {
		if value, exists := upstream[key]; exists && value != nil {
			response[key] = value
		}
	}
	if officialStatus == "succeeded" {
		response["content"] = map[string]any{"video_url": strings.TrimSpace(contentURL)}
	}
	if officialStatus == "failed" {
		if value, exists := upstream["error"]; exists {
			response["error"] = value
		} else if value, exists := upstream["error_message"]; exists {
			response["error"] = map[string]any{"message": value}
		}
	}
	return response, nil
}

func MapSeedanceTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued":
		return "queued"
	case "running", "processing", "settling":
		return "running"
	case "completed", "succeeded", "success":
		return "succeeded"
	case "failed", "error":
		return "failed"
	case "canceled", "cancelled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func SeedanceUpstreamErrorMessage(body []byte) string {
	message := strings.TrimSpace(extractUpstreamErrorMessage(body))
	if message == "" {
		return "Seedance upstream request failed"
	}
	return sanitizeUpstreamErrorMessage(message)
}

func sanitizeSeedanceUpstreamErrorBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	return seedanceSensitiveQueryParamPattern.ReplaceAll(body, []byte("${1}***"))
}

func writeSeedanceContentResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	writeOpenAIMediaResponseHeaders(dst, src, filter)
	if mediaType, _, err := mime.ParseMediaType(src.Get("Content-Type")); err == nil && strings.HasPrefix(mediaType, "video/") {
		dst.Set("Content-Type", mediaType)
	}
}

func (s *OpenAIGatewayService) WriteSeedanceContentResponseHeaders(dst http.Header, src http.Header) {
	if s == nil {
		return
	}
	writeSeedanceContentResponseHeaders(dst, src, s.responseHeaderFilter)
}
