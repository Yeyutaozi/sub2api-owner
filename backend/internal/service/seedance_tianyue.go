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

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	VideoProviderTianyue             = "tianyue"
	DefaultTianyueVideoBaseURL       = "http://192.220.23.225:3000"
	SeedanceTianyueSD20Model         = "L-SD2-F-720-933"
	SeedanceTianyueSD20UpstreamModel = "ME-SD2.0-933"
	SeedanceTianyueSD20FastModel     = "L-stable-seedance-2-0-933-720p"
	tianyueVideoTaskPath             = "/v1/videos"
)

type tianyueVideoCreateRequest struct {
	Model         string   `json:"model"`
	Prompt        string   `json:"prompt"`
	Duration      int      `json:"duration"`
	VideoDuration int      `json:"video_duration"`
	AspectRatio   string   `json:"aspect_ratio,omitempty"`
	Resolution    string   `json:"resolution"`
	ImageURLs     []string `json:"image_urls"`
	AudioURLs     []string `json:"audio_urls,omitempty"`
	VideoURLs     []string `json:"video_urls,omitempty"`
}

func isTianyueVideoModel(model string) bool {
	_, ok := canonicalTianyueVideoModel(model)
	return ok
}

func canonicalTianyueVideoModel(model string) (string, bool) {
	switch {
	case strings.EqualFold(strings.TrimSpace(model), SeedanceTianyueSD20Model):
		return SeedanceTianyueSD20Model, true
	case strings.EqualFold(strings.TrimSpace(model), SeedanceTianyueSD20FastModel):
		return SeedanceTianyueSD20FastModel, true
	default:
		return "", false
	}
}

func tianyueUpstreamVideoModel(model string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(model), SeedanceTianyueSD20UpstreamModel) {
		return SeedanceTianyueSD20UpstreamModel, true
	}
	canonical, ok := canonicalTianyueVideoModel(model)
	if !ok {
		return "", false
	}
	if canonical == SeedanceTianyueSD20Model {
		return SeedanceTianyueSD20UpstreamModel, true
	}
	return canonical, true
}

// normalizeTianyueModelMappingCredentials keeps the public model IDs as the
// mapping keys while storing the provider's actual execution IDs as targets.
// This makes account editing deterministic: a whitelist entry for the public
// SD2 model is persisted as an explicit mapping to ME-SD2.0-933.
func normalizeTianyueModelMappingCredentials(credentials map[string]any) {
	if credentials == nil {
		return
	}
	provider, _ := credentials["video_provider"].(string)
	if !strings.EqualFold(strings.TrimSpace(provider), VideoProviderTianyue) {
		return
	}
	mapping := stringMappingFromRaw(credentials["model_mapping"])
	if len(mapping) == 0 {
		return
	}
	normalized := make(map[string]any, len(mapping))
	for from, to := range mapping {
		if upstream, ok := tianyueUpstreamVideoModel(to); ok {
			normalized[from] = upstream
		} else {
			normalized[from] = to
		}
	}
	credentials["model_mapping"] = normalized
}

func (a *Account) IsTianyueVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderTianyue
}

func buildTianyueVideoCreateRequest(info *SeedanceRequestInfo, upstreamModel string) ([]byte, error) {
	if info == nil {
		return nil, errors.New("video request is required")
	}
	canonicalModel, ok := tianyueUpstreamVideoModel(upstreamModel)
	if !ok {
		return nil, fmt.Errorf("unsupported Tianyue video model: %s", strings.TrimSpace(upstreamModel))
	}
	request := tianyueVideoCreateRequest{
		Model:         canonicalModel,
		Prompt:        info.Prompt,
		Duration:      1,
		VideoDuration: info.DurationSeconds,
		AspectRatio:   info.AspectRatio,
		Resolution:    info.Resolution,
		ImageURLs:     make([]string, 0),
	}
	for _, image := range ximeiOrderedImageSlots(info) {
		if value := strings.TrimSpace(image.URL); value != "" {
			request.ImageURLs = append(request.ImageURLs, value)
		}
	}
	for _, audio := range info.AudioReferences {
		if value := strings.TrimSpace(audio.URL); value != "" {
			request.AudioURLs = append(request.AudioURLs, value)
		}
	}
	for _, video := range info.VideoReferences {
		if value := strings.TrimSpace(video.URL); value != "" {
			request.VideoURLs = append(request.VideoURLs, value)
		}
	}
	return json.Marshal(request)
}

func (s *OpenAIGatewayService) forwardTianyueSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method, taskID string,
	info *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsTianyueVideo() {
		return nil, errors.New("video forwarding requires a compatible Tianyue API key account")
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path := tianyueVideoTaskPath
	var body []byte
	upstreamModel := ""
	if method == http.MethodPost {
		if info == nil {
			return nil, errors.New("video request is required")
		}
		mappedModel := strings.TrimSpace(account.GetMappedModel(info.Model))
		var ok bool
		upstreamModel, ok = tianyueUpstreamVideoModel(mappedModel)
		if !ok {
			return nil, fmt.Errorf("unsupported Tianyue video model: %s", mappedModel)
		}
		body, err = buildTianyueVideoCreateRequest(info, upstreamModel)
		if err != nil {
			return nil, err
		}
	} else if method == http.MethodGet {
		taskID = strings.TrimSpace(taskID)
		if !seedanceTaskIDPattern.MatchString(taskID) {
			return nil, errors.New("invalid Tianyue task id")
		}
		path += "/" + url.PathEscape(taskID)
		if contentRangeOverride != nil {
			return s.forwardTianyueSeedanceContent(
				ctx, c, account, baseURL, taskID, rangeValue(contentRangeOverride),
			)
		}
	} else {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("this video provider does not support this method")}
	}

	response, err := s.doTianyueRequest(ctx, c, account, method, buildXimeiEndpointURL(baseURL, path), path, body, "", true)
	if err != nil || method != http.MethodPost {
		return response, err
	}
	upstreamTaskID := extractSeedanceUpstreamTaskID(response.Body)
	if upstreamTaskID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: errors.New("Tianyue response did not include a task id")}
	}
	publicTaskID := tianyuePublicTaskID(account.ID, upstreamTaskID)
	duration := time.Duration(0)
	if response.Result != nil {
		duration = response.Result.Duration
	}
	response.Result = &OpenAIForwardResult{
		RequestID: "seedance:" + publicTaskID, ResponseID: publicTaskID, UpstreamResponseID: upstreamTaskID,
		Model: info.Model, BillingModel: info.Model, UpstreamModel: upstreamModel, UpstreamEndpoint: path,
		ResponseHeaders: response.Header.Clone(), Duration: duration, VideoCount: 1,
		VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds,
	}
	return response, nil
}

func (s *OpenAIGatewayService) forwardTianyueSeedanceContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL, taskID, rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	queryPath := tianyueVideoTaskPath + "/" + url.PathEscape(taskID)
	query, err := s.doTianyueRequest(
		ctx, c, account, http.MethodGet, buildXimeiEndpointURL(baseURL, queryPath), queryPath, nil, "", true,
	)
	if err != nil {
		return nil, err
	}
	status, resultURL, err := parseTianyueTaskResult(query.Body)
	if err != nil {
		return nil, err
	}
	if MapSeedanceTaskStatus(status) != SeedanceTaskStatusSucceeded {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusConflict, Body: []byte("video task is not completed")}
	}

	validatedURL, initialHTTPS, err := validateTianyueVideoResultURL(resultURL, true)
	if err != nil {
		return nil, errors.New("Tianyue video result URL is invalid")
	}
	response, err := s.doTianyueRequest(
		ctx, c, account, http.MethodGet, validatedURL, tianyueVideoTaskPath+"/{task_id}/content", nil, rangeHeader, false,
	)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusMultipleChoices || response.StatusCode >= http.StatusBadRequest {
		if !initialHTTPS {
			closeSeedanceUpstreamBody(response)
			return nil, errors.New("Tianyue video result URL did not redirect to HTTPS")
		}
		return response, nil
	}

	location := strings.TrimSpace(response.Header.Get("Location"))
	closeSeedanceUpstreamBody(response)
	redirectURL, err := resolveTianyueVideoRedirect(validatedURL, location)
	if err != nil {
		return nil, errors.New("Tianyue video result redirect is invalid")
	}
	redirectURL, _, err = validateTianyueVideoResultURL(redirectURL, false)
	if err != nil {
		return nil, errors.New("Tianyue video result redirect is invalid")
	}
	response, err = s.doTianyueRequest(
		ctx, c, account, http.MethodGet, redirectURL, tianyueVideoTaskPath+"/{task_id}/content", nil, rangeHeader, false,
	)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		closeSeedanceUpstreamBody(response)
		return nil, errors.New("Tianyue video result redirected more than once")
	}
	return response, nil
}

func parseTianyueTaskResult(body []byte) (string, string, error) {
	var payload struct {
		Status    string `json:"status"`
		URL       string `json:"url"`
		VideoURL  string `json:"video_url"`
		ResultURL string `json:"result_url"`
		Metadata  struct {
			URL string `json:"url"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", errors.New("invalid Tianyue task response")
	}
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		return "", "", errors.New("Tianyue task response is missing status")
	}
	resultURL := firstTianyueResultURL(payload.VideoURL, payload.ResultURL, payload.Metadata.URL, payload.URL)
	if MapSeedanceTaskStatus(status) == SeedanceTaskStatusSucceeded && resultURL == "" {
		return "", "", errors.New("completed Tianyue task response is missing video_url")
	}
	return status, resultURL, nil
}

func firstTianyueResultURL(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, "none") || strings.EqualFold(value, "null") {
			continue
		}
		return value
	}
	return ""
}

func validateTianyueVideoResultURL(raw string, allowHTTPRedirectBootstrap bool) (string, bool, error) {
	validated, err := urlvalidator.ValidateHTTPURL(
		raw, allowHTTPRedirectBootstrap, urlvalidator.ValidationOptions{AllowPrivate: false},
	)
	if err != nil {
		return "", false, err
	}
	parsed, err := url.Parse(validated)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return "", false, errors.New("invalid Tianyue video result URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		if port := parsed.Port(); port != "" && port != "443" {
			return "", false, errors.New("invalid Tianyue HTTPS result port")
		}
	case "http":
		if !allowHTTPRedirectBootstrap || parsed.Port() != "15036" {
			return "", false, errors.New("invalid Tianyue HTTP redirect port")
		}
	default:
		return "", false, errors.New("invalid Tianyue video result scheme")
	}
	if err := urlvalidator.ValidateResolvedIP(parsed.Hostname()); err != nil {
		return "", false, err
	}
	return validated, strings.EqualFold(parsed.Scheme, "https"), nil
}

func resolveTianyueVideoRedirect(baseURL, location string) (string, error) {
	if strings.TrimSpace(location) == "" {
		return "", errors.New("redirect location is missing")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	target, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(target).String(), nil
}

func closeSeedanceUpstreamBody(response *SeedanceUpstreamResponse) {
	if response != nil && response.BodyStream != nil {
		_ = response.BodyStream.Close()
	}
}

func (s *OpenAIGatewayService) doTianyueRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method, targetURL, endpoint string,
	body []byte,
	rangeHeader string,
	authenticated bool,
) (*SeedanceUpstreamResponse, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	requestCtx := ctx
	isContent := strings.HasSuffix(endpoint, "/content")
	if isContent {
		requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, targetURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build Tianyue video request: %w", err)
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+account.GetSeedanceAPIKey())
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
	response, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		forwardErr := fmt.Errorf("Tianyue video upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
		if method == http.MethodPost {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: forwardErr}
		}
		return nil, forwardErr
	}
	out := &SeedanceUpstreamResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), ContentType: strings.TrimSpace(response.Header.Get("Content-Type"))}
	if isContent && (response.StatusCode < http.StatusBadRequest || response.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		out.BodyStream = response.Body
		return out, nil
	}
	defer response.Body.Close()
	out.Body, err = ReadUpstreamResponseBody(response.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		// Tianyue has its own response schema. Keep its error object intact so
		// create-time 4xx responses can reach the canvas without being replaced
		// by the Huiqu sanitizer's generic failure body.
		return nil, &SeedanceUpstreamError{StatusCode: response.StatusCode, Body: sanitizeSeedanceUpstreamErrorBody(out.Body)}
	}
	if method == http.MethodPost {
		out.Result = &OpenAIForwardResult{Duration: time.Since(startedAt)}
	}
	return out, nil
}

func tianyuePublicTaskID(accountID int64, upstreamTaskID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("tianyue:%d:%s", accountID, strings.TrimSpace(upstreamTaskID))))
	return "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}
