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
	VideoProviderTianyue         = "tianyue"
	DefaultTianyueVideoBaseURL   = "http://192.220.23.225:3000"
	SeedanceTianyueSD20Model     = "B-SD2.0-933"
	SeedanceTianyueSD20FastModel = "B-SD2.0-F-933"
	tianyueVideoTaskPath         = "/v1/videos"
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

func (a *Account) IsTianyueVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderTianyue
}

func buildTianyueVideoCreateRequest(info *SeedanceRequestInfo, upstreamModel string) ([]byte, error) {
	if info == nil {
		return nil, errors.New("video request is required")
	}
	canonicalModel, ok := canonicalTianyueVideoModel(upstreamModel)
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
		upstreamModel, ok = canonicalTianyueVideoModel(mappedModel)
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
			query, queryErr := s.doTianyueRequest(ctx, c, account, http.MethodGet, buildXimeiEndpointURL(baseURL, path), path, nil, "", true)
			if queryErr != nil {
				return nil, queryErr
			}
			status, resultURL, parseErr := parseTianyueTaskResult(query.Body)
			if parseErr != nil {
				return nil, parseErr
			}
			if MapSeedanceTaskStatus(status) != SeedanceTaskStatusSucceeded {
				return nil, &SeedanceUpstreamError{StatusCode: http.StatusConflict, Body: []byte("video task is not completed")}
			}
			validatedURL, validateErr := s.validateTianyueResultURL(resultURL)
			if validateErr != nil {
				return nil, errors.New("Tianyue result URL is invalid")
			}
			return s.doTianyueRequest(ctx, c, account, http.MethodGet, validatedURL, tianyueVideoTaskPath+"/{task_id}/content", nil, rangeValue(contentRangeOverride), false)
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
		return nil, &SeedanceUpstreamError{StatusCode: response.StatusCode, Body: sanitizeHuiquSeedanceUpstreamErrorBody(out.Body)}
	}
	if method == http.MethodPost {
		out.Result = &OpenAIForwardResult{Duration: time.Since(startedAt)}
	}
	return out, nil
}

func (s *OpenAIGatewayService) validateTianyueResultURL(raw string) (string, error) {
	allowInsecure := false
	allowPrivate := false
	if s != nil && s.cfg != nil {
		allowInsecure = s.cfg.Security.URLAllowlist.AllowInsecureHTTP
		allowPrivate = s.cfg.Security.URLAllowlist.AllowPrivateHosts
	}
	validated, err := urlvalidator.ValidateHTTPURL(raw, allowInsecure, urlvalidator.ValidationOptions{AllowPrivate: allowPrivate})
	if err != nil || allowPrivate {
		return validated, err
	}
	parsed, err := url.Parse(validated)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("invalid result URL")
	}
	if err := urlvalidator.ValidateResolvedIP(parsed.Hostname()); err != nil {
		return "", err
	}
	return validated, nil
}

func parseTianyueTaskResult(body []byte) (string, string, error) {
	var payload struct {
		Status   string `json:"status"`
		URL      string `json:"url"`
		VideoURL string `json:"video_url"`
		Metadata struct {
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
	resultURL := firstNonEmptyString(strings.TrimSpace(payload.VideoURL), strings.TrimSpace(payload.URL), strings.TrimSpace(payload.Metadata.URL))
	if MapSeedanceTaskStatus(status) == SeedanceTaskStatusSucceeded && resultURL == "" {
		return "", "", errors.New("completed Tianyue task response is missing video_url")
	}
	return status, resultURL, nil
}

func tianyuePublicTaskID(accountID int64, upstreamTaskID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("tianyue:%d:%s", accountID, strings.TrimSpace(upstreamTaskID))))
	return "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}
