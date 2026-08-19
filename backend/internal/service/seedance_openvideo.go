package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
)

const (
	DefaultOpenVideoBaseURL = "https://www.openvideo.top/api/v1"
	openVideoTaskPath       = "/videos"
)

type openVideoCreateRequest struct {
	Model      string   `json:"model"`
	Prompt     string   `json:"prompt"`
	Object     string   `json:"object"`
	Images     []string `json:"images,omitempty"`
	Videos     []string `json:"videos,omitempty"`
	Audios     []string `json:"audios,omitempty"`
	Duration   int      `json:"duration,omitempty"`
	Ratio      string   `json:"ratio,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
}

func (a *Account) IsOpenVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderOpenVideo
}

func buildOpenVideoCreateRequest(info *SeedanceRequestInfo) ([]byte, error) {
	if info == nil || !strings.EqualFold(strings.TrimSpace(info.Model), SeedanceOpenVideoMiniModel) {
		return nil, errors.New("unsupported OpenVideo model")
	}
	if strings.TrimSpace(info.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	request := openVideoCreateRequest{
		Model: SeedanceOpenVideoMiniModel, Prompt: info.Prompt, Object: "video",
		Duration: info.DurationSeconds, Ratio: info.AspectRatio, Resolution: info.Resolution,
	}
	for _, reference := range info.References {
		request.Images = append(request.Images, strings.TrimSpace(reference.URL))
	}
	if value := strings.TrimSpace(info.StartFrameURL); value != "" {
		request.Images = append(request.Images, value)
	}
	if value := strings.TrimSpace(info.EndFrameURL); value != "" {
		request.Images = append(request.Images, value)
	}
	for _, reference := range info.VideoReferences {
		request.Videos = append(request.Videos, strings.TrimSpace(reference.URL))
	}
	for _, reference := range info.AudioReferences {
		request.Audios = append(request.Audios, strings.TrimSpace(reference.URL))
	}
	return json.Marshal(request)
}

func (s *OpenAIGatewayService) forwardOpenVideoSeedance(ctx context.Context, c *gin.Context, account *Account, method, taskID string, info *SeedanceRequestInfo, contentRangeOverride *string) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsOpenVideo() {
		return nil, errors.New("video forwarding requires a compatible OpenVideo API key account")
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path := openVideoTaskPath
	var body []byte
	if method == http.MethodPost {
		body, err = buildOpenVideoCreateRequest(info)
		if err != nil {
			return nil, err
		}
	} else if method == http.MethodGet {
		if !seedanceTaskIDPattern.MatchString(strings.TrimSpace(taskID)) {
			return nil, errors.New("invalid OpenVideo task id")
		}
		path += "/" + url.PathEscape(strings.TrimSpace(taskID))
		if contentRangeOverride != nil {
			query, queryErr := s.doGlobalAIOPCRequest(ctx, c, account, http.MethodGet, buildXimeiEndpointURL(baseURL, path), path, account.GetSeedanceAPIKey(), nil, "")
			if queryErr != nil {
				return nil, queryErr
			}
			_, resultURL, parseErr := parseOpenVideoTaskResult(query.Body)
			if parseErr != nil {
				return nil, parseErr
			}
			validatedURL, validateErr := urlvalidator.ValidateHTTPSURL(resultURL, urlvalidator.ValidationOptions{AllowPrivate: false})
			if validateErr != nil {
				return nil, errors.New("OpenVideo result URL is invalid")
			}
			parsedURL, parseURLErr := url.Parse(validatedURL)
			if parseURLErr != nil || parsedURL.User != nil || parsedURL.Fragment != "" || (parsedURL.Port() != "" && parsedURL.Port() != "443") {
				return nil, errors.New("OpenVideo result URL is invalid")
			}
			if resolveErr := urlvalidator.ValidateResolvedIP(parsedURL.Hostname()); resolveErr != nil {
				return nil, errors.New("OpenVideo result URL is invalid")
			}
			return s.doGlobalAIOPCRequest(ctx, c, account, http.MethodGet, validatedURL, globalAIOPCVideoTaskPath+"/{task_id}/content", "", nil, rangeValue(contentRangeOverride))
		}
	} else {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("unsupported method")}
	}
	response, err := s.doGlobalAIOPCRequest(ctx, c, account, method, buildXimeiEndpointURL(baseURL, path), path, account.GetSeedanceAPIKey(), body, "")
	if err != nil || method != http.MethodPost {
		return response, err
	}
	upstreamID := extractGlobalAIOPCTaskID(response.Body)
	if upstreamID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: errors.New("OpenVideo response did not include a task id")}
	}
	response.Result = &OpenAIForwardResult{RequestID: "seedance:" + upstreamID, ResponseID: upstreamID, UpstreamResponseID: upstreamID, Model: info.Model, BillingModel: info.Model, UpstreamModel: SeedanceOpenVideoMiniModel, UpstreamEndpoint: path, ResponseHeaders: response.Header.Clone(), VideoCount: 1, VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds}
	return response, nil
}

func parseOpenVideoTaskResult(body []byte) (string, string, error) {
	var payload struct {
		State      string   `json:"state"`
		ResultURL  string   `json:"result_url"`
		ResultURLs []string `json:"result_urls"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", errors.New("invalid OpenVideo task response")
	}
	resultURL := strings.TrimSpace(payload.ResultURL)
	if resultURL == "" && len(payload.ResultURLs) > 0 {
		resultURL = strings.TrimSpace(payload.ResultURLs[0])
	}
	if strings.EqualFold(payload.State, "succeeded") && resultURL == "" {
		return "", "", errors.New("completed OpenVideo task response is missing result_url")
	}
	return payload.State, resultURL, nil
}
