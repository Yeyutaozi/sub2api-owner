package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIAudioSpeechEndpoint         = "/v1/audio/speech"
	openAIAudioTranscriptionsEndpoint = "/v1/audio/transcriptions"
	openAIAudioTranslationsEndpoint   = "/v1/audio/translations"
	openAIVideosEndpoint              = "/v1/videos"
	openAIMediaModelFieldMaxBytes     = 4096
)

var openAIMediaResourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

var openAIMediaRequestHeaderAllowlist = map[string]struct{}{
	"accept":          {},
	"accept-language": {},
	"content-type":    {},
	"idempotency-key": {},
	"if-none-match":   {},
	"if-range":        {},
	"openai-beta":     {},
	"range":           {},
	"user-agent":      {},
}

var openAIMediaExtraResponseHeaders = []string{
	"Accept-Ranges",
	"Content-Disposition",
	"Content-Length",
	"Content-Range",
}

type OpenAIMediaRequest struct {
	Endpoint           string
	Method             string
	ContentType        string
	Model              string
	Multipart          bool
	Billable           bool
	ResourceID         string
	RequiredCapability OpenAIEndpointCapability
}

func (r *OpenAIMediaRequest) IsVideoCreate() bool {
	return r != nil && r.Method == http.MethodPost && r.Endpoint == openAIVideosEndpoint
}

func (s *OpenAIGatewayService) ParseOpenAIMediaRequest(c *gin.Context, body []byte) (*OpenAIMediaRequest, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return nil, fmt.Errorf("missing request context")
	}

	parsed, err := classifyOpenAIMediaRequest(c.Request.Method, c.Request.URL.Path)
	if err != nil {
		return nil, err
	}
	parsed.ContentType = strings.TrimSpace(c.GetHeader("Content-Type"))
	if !parsed.Billable {
		return parsed, nil
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}

	mediaType, _, err := mime.ParseMediaType(parsed.ContentType)
	if err != nil {
		return nil, fmt.Errorf("invalid content-type: %w", err)
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	switch parsed.Endpoint {
	case openAIAudioSpeechEndpoint:
		if !isOpenAIMediaJSONType(mediaType) {
			return nil, fmt.Errorf("audio speech requires application/json")
		}
	case openAIAudioTranscriptionsEndpoint, openAIAudioTranslationsEndpoint:
		if mediaType != "multipart/form-data" {
			return nil, fmt.Errorf("audio upload requires multipart/form-data")
		}
	case openAIVideosEndpoint:
		if !isOpenAIMediaJSONType(mediaType) && mediaType != "multipart/form-data" {
			return nil, fmt.Errorf("video creation requires application/json or multipart/form-data")
		}
	}

	if mediaType == "multipart/form-data" {
		parsed.Multipart = true
		parsed.Model, err = parseOpenAIMediaMultipartModel(body, parsed.ContentType)
	} else {
		if !gjson.ValidBytes(body) {
			return nil, fmt.Errorf("failed to parse request body")
		}
		model := gjson.GetBytes(body, "model")
		if model.Exists() && model.Type != gjson.String {
			return nil, fmt.Errorf("model must be a string")
		}
		parsed.Model = strings.TrimSpace(model.String())
	}
	if err != nil {
		return nil, err
	}
	if parsed.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	return parsed, nil
}

func classifyOpenAIMediaRequest(method string, requestPath string) (*OpenAIMediaRequest, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	requestPath = strings.TrimSpace(requestPath)

	if method == http.MethodPost {
		switch requestPath {
		case openAIAudioSpeechEndpoint:
			return &OpenAIMediaRequest{Endpoint: requestPath, Method: method, Billable: true, RequiredCapability: OpenAIEndpointCapabilityAudioSpeech}, nil
		case openAIAudioTranscriptionsEndpoint:
			return &OpenAIMediaRequest{Endpoint: requestPath, Method: method, Billable: true, RequiredCapability: OpenAIEndpointCapabilityTranscriptions}, nil
		case openAIAudioTranslationsEndpoint:
			return &OpenAIMediaRequest{Endpoint: requestPath, Method: method, Billable: true, RequiredCapability: OpenAIEndpointCapabilityTranslations}, nil
		case openAIVideosEndpoint:
			return &OpenAIMediaRequest{Endpoint: requestPath, Method: method, Billable: true, RequiredCapability: OpenAIEndpointCapabilityVideos}, nil
		default:
			return nil, fmt.Errorf("unsupported media endpoint")
		}
	}

	if method != http.MethodGet && method != http.MethodDelete {
		return nil, fmt.Errorf("unsupported media method")
	}
	if !strings.HasPrefix(requestPath, openAIVideosEndpoint+"/") {
		return nil, fmt.Errorf("unsupported media endpoint")
	}

	suffix := strings.TrimPrefix(requestPath, openAIVideosEndpoint+"/")
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || !openAIMediaResourceIDPattern.MatchString(parts[0]) {
		return nil, fmt.Errorf("invalid video resource id")
	}
	if method == http.MethodDelete && len(parts) != 1 {
		return nil, fmt.Errorf("unsupported video delete endpoint")
	}
	if method == http.MethodGet && (len(parts) > 2 || (len(parts) == 2 && parts[1] != "content")) {
		return nil, fmt.Errorf("unsupported video get endpoint")
	}

	endpoint := openAIVideosEndpoint + "/" + parts[0]
	if len(parts) == 2 {
		endpoint += "/content"
	}
	return &OpenAIMediaRequest{
		Endpoint:           endpoint,
		Method:             method,
		ResourceID:         parts[0],
		RequiredCapability: OpenAIEndpointCapabilityVideos,
	}, nil
}

func parseOpenAIMediaMultipartModel(body []byte, contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("invalid multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	model := ""
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read multipart body: %w", err)
		}
		if strings.TrimSpace(part.FormName()) != "model" || part.FileName() != "" {
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}

		value, readErr := io.ReadAll(io.LimitReader(part, openAIMediaModelFieldMaxBytes+1))
		_ = part.Close()
		if readErr != nil {
			return "", fmt.Errorf("read multipart model: %w", readErr)
		}
		if len(value) > openAIMediaModelFieldMaxBytes {
			return "", fmt.Errorf("multipart model is too large")
		}
		if model != "" {
			return "", fmt.Errorf("multipart model must be provided once")
		}
		model = strings.TrimSpace(string(value))
	}
	return model, nil
}

func isOpenAIMediaJSONType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func OpenAIMediaResourceSessionHash(apiKeyID int64, resourceID string) string {
	seed := fmt.Sprintf("openai-media-video:%d:%s", apiKeyID, strings.TrimSpace(resourceID))
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func (s *OpenAIGatewayService) ForwardMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIMediaRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed media request is required")
	}
	if account == nil || !account.IsOpenAIApiKey() {
		return nil, fmt.Errorf("media endpoint requires an OpenAI API key account")
	}
	apiKey := strings.TrimSpace(account.GetOpenAIApiKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}

	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	forwardModel := requestModel
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		forwardModel = mapped
	}
	billingModel := resolveOpenAIForwardModel(account, forwardModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)

	forwardBody := body
	forwardContentType := parsed.ContentType
	var err error
	if parsed.Billable && upstreamModel != "" && upstreamModel != requestModel {
		forwardBody, forwardContentType, err = rewriteOpenAIMediaModel(body, parsed.ContentType, upstreamModel)
		if err != nil {
			return nil, err
		}
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL, err := url.Parse(buildOpenAIEndpointURL(baseURL, parsed.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("build media upstream url: %w", err)
	}
	if c != nil && c.Request != nil && c.Request.URL != nil {
		targetURL.RawQuery = c.Request.URL.RawQuery
		targetURL.ForceQuery = c.Request.URL.ForceQuery
	}

	var requestBody io.Reader
	if len(forwardBody) > 0 {
		requestBody = bytes.NewReader(forwardBody)
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, parsed.Method, targetURL.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("build media upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	if c != nil && c.Request != nil {
		copyOpenAIMediaRequestHeaders(upstreamReq.Header, c.Request.Header)
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if strings.TrimSpace(forwardContentType) != "" {
		upstreamReq.Header.Set("Content-Type", forwardContentType)
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMessage := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMessage, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMessage,
			})
			s.handleFailoverSideEffects(ctx, resp, account, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		writeOpenAIMediaResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(resp.StatusCode, contentType, respBody)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	result := &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		Model:           requestModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if isOpenAIMediaJSONType(mediaType) {
		respBody, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
		if readErr != nil {
			return nil, readErr
		}
		writeOpenAIMediaResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(resp.StatusCode, contentType, respBody)
		if usage, ok := extractOpenAIUsageFromJSONBytes(respBody); ok {
			result.Usage = usage
		}
		result.ResponseID = extractOpenAIResponseIDFromJSONBytes(respBody)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	writeOpenAIMediaResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	if contentType != "" {
		c.Writer.Header().Set("Content-Type", contentType)
	}
	c.Status(resp.StatusCode)
	copyBuffer := make([]byte, 32<<10)
	if _, copyErr := io.CopyBuffer(c.Writer, resp.Body, copyBuffer); copyErr != nil {
		result.ClientDisconnect = true
	}
	result.Duration = time.Since(startTime)
	return result, nil
}

func copyOpenAIMediaRequestHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if _, allowed := openAIMediaRequestHeaderAllowlist[strings.ToLower(key)]; !allowed {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeOpenAIMediaResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	responseheaders.WriteFilteredHeaders(dst, src, filter)
	for _, key := range openAIMediaExtraResponseHeaders {
		values := src.Values(key)
		if len(values) == 0 {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func rewriteOpenAIMediaModel(body []byte, contentType string, model string) ([]byte, string, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse media content-type: %w", err)
	}
	if strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIMediaMultipartModel(body, contentType, model)
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite media request model: %w", err)
	}
	return rewritten, contentType, nil
}

func rewriteOpenAIMediaMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	modelWritten := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}
		target, err := writer.CreatePart(cloneOpenAIMediaMIMEHeader(part.Header))
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}
		if strings.TrimSpace(part.FormName()) == "model" && part.FileName() == "" {
			if _, err := target.Write([]byte(model)); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("rewrite multipart model: %w", err)
			}
			modelWritten = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}
	if !modelWritten {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("append multipart model field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func cloneOpenAIMediaMIMEHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}
