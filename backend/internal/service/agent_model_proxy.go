package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	agentModelProxyMaxRequestBytes  = 64 << 20
	agentModelProxyMaxResponseBytes = 64 << 20
)

var errAgentModelProxyPayloadTooLarge = errors.New("model proxy payload too large")

type AgentModelProxyGatewayCaller struct {
	baseURL    string
	httpClient *http.Client
}

func NewAgentModelProxyGatewayCaller(cfg *config.Config) *AgentModelProxyGatewayCaller {
	host := "127.0.0.1"
	port := 8080
	if cfg != nil {
		host = modelProxyLoopbackHost(cfg.Server.Host)
		if cfg.Server.Port > 0 {
			port = cfg.Server.Port
		}
	}
	return &AgentModelProxyGatewayCaller{
		baseURL: "http://" + net.JoinHostPort(host, strconv.Itoa(port)),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c *AgentModelProxyGatewayCaller) CallModelProxy(ctx context.Context, req ModelProxyRequest, apiKey *APIKey) (*ModelProxyResponse, error) {
	if stream, _ := req.Request["stream"].(bool); stream {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_STREAM_UNSUPPORTED", "streaming model proxy is not supported yet")
	}
	httpReq, endpoint, method, err := c.buildModelProxyHTTPRequest(ctx, req, apiKey, "")
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_REQUEST_FAILED", "model proxy request failed").WithCause(err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, agentModelProxyMaxResponseBytes+1))
	if err != nil {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_RESPONSE_READ_FAILED", "model proxy response read failed").WithCause(err)
	}
	if len(raw) > agentModelProxyMaxResponseBytes {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_RESPONSE_TOO_LARGE", "model proxy response exceeds the maximum supported size")
	}
	responseContentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	var payload map[string]any
	bodyBase64 := ""
	if len(raw) > 0 {
		if isJSONMediaType(responseContentType) || json.Valid(raw) {
			if err := json.Unmarshal(raw, &payload); err != nil {
				return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_RESPONSE_INVALID", "model proxy response is not valid json").WithCause(err)
			}
		} else {
			bodyBase64 = base64.StdEncoding.EncodeToString(raw)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, infraerrors.New(resp.StatusCode, "AGENT_MODEL_PROXY_UPSTREAM_ERROR", modelProxyErrorMessage(resp.StatusCode, payload, raw))
	}

	usage := map[string]any{}
	if value, ok := payload["usage"].(map[string]any); ok {
		usage = value
	}
	return &ModelProxyResponse{
		Response:    ensureMap(payload),
		Usage:       usage,
		Status:      resp.StatusCode,
		ContentType: responseContentType,
		BodyBase64:  bodyBase64,
		Headers:     modelProxyResponseHeaders(resp.Header),
		Metadata: map[string]any{
			"endpoint":    endpoint,
			"method":      method,
			"status_code": resp.StatusCode,
		},
	}, nil
}

func (c *AgentModelProxyGatewayCaller) CallModelProxyStream(ctx context.Context, req ModelProxyRequest, apiKey *APIKey) (*ModelProxyStreamResponse, error) {
	stream, _ := req.Request["stream"].(bool)
	if !stream {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_STREAM_REQUIRED", "streaming model proxy requires stream=true")
	}
	httpReq, endpoint, _, err := c.buildModelProxyHTTPRequest(ctx, req, apiKey, "text/event-stream")
	if err != nil {
		return nil, err
	}
	if endpoint != "/v1/chat/completions" && endpoint != "/v1/responses" {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_STREAM_ENDPOINT_UNSUPPORTED", "streaming is only supported for chat completions and responses")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_REQUEST_FAILED", "model proxy request failed").WithCause(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, agentModelProxyMaxResponseBytes+1))
		if readErr != nil {
			return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_RESPONSE_READ_FAILED", "model proxy response read failed").WithCause(readErr)
		}
		if len(raw) > agentModelProxyMaxResponseBytes {
			return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_RESPONSE_TOO_LARGE", "model proxy response exceeds the maximum supported size")
		}
		var payload map[string]any
		if json.Valid(raw) {
			_ = json.Unmarshal(raw, &payload)
		}
		return nil, infraerrors.New(resp.StatusCode, "AGENT_MODEL_PROXY_UPSTREAM_ERROR", modelProxyErrorMessage(resp.StatusCode, payload, raw))
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	if parseErr != nil || !strings.EqualFold(mediaType, "text/event-stream") {
		_ = resp.Body.Close()
		return nil, infraerrors.InternalServer("AGENT_MODEL_PROXY_STREAM_INVALID", "model proxy did not return an SSE stream")
	}
	return &ModelProxyStreamResponse{
		Status:      resp.StatusCode,
		ContentType: contentType,
		Headers:     modelProxyResponseHeaders(resp.Header),
		Body:        resp.Body,
	}, nil
}

func (c *AgentModelProxyGatewayCaller) buildModelProxyHTTPRequest(
	ctx context.Context,
	req ModelProxyRequest,
	apiKey *APIKey,
	accept string,
) (*http.Request, string, string, error) {
	if c == nil || c.httpClient == nil {
		return nil, "", "", infraerrors.InternalServer("AGENT_MODEL_PROXY_UNAVAILABLE", "model proxy is not configured")
	}
	if apiKey == nil || strings.TrimSpace(apiKey.Key) == "" {
		return nil, "", "", infraerrors.Forbidden("AGENT_MODEL_PROXY_API_KEY_UNAVAILABLE", "api key is unavailable")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return nil, "", "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MODEL_REQUIRED", "model is required")
	}
	endpoint := normalizeModelProxyEndpoint(req.Endpoint)
	if endpoint == "" {
		return nil, "", "", infraerrors.BadRequest("AGENT_MODEL_PROXY_ENDPOINT_UNSUPPORTED", "model proxy endpoint is unsupported")
	}
	method := normalizeModelProxyMethod(req.Method)
	if method == "" || (!isModelProxyMediaEndpoint(endpoint) && method != http.MethodPost) {
		return nil, "", "", infraerrors.BadRequest("AGENT_MODEL_PROXY_METHOD_UNSUPPORTED", "model proxy http method is unsupported")
	}
	body, contentType, err := buildModelProxyRequestBody(req, method)
	if err != nil {
		return nil, "", "", err
	}
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bodyReader)
	if err != nil {
		return nil, "", "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_INVALID", "model proxy request is invalid").WithCause(err)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		httpReq.Header.Set("Accept", accept)
	} else if isModelProxyMediaEndpoint(endpoint) {
		httpReq.Header.Set("Accept", "*/*")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey.Key)
	httpReq.Header.Set("User-Agent", "Sub2API-Agent-ModelProxy/1.0")
	httpReq.Header.Set("X-Sub2API-Agent-Run-ID", strconv.FormatInt(req.RunID, 10))
	if req.RunID > 0 {
		httpReq.Header.Set("session_id", "agent-run-"+strconv.FormatInt(req.RunID, 10))
	}
	httpReq.Header.Set("X-Sub2API-Model", req.Model)
	if appID := int64FromMap(req.Metadata, "agent_app_id"); appID > 0 {
		httpReq.Header.Set("X-Sub2API-Agent-App-ID", strconv.FormatInt(appID, 10))
	}
	if versionID := int64FromMap(req.Metadata, "agent_app_version_id"); versionID > 0 {
		httpReq.Header.Set("X-Sub2API-Agent-App-Version-ID", strconv.FormatInt(versionID, 10))
	}
	if nodeID := strings.TrimSpace(req.NodeID); nodeID != "" {
		httpReq.Header.Set("X-Sub2API-Agent-Node-ID", nodeID)
	}
	if role := strings.TrimSpace(req.Role); role != "" {
		httpReq.Header.Set("X-Sub2API-Agent-Node-Role", role)
	}
	if req.GroupID != nil {
		httpReq.Header.Set("X-Sub2API-Agent-Model-Group-ID", strconv.FormatInt(*req.GroupID, 10))
	}
	SignAgentModelProxyInternalRequest(httpReq)
	return httpReq, endpoint, method, nil
}

func normalizeModelProxyMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return http.MethodPost
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return method
	default:
		return ""
	}
}

func buildModelProxyRequestBody(req ModelProxyRequest, method string) ([]byte, string, error) {
	if req.BodyBase64 != "" && len(req.Multipart) > 0 {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_INVALID", "body_base64 and multipart cannot be used together")
	}
	if req.BodyBase64 != "" {
		body, err := decodeModelProxyBase64(req.BodyBase64, agentModelProxyMaxRequestBytes)
		if err != nil {
			if errors.Is(err, errAgentModelProxyPayloadTooLarge) {
				return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
			}
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_BODY_BASE64_INVALID", "model proxy body_base64 is invalid").WithCause(err)
		}
		contentType, err := normalizeModelProxyContentType(req.ContentType, "application/octet-stream")
		if err != nil {
			return nil, "", err
		}
		if isJSONMediaType(contentType) || json.Valid(body) {
			body, err = normalizeModelProxyJSONBody(body, req.Model)
			if err != nil {
				return nil, "", err
			}
			if !isJSONMediaType(contentType) {
				contentType = "application/json"
			}
		}
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if strings.EqualFold(mediaType, "multipart/form-data") || strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_BODY_ENCODING_UNSUPPORTED", "structured model proxy bodies must use request or multipart fields")
		}
		return body, contentType, nil
	}
	if len(req.Multipart) > 0 {
		return buildModelProxyMultipartBody(req)
	}
	if method == http.MethodGet || method == http.MethodDelete {
		return nil, "", nil
	}
	contentType, err := normalizeModelProxyContentType(req.ContentType, "application/json")
	if err != nil {
		return nil, "", err
	}
	if !isJSONMediaType(contentType) {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_CONTENT_TYPE_UNSUPPORTED", "non-json model proxy requests must use body_base64 or multipart")
	}
	body, err := json.Marshal(normalizeModelProxyJSONRequest(req.Request, req.Model))
	if err != nil {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_INVALID", "model proxy request is invalid").WithCause(err)
	}
	if len(body) > agentModelProxyMaxRequestBytes {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
	}
	return body, contentType, nil
}

func buildModelProxyMultipartBody(req ModelProxyRequest) ([]byte, string, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MODEL_REQUIRED", "model is required")
	}
	if req.ContentType != "" {
		contentType, err := normalizeModelProxyContentType(req.ContentType, "multipart/form-data")
		if err != nil {
			return nil, "", err
		}
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if !strings.EqualFold(mediaType, "multipart/form-data") {
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_CONTENT_TYPE_UNSUPPORTED", "multipart references require multipart/form-data")
		}
	}

	explicitNames := make(map[string]struct{}, len(req.Multipart))
	modelReferenceSeen := false
	for _, ref := range req.Multipart {
		name := strings.TrimSpace(ref.Name)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart field name is invalid")
		}
		if ref.BodyBase64 != "" && ref.Value != "" {
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart field cannot contain both value and body_base64")
		}
		if strings.EqualFold(name, "model") {
			if modelReferenceSeen {
				return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_MODEL_INVALID", "multipart model field must be unique")
			}
			modelReferenceSeen = true
			if ref.BodyBase64 != "" || strings.TrimSpace(ref.Filename) != "" {
				return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_MODEL_INVALID", "multipart model field must be plain text")
			}
			if contentType := strings.TrimSpace(ref.ContentType); contentType != "" {
				mediaType, _, err := mime.ParseMediaType(contentType)
				if err != nil || !strings.EqualFold(mediaType, "text/plain") {
					return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_MODEL_INVALID", "multipart model field must be plain text")
				}
			}
			if strings.TrimSpace(ref.Value) != model {
				return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MODEL_MISMATCH", "multipart model does not match the authorized model")
			}
			continue
		}
		explicitNames[name] = struct{}{}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeModelProxyMultipartPart(writer, "model", "", "", []byte(model)); err != nil {
		_ = writer.Close()
		return nil, "", err
	}
	requestKeys := make([]string, 0, len(req.Request))
	for key := range req.Request {
		requestKeys = append(requestKeys, key)
	}
	sort.Strings(requestKeys)
	for _, key := range requestKeys {
		name := strings.TrimSpace(key)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			_ = writer.Close()
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart request field name is invalid")
		}
		if strings.EqualFold(name, "model") {
			continue
		}
		if _, exists := explicitNames[name]; exists {
			continue
		}
		value, err := modelProxyMultipartFieldValue(req.Request[key])
		if err != nil {
			_ = writer.Close()
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart request field is invalid").WithCause(err)
		}
		if err := writeModelProxyMultipartPart(writer, name, "", "", value); err != nil {
			_ = writer.Close()
			return nil, "", err
		}
		if body.Len() > agentModelProxyMaxRequestBytes {
			_ = writer.Close()
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
		}
	}

	for _, ref := range req.Multipart {
		name := strings.TrimSpace(ref.Name)
		if strings.EqualFold(name, "model") {
			continue
		}
		filename := strings.TrimSpace(ref.Filename)
		if strings.ContainsAny(filename, "\r\n") {
			_ = writer.Close()
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart filename is invalid")
		}
		contentType := strings.TrimSpace(ref.ContentType)
		if contentType != "" {
			var err error
			contentType, err = normalizeModelProxyContentType(contentType, "")
			if err != nil {
				_ = writer.Close()
				return nil, "", err
			}
		}
		partBody := []byte(ref.Value)
		if ref.BodyBase64 != "" {
			remaining := agentModelProxyMaxRequestBytes - body.Len()
			if remaining < 0 {
				remaining = 0
			}
			decoded, err := decodeModelProxyBase64(ref.BodyBase64, remaining)
			if err != nil {
				_ = writer.Close()
				if errors.Is(err, errAgentModelProxyPayloadTooLarge) {
					return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
				}
				return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart body_base64 is invalid").WithCause(err)
			}
			partBody = decoded
		}
		if err := writeModelProxyMultipartPart(writer, name, filename, contentType, partBody); err != nil {
			_ = writer.Close()
			return nil, "", err
		}
		if body.Len() > agentModelProxyMaxRequestBytes {
			_ = writer.Close()
			return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart request is invalid").WithCause(err)
	}
	if body.Len() > agentModelProxyMaxRequestBytes {
		return nil, "", infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func normalizeModelProxyJSONRequest(request map[string]any, model string) map[string]any {
	normalized := make(map[string]any, len(request)+1)
	for key, value := range request {
		if strings.EqualFold(strings.TrimSpace(key), "model") {
			continue
		}
		normalized[key] = value
	}
	normalized["model"] = strings.TrimSpace(model)
	return normalized
}

func normalizeModelProxyJSONBody(body []byte, model string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		if err == nil {
			err = fmt.Errorf("json request body must be an object")
		}
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_INVALID", "json model proxy body must be an object").WithCause(err)
	}
	normalized, err := json.Marshal(normalizeModelProxyJSONRequest(payload, model))
	if err != nil {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_INVALID", "model proxy request is invalid").WithCause(err)
	}
	if len(normalized) > agentModelProxyMaxRequestBytes {
		return nil, infraerrors.BadRequest("AGENT_MODEL_PROXY_REQUEST_TOO_LARGE", "model proxy request exceeds the maximum supported size")
	}
	return normalized, nil
}

func writeModelProxyMultipartPart(writer *multipart.Writer, name, filename, contentType string, body []byte) error {
	header := make(textproto.MIMEHeader)
	disposition := `form-data; name="` + escapeModelProxyMultipartValue(name) + `"`
	if filename != "" {
		disposition += `; filename="` + escapeModelProxyMultipartValue(filename) + `"`
	}
	header.Set("Content-Disposition", disposition)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	} else if filename != "" {
		header.Set("Content-Type", "application/octet-stream")
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		return infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart request is invalid").WithCause(err)
	}
	if _, err := part.Write(body); err != nil {
		return infraerrors.BadRequest("AGENT_MODEL_PROXY_MULTIPART_INVALID", "multipart request is invalid").WithCause(err)
	}
	return nil
}

func modelProxyMultipartFieldValue(value any) ([]byte, error) {
	if text, ok := value.(string); ok {
		return []byte(text), nil
	}
	return json.Marshal(value)
}

func decodeModelProxyBase64(value string, maxBytes int) ([]byte, error) {
	if maxBytes < 0 || base64.StdEncoding.DecodedLen(len(value)) > maxBytes {
		return nil, fmt.Errorf("%w: decoded body exceeds %d bytes", errAgentModelProxyPayloadTooLarge, maxBytes)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(decoded) > maxBytes {
		return nil, fmt.Errorf("%w: decoded body exceeds %d bytes", errAgentModelProxyPayloadTooLarge, maxBytes)
	}
	return decoded, nil
}

func normalizeModelProxyContentType(contentType, defaultValue string) (string, error) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = defaultValue
	}
	if contentType == "" {
		return "", nil
	}
	if strings.ContainsAny(contentType, "\r\n") {
		return "", infraerrors.BadRequest("AGENT_MODEL_PROXY_CONTENT_TYPE_INVALID", "model proxy content_type is invalid")
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil {
		return "", infraerrors.BadRequest("AGENT_MODEL_PROXY_CONTENT_TYPE_INVALID", "model proxy content_type is invalid").WithCause(err)
	}
	return contentType, nil
}

func isJSONMediaType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func escapeModelProxyMultipartValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, `\"`).Replace(value)
}

func modelProxyResponseHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for name, values := range headers {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "set-cookie", "set-cookie2", "te", "trailer", "transfer-encoding", "upgrade", "www-authenticate":
			continue
		}
		if len(values) == 0 {
			continue
		}
		out[http.CanonicalHeaderKey(name)] = strings.Join(values, ", ")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func modelProxyLoopbackHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return host
	}
}

func normalizeModelProxyEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.Contains(endpoint, "://") || strings.ContainsAny(endpoint, "?#\\%") {
		return ""
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	switch endpoint {
	case "/chat/completions", "/v1/chat/completions":
		return "/v1/chat/completions"
	case "/responses", "/v1/responses":
		return "/v1/responses"
	case "/embeddings", "/v1/embeddings":
		return "/v1/embeddings"
	case "/images/generations", "/v1/images/generations":
		return "/v1/images/generations"
	case "/images/edits", "/v1/images/edits":
		return "/v1/images/edits"
	default:
		if strings.HasPrefix(endpoint, "/audio/") || strings.HasPrefix(endpoint, "/videos/") || endpoint == "/videos" {
			endpoint = "/v1" + endpoint
		}
		if isSafeModelProxyMediaEndpoint(endpoint) {
			return endpoint
		}
		return ""
	}
}

func isModelProxyMediaEndpoint(endpoint string) bool {
	return isSafeModelProxyMediaEndpoint(endpoint)
}

func isSafeModelProxyMediaEndpoint(endpoint string) bool {
	switch endpoint {
	case "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations":
		return true
	}
	if endpoint == "/v1/videos" {
		return true
	}
	const prefix = "/v1/videos/"
	if !strings.HasPrefix(endpoint, prefix) {
		return false
	}
	remainder := strings.TrimPrefix(endpoint, prefix)
	segments := strings.Split(remainder, "/")
	if len(segments) == 0 || len(segments) > 4 {
		return false
	}
	for _, segment := range segments {
		if !isSafeModelProxyPathSegment(segment) {
			return false
		}
	}
	return true
}

func isSafeModelProxyPathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	for _, char := range segment {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '-', '_', '.', '~', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func modelProxyErrorMessage(statusCode int, payload map[string]any, raw []byte) string {
	if errObj, ok := payload["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		return message
	}
	if len(raw) > 0 {
		text := strings.TrimSpace(string(raw))
		if len(text) > 500 {
			text = text[:500]
		}
		if text != "" {
			return text
		}
	}
	return fmt.Sprintf("model proxy upstream returned HTTP %d", statusCode)
}

func int64FromMap(values map[string]any, key string) int64 {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return int64(value)
	case int8:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint:
		return int64(value)
	case uint8:
		return int64(value)
	case uint16:
		return int64(value)
	case uint32:
		return int64(value)
	case uint64:
		if value > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(value)
	case float64:
		if value <= 0 || value != float64(int64(value)) {
			return 0
		}
		return int64(value)
	case json.Number:
		n, err := value.Int64()
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
