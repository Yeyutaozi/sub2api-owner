package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentModelProxyGatewayCallerPropagatesAgentUsageHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "Bearer sk-user-platform-key", r.Header.Get("Authorization"))
		require.Equal(t, "Sub2API-Agent-ModelProxy/1.0", r.Header.Get("User-Agent"))
		require.Equal(t, "11", r.Header.Get("X-Sub2API-Agent-App-ID"))
		require.Equal(t, "22", r.Header.Get("X-Sub2API-Agent-App-Version-ID"))
		require.Equal(t, "33", r.Header.Get("X-Sub2API-Agent-Run-ID"))
		require.Equal(t, "agent-run-33", r.Header.Get("session_id"))
		require.Equal(t, "gpt-5.1", r.Header.Get("X-Sub2API-Model"))
		require.Equal(t, "node-1", r.Header.Get("X-Sub2API-Agent-Node-ID"))
		require.Equal(t, "llm", r.Header.Get("X-Sub2API-Agent-Node-Role"))
		var upstreamRequest map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&upstreamRequest))
		require.Equal(t, "gpt-5.1", upstreamRequest["model"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-5.1",
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens":     3,
				"completion_tokens": 4,
				"total_tokens":      7,
			},
		})
	}))
	defer server.Close()

	caller := &AgentModelProxyGatewayCaller{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	resp, err := caller.CallModelProxy(context.Background(), ModelProxyRequest{
		RunID:    33,
		Endpoint: "/chat/completions",
		NodeID:   "node-1",
		Role:     "llm",
		Model:    "gpt-5.1",
		Request: map[string]any{
			"model": "unauthorized-model",
			"messages": []any{
				map[string]any{"role": "user", "content": "hello"},
			},
		},
		Metadata: map[string]any{
			"agent_app_id":         int64(11),
			"agent_app_version_id": json.Number("22"),
		},
	}, &APIKey{Key: "sk-user-platform-key"})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "application/json", resp.ContentType)
	require.Empty(t, resp.BodyBase64)
	require.Equal(t, "/v1/chat/completions", resp.Metadata["endpoint"])
	require.Equal(t, http.MethodPost, resp.Metadata["method"])
}

func TestAgentModelProxyGatewayCallerSupportsRawMediaBodiesAndBinaryResponses(t *testing.T) {
	requestBody := []byte(`{"model":"unauthorized-tts","input":"hello"}`)
	responseBody := []byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x00}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/audio/speech", r.URL.Path)
		require.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
		require.Equal(t, "*/*", r.Header.Get("Accept"))
		require.Equal(t, "agent-run-44", r.Header.Get("session_id"))
		require.Equal(t, "tts-1", r.Header.Get("X-Sub2API-Model"))
		var upstreamRequest map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&upstreamRequest))
		require.Equal(t, "tts-1", upstreamRequest["model"])
		require.Equal(t, "hello", upstreamRequest["input"])

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `attachment; filename="speech.mp3"`)
		w.Header().Set("X-Request-ID", "req-audio-1")
		w.Header().Set("Set-Cookie", "secret=value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	caller := &AgentModelProxyGatewayCaller{baseURL: server.URL, httpClient: server.Client()}
	resp, err := caller.CallModelProxy(context.Background(), ModelProxyRequest{
		RunID:       44,
		Model:       "tts-1",
		Endpoint:    "/audio/speech",
		Method:      "post",
		ContentType: "application/json; charset=utf-8",
		BodyBase64:  base64.StdEncoding.EncodeToString(requestBody),
	}, &APIKey{Key: "sk-media"})

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.Status)
	require.Equal(t, "audio/mpeg", resp.ContentType)
	require.Equal(t, base64.StdEncoding.EncodeToString(responseBody), resp.BodyBase64)
	require.Empty(t, resp.Response)
	require.Equal(t, `attachment; filename="speech.mp3"`, resp.Headers["Content-Disposition"])
	require.Equal(t, "req-audio-1", resp.Headers["X-Request-Id"])
	require.NotContains(t, resp.Headers, "Set-Cookie")
}

func TestAgentModelProxyGatewayCallerBuildsMultipartFromRequestAndReferences(t *testing.T) {
	audioBody := []byte("fake-wave-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/audio/transcriptions", r.URL.Path)
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data; boundary=")
		require.NoError(t, r.ParseMultipartForm(agentModelProxyMaxRequestBytes))
		require.Equal(t, "whisper-1", r.FormValue("model"))
		require.Len(t, r.MultipartForm.Value["model"], 1)
		require.Equal(t, "zh", r.FormValue("language"))

		file, fileHeader, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()
		require.Equal(t, "sample.wav", fileHeader.Filename)
		require.Equal(t, "audio/wav", fileHeader.Header.Get("Content-Type"))
		actualAudio, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, audioBody, actualAudio)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":  "hello",
			"usage": map[string]any{"input_tokens": 8},
		})
	}))
	defer server.Close()

	caller := &AgentModelProxyGatewayCaller{baseURL: server.URL, httpClient: server.Client()}
	resp, err := caller.CallModelProxy(context.Background(), ModelProxyRequest{
		RunID:       55,
		Model:       "whisper-1",
		Endpoint:    "/v1/audio/transcriptions",
		ContentType: "multipart/form-data",
		Request: map[string]any{
			"model":    "unauthorized-whisper",
			"language": "en",
		},
		Multipart: []ModelProxyMultipartReference{
			{Name: "model", Value: "whisper-1", ContentType: "text/plain"},
			{Name: "language", Value: "zh"},
			{
				Name:        "file",
				Filename:    "sample.wav",
				ContentType: "audio/wav",
				BodyBase64:  base64.StdEncoding.EncodeToString(audioBody),
			},
		},
	}, &APIKey{Key: "sk-media"})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "hello", resp.Response["text"])
	require.Equal(t, float64(8), resp.Usage["input_tokens"])
}

func TestAgentModelProxyGatewayCallerSupportsVideoStatusMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/videos/video_123", r.URL.Path)
		require.Empty(t, r.Header.Get("Content-Type"))
		require.Equal(t, "agent-run-66", r.Header.Get("session_id"))
		require.Equal(t, "video-model-1", r.Header.Get("X-Sub2API-Model"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_123", "status": "completed"})
	}))
	defer server.Close()

	caller := &AgentModelProxyGatewayCaller{baseURL: server.URL, httpClient: server.Client()}
	resp, err := caller.CallModelProxy(context.Background(), ModelProxyRequest{
		RunID:    66,
		Model:    "video-model-1",
		Endpoint: "/videos/video_123",
		Method:   "GET",
		Request:  map[string]any{"model": "video-model-1"},
	}, &APIKey{Key: "sk-media"})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "completed", resp.Response["status"])
}

func TestNormalizeModelProxyEndpointSupportsRestrictedMediaPaths(t *testing.T) {
	tests := map[string]string{
		"/audio/speech":                   "/v1/audio/speech",
		"v1/audio/transcriptions":         "/v1/audio/transcriptions",
		"/v1/audio/translations":          "/v1/audio/translations",
		"/videos":                         "/v1/videos",
		"/v1/videos/video_123":            "/v1/videos/video_123",
		"/v1/videos/video_123/content":    "/v1/videos/video_123/content",
		"/v1/images/edits":                "/v1/images/edits",
		"/v1/video/generations/job_123":   "",
		"https://example.com/v1/videos":   "",
		"/v1/videos/../admin":             "",
		"/v1/videos/video_123?download=1": "",
		"/v1/videos/a/b/c/d/e":            "",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeModelProxyEndpoint(input))
		})
	}
}

func TestAgentModelProxyGatewayCallerRejectsInvalidMediaPayloads(t *testing.T) {
	caller := &AgentModelProxyGatewayCaller{
		baseURL:    "http://127.0.0.1:1",
		httpClient: http.DefaultClient,
	}
	apiKey := &APIKey{Key: "sk-media"}

	_, err := caller.CallModelProxy(context.Background(), ModelProxyRequest{
		Model:      "tts-1",
		Endpoint:   "/v1/audio/speech",
		Method:     http.MethodTrace,
		BodyBase64: base64.StdEncoding.EncodeToString([]byte("body")),
	}, apiKey)
	require.Error(t, err)

	_, err = caller.CallModelProxy(context.Background(), ModelProxyRequest{
		Model:      "tts-1",
		Endpoint:   "/v1/audio/speech",
		BodyBase64: "not-base64",
	}, apiKey)
	require.Error(t, err)

	_, err = caller.CallModelProxy(context.Background(), ModelProxyRequest{
		Model:      "whisper-1",
		Endpoint:   "/v1/audio/transcriptions",
		BodyBase64: base64.StdEncoding.EncodeToString([]byte("body")),
		Multipart:  []ModelProxyMultipartReference{{Name: "file", Value: "value"}},
	}, apiKey)
	require.Error(t, err)

	_, err = caller.CallModelProxy(context.Background(), ModelProxyRequest{
		Model:       "whisper-1",
		Endpoint:    "/v1/audio/transcriptions",
		ContentType: "multipart/form-data; boundary=opaque",
		BodyBase64:  base64.StdEncoding.EncodeToString([]byte("--opaque\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nunauthorized-whisper\r\n--opaque--\r\n")),
	}, apiKey)
	require.Error(t, err)
}

func TestAgentModelProxyGatewayCallerRejectsMultipartModelOverrides(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	caller := &AgentModelProxyGatewayCaller{baseURL: server.URL, httpClient: server.Client()}
	apiKey := &APIKey{Key: "sk-media"}
	baseRequest := ModelProxyRequest{
		Model:    "whisper-1",
		Endpoint: "/v1/audio/transcriptions",
	}

	mismatch := baseRequest
	mismatch.Multipart = []ModelProxyMultipartReference{{Name: "model", Value: "unauthorized-whisper"}}
	_, err := caller.CallModelProxy(context.Background(), mismatch, apiKey)
	require.Error(t, err)

	encoded := baseRequest
	encoded.Multipart = []ModelProxyMultipartReference{{
		Name:       "model",
		BodyBase64: base64.StdEncoding.EncodeToString([]byte("whisper-1")),
	}}
	_, err = caller.CallModelProxy(context.Background(), encoded, apiKey)
	require.Error(t, err)

	duplicate := baseRequest
	duplicate.Multipart = []ModelProxyMultipartReference{
		{Name: "model", Value: "whisper-1"},
		{Name: "MODEL", Value: "whisper-1"},
	}
	_, err = caller.CallModelProxy(context.Background(), duplicate, apiKey)
	require.Error(t, err)
	require.Zero(t, calls)
}
