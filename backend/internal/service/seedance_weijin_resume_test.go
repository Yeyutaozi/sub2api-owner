package service

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type weijinResumeHTTPUpstreamStub struct {
	mu        sync.Mutex
	responses []func(*http.Request) *http.Response
	requests  []*http.Request
}

func (s *weijinResumeHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if len(s.responses) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	factory := s.responses[0]
	s.responses = s.responses[1:]
	return factory(req), nil
}

func (s *weijinResumeHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func newWeijinResumeTestAccount() *Account {
	return &Account{
		ID:       801,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":       DefaultWeijinVideoBaseURL,
			"api_key":        "upstream-secret",
			"video_provider": VideoProviderWeijin,
		},
	}
}

func TestWeijinContentResumesShortInitialResponse(t *testing.T) {
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{"10"},
					},
					Body: io.NopCloser(strings.NewReader("abcd")),
				}
			},
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=4-9", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{"6"},
						"Content-Range":  []string{"bytes 4-9/10"},
					},
					Body: io.NopCloser(strings.NewReader("efghij")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "")
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "10", response.Header.Get("Content-Length"))
	body, err := io.ReadAll(response.BodyStream)
	require.NoError(t, err)
	require.Equal(t, "abcdefghij", string(body))
	require.NoError(t, response.BodyStream.Close())

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "", upstream.requests[0].Header.Get("Range"))
}

func TestWeijinContentRejectsMismatchedResumeRange(t *testing.T) {
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{"10"},
					},
					Body: io.NopCloser(strings.NewReader("abcd")),
				}
			},
			func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"6"},
						"Content-Range":  []string{"bytes 3-9/10"},
					},
					Body: io.NopCloser(strings.NewReader("defghi")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "")
	require.NoError(t, err)
	_, readErr := io.ReadAll(response.BodyStream)
	require.Error(t, readErr)
	require.Contains(t, readErr.Error(), "invalid byte range")
	require.NoError(t, response.BodyStream.Close())
}

func TestWeijinContentResumeUsesBoundedRanges(t *testing.T) {
	const total = int64(600000)
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{strconv.FormatInt(total, 10)},
					},
					Body: io.NopCloser(strings.NewReader("a")),
				}
			},
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=1-524288", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"524288"},
						"Content-Range":  []string{"bytes 1-524288/600000"},
					},
					Body: io.NopCloser(strings.NewReader("b")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "")
	require.NoError(t, err)
	_, readErr := io.ReadAll(response.BodyStream)
	require.Error(t, readErr)
	require.NoError(t, response.BodyStream.Close())
}

func TestWeijinContentDoesNotWrapRangeIgnored200(t *testing.T) {
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=0-3", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{"10"},
					},
					Body: io.NopCloser(strings.NewReader("abcdefghij")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "bytes=0-3")
	require.NoError(t, err)
	body, err := io.ReadAll(response.BodyStream)
	require.NoError(t, err)
	require.Equal(t, "abcdefghij", string(body))
	require.NoError(t, response.BodyStream.Close())

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
}

func TestWeijinContentResumeOpenEndedRange(t *testing.T) {
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=0-", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"4"},
						"Content-Range":  []string{"bytes 0-3/10"},
					},
					Body: io.NopCloser(strings.NewReader("abcd")),
				}
			},
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=4-9", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"6"},
						"Content-Range":  []string{"bytes 4-9/10"},
					},
					Body: io.NopCloser(strings.NewReader("efghij")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "bytes=0-")
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, response.StatusCode)
	require.Equal(t, "bytes 0-9/10", response.Header.Get("Content-Range"))
	require.Equal(t, "10", response.Header.Get("Content-Length"))
	body, err := io.ReadAll(response.BodyStream)
	require.NoError(t, err)
	require.Equal(t, "abcdefghij", string(body))
}

func TestWeijinContentNormalizesPartialInitialHeaders(t *testing.T) {
	upstream := &weijinResumeHTTPUpstreamStub{
		responses: []func(*http.Request) *http.Response{
			func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"4"},
						"Content-Range":  []string{"bytes 0-3/10"},
					},
					Body: io.NopCloser(strings.NewReader("abcd")),
				}
			},
			func(req *http.Request) *http.Response {
				require.Equal(t, "bytes=4-9", req.Header.Get("Range"))
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header: http.Header{
						"Content-Length": []string{"6"},
						"Content-Range":  []string{"bytes 4-9/10"},
					},
					Body: io.NopCloser(strings.NewReader("efghij")),
				}
			},
		},
	}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	response, err := service.ForwardSeedanceContent(context.Background(), nil, newWeijinResumeTestAccount(), "task_resume", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "10", response.Header.Get("Content-Length"))
	require.Empty(t, response.Header.Get("Content-Range"))
	body, err := io.ReadAll(response.BodyStream)
	require.NoError(t, err)
	require.Equal(t, "abcdefghij", string(body))
}
