package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	DefaultLensForgeVideoBaseURL = "https://lensforge.tsxzz.com"
	lensForge933OfferingID       = "11002595-11bb-4d23-a2f2-5ae090151630"
	lensForgeVideoPath           = "/v1/videos"
)

type lensForgeCreateRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Seconds        string `json:"seconds"`
	Size           string `json:"size"`
	GenerateAudio  bool   `json:"generate_audio"`
	InputReference string `json:"input_reference,omitempty"`
}

func (a *Account) IsLensForgeVideo() bool {
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderLensForge
}

func buildLensForgeCreateRequest(info *SeedanceRequestInfo) ([]byte, error) {
	if info == nil || !strings.EqualFold(strings.TrimSpace(info.Model), SeedanceLensForge933Model) {
		return nil, errors.New("unsupported LensForge video model")
	}
	if info.DurationSeconds < 4 || info.DurationSeconds > 15 {
		return nil, errors.New("sd2.0-933-345 duration must be between 4 and 15 seconds")
	}
	if len(info.VideoReferences) > 0 || len(info.AudioReferences) > 0 || len(info.References) > 1 || info.EndFrameURL != "" {
		return nil, errors.New("sd2.0-933-345 currently supports text-to-video or one input reference image")
	}
	reference := strings.TrimSpace(info.StartFrameURL)
	if reference == "" && len(info.References) == 1 {
		reference = strings.TrimSpace(info.References[0].URL)
	}
	size := "1280x720"
	return json.Marshal(lensForgeCreateRequest{
		Model: lensForge933OfferingID, Prompt: info.Prompt, Seconds: fmt.Sprintf("%d", info.DurationSeconds),
		Size: size, GenerateAudio: info.GenerateAudio, InputReference: reference,
	})
}

func (s *OpenAIGatewayService) forwardLensForgeSeedance(ctx context.Context, c *gin.Context, account *Account, method, taskID string, info *SeedanceRequestInfo, contentRangeOverride *string) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsLensForgeVideo() {
		return nil, errors.New("video forwarding requires a compatible LensForge API key account")
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path := lensForgeVideoPath
	var body []byte
	if method == http.MethodPost {
		body, err = buildLensForgeCreateRequest(info)
		if err != nil {
			return nil, err
		}
	} else if method == http.MethodGet {
		if !seedanceTaskIDPattern.MatchString(strings.TrimSpace(taskID)) {
			return nil, errors.New("invalid LensForge task id")
		}
		path += "/" + url.PathEscape(strings.TrimSpace(taskID))
		if contentRangeOverride != nil {
			path += "/content"
		}
	} else {
		return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("unsupported method")}
	}

	requestEndpoint := path
	if contentRangeOverride != nil {
		requestEndpoint = globalAIOPCVideoTaskPath + "/{task_id}/content"
	}
	response, err := s.doGlobalAIOPCRequest(ctx, c, account, method, buildXimeiEndpointURL(baseURL, path), requestEndpoint, account.GetSeedanceAPIKey(), body, rangeValue(contentRangeOverride))
	if err != nil || method != http.MethodPost {
		return response, err
	}
	upstreamID := extractGlobalAIOPCTaskID(response.Body)
	if upstreamID == "" {
		return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: errors.New("LensForge response did not include a task id")}
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("lensforge:%d:%s", account.ID, upstreamID)))
	publicID := "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18])
	duration := time.Duration(0)
	if response.Result != nil {
		duration = response.Result.Duration
	}
	response.Result = &OpenAIForwardResult{RequestID: "seedance:" + publicID, ResponseID: publicID, UpstreamResponseID: upstreamID, Model: info.Model, BillingModel: info.Model, UpstreamModel: lensForge933OfferingID, UpstreamEndpoint: path, ResponseHeaders: response.Header.Clone(), Duration: duration, VideoCount: 1, VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds}
	return response, nil
}

func rangeValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
