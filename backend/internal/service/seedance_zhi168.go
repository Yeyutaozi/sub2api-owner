package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type zhi168VideoRequest struct {
	ModelCode          string   `json:"model_code"`
	Prompt             string   `json:"prompt"`
	Resolution         string   `json:"resolution"`
	Duration           int      `json:"duration_seconds"`
	AspectRatio        string   `json:"aspect_ratio"`
	WithAudio          bool     `json:"with_audio"`
	ReferenceImageURLs []string `json:"reference_image_urls,omitempty"`
	VideoURLs          []string `json:"video_urls,omitempty"`
	AudioURLs          []string `json:"audio_urls,omitempty"`
}

func (s *OpenAIGatewayService) forwardZhi168Seedance(ctx context.Context, c *gin.Context, account *Account, method, taskID string, info *SeedanceRequestInfo, contentRangeOverride *string) (*SeedanceUpstreamResponse, error) {
	base, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, err
	}
	path := "/v1/video-tasks"
	var body []byte
	if method == http.MethodPost {
		if info == nil {
			return nil, errors.New("video request is required")
		}
		req := zhi168VideoRequest{ModelCode: SeedanceZhi168UpstreamModel, Prompt: info.Prompt, Resolution: VideoBillingResolution1080P, Duration: 15, AspectRatio: info.AspectRatio, WithAudio: info.GenerateAudio}
		for _, v := range info.References {
			req.ReferenceImageURLs = append(req.ReferenceImageURLs, v.URL)
		}
		for _, v := range info.VideoReferences {
			req.VideoURLs = append(req.VideoURLs, v.URL)
		}
		for _, v := range info.AudioReferences {
			req.AudioURLs = append(req.AudioURLs, v.URL)
		}
		body, err = json.Marshal(req)
	} else if method == http.MethodGet {
		if !seedanceTaskIDPattern.MatchString(taskID) {
			return nil, errors.New("invalid task id")
		}
		path += "/" + url.PathEscape(taskID)
		if contentRangeOverride != nil {
			query, queryErr := s.forwardZhi168Request(ctx, c, account, http.MethodGet, buildXimeiEndpointURL(base, path), path, nil, "")
			if queryErr != nil {
				return nil, queryErr
			}
			_, resultURL, parseErr := parseZhi168TaskResult(query.Body)
			if parseErr != nil {
				return nil, parseErr
			}
			validated, validateErr := urlvalidator.ValidateHTTPSURL(resultURL, urlvalidator.ValidationOptions{AllowPrivate: false})
			if validateErr != nil {
				return nil, errors.New("zhi168 result URL is invalid")
			}
			return s.forwardZhi168Request(ctx, c, account, http.MethodGet, validated, "/v1/video-tasks/{task_id}/content", nil, rangeValue(contentRangeOverride))
		}
	} else {
		return nil, errors.New("unsupported method")
	}
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, buildXimeiEndpointURL(base, path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", account.GetSeedanceAPIKey())
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	SetActualOpenAIUpstreamEndpoint(c, path)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: resp.Header.Get("Content-Type")}
	out.Body, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: out.Body}
	}
	if method == http.MethodPost {
		var created struct {
			TaskID int64 `json:"task_id"`
			ID     int64 `json:"id"`
			Data   struct {
				TaskID int64 `json:"task_id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(out.Body, &created)
		upstreamID := created.TaskID
		if upstreamID == 0 {
			upstreamID = created.ID
		}
		if upstreamID == 0 {
			upstreamID = created.Data.TaskID
		}
		if upstreamID == 0 {
			return nil, errors.New("zhi168 response did not include task_id")
		}
		taskID := strconv.FormatInt(upstreamID, 10)
		out.Result = &OpenAIForwardResult{RequestID: "seedance:" + taskID, ResponseID: taskID, UpstreamResponseID: taskID, Model: info.Model, BillingModel: info.Model, UpstreamModel: SeedanceZhi168UpstreamModel, UpstreamEndpoint: path, ResponseHeaders: out.Header.Clone(), VideoCount: 1, VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds}
	}
	return out, nil
}

func parseZhi168TaskResult(body []byte) (string, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", errors.New("invalid zhi168 task response")
	}
	status := zhi168String(payload, "status", "state")
	result := zhi168FindURL(payload)
	if status != "" && MapSeedanceTaskStatus(status) != SeedanceTaskStatusSucceeded {
		return status, "", &SeedanceUpstreamError{StatusCode: http.StatusConflict, Body: []byte("video task is not completed")}
	}
	if result == "" {
		return status, "", errors.New("completed zhi168 task response is missing result URL")
	}
	return status, result, nil
}

func zhi168String(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if nested, ok := m["data"].(map[string]any); ok {
		return zhi168String(nested, keys...)
	}
	return ""
}

func zhi168FindURL(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"result_url", "video_url", "output_url", "download_url", "url"} {
			if raw, ok := typed[key].(string); ok && strings.HasPrefix(strings.TrimSpace(raw), "http") {
				return strings.TrimSpace(raw)
			}
		}
		for _, child := range typed {
			if found := zhi168FindURL(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := zhi168FindURL(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func (s *OpenAIGatewayService) forwardZhi168Request(ctx context.Context, c *gin.Context, account *Account, method, target, endpoint string, body []byte, rangeHeader string) (*SeedanceUpstreamResponse, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(target, account.GetSeedanceBaseURL()) {
		req.Header.Set("X-API-Key", account.GetSeedanceAPIKey())
	}
	req.Header.Set("Accept", "*/*")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	SetActualOpenAIUpstreamEndpoint(c, endpoint)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	out := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: resp.Header.Get("Content-Type")}
	if strings.HasSuffix(endpoint, "/content") && resp.StatusCode < 400 {
		out.BodyStream = resp.Body
		return out, nil
	}
	defer resp.Body.Close()
	out.Body, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: out.Body}
	}
	return out, nil
}
