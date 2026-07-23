package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	artifactPreviewTokenPurpose      = "agent-artifact-preview"
	artifactPreviewTokenVersion      = "v1"
	artifactPreviewTokenTTL          = 10 * time.Minute
	artifactPreviewMaxStreamsPerUser = 6
	artifactPreviewIdleReadTimeout   = 2 * time.Minute
)

var artifactPreviewRangePattern = regexp.MustCompile(`^bytes=(?:\d+-\d*|-\d+)$`)

type artifactPreviewTokenClaims struct {
	ArtifactID int64
	UserID     int64
	ExpiresAt  time.Time
}

type ArtifactPreviewContent struct {
	StatusCode    int
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	ContentRange  string
	AcceptRanges  string
	ETag          string
	LastModified  string
}

func (s *AgentRunService) GetArtifactPreviewURL(ctx context.Context, artifactID, userID int64) (*ArtifactDownloadURL, error) {
	artifact, err := s.artifactForPreview(ctx, artifactID, userID)
	if err != nil {
		return nil, err
	}
	if _, ok := artifactPreviewMediaType(artifact); !ok {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_PREVIEW_TYPE_UNSUPPORTED", "artifact type does not support inline preview")
	}
	expiresAt := s.artifactPreviewExpiresAt(artifact)
	if artifactUsesExternalPreviewURL(artifact) {
		return &ArtifactDownloadURL{ArtifactID: artifact.ID, URL: artifact.ObjectURL, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
	}
	if s.artifactStore == nil || !s.artifactStore.IsConfigured() || strings.TrimSpace(artifact.ObjectKey) == "" {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	token, err := s.signArtifactPreviewToken(artifact.ID, artifact.UserID, expiresAt)
	if err != nil {
		return nil, err
	}
	previewPath := fmt.Sprintf("/agent-artifacts/%d/content?token=%s", artifact.ID, url.QueryEscape(token))
	return &ArtifactDownloadURL{ArtifactID: artifact.ID, URL: previewPath, ExpiresAt: expiresAt.Format(time.RFC3339)}, nil
}

func (s *AgentRunService) OpenArtifactPreview(ctx context.Context, artifactID int64, token, rangeHeader string) (*ArtifactPreviewContent, error) {
	claims, err := s.verifyArtifactPreviewToken(token)
	if err != nil || claims.ArtifactID != artifactID {
		return nil, invalidArtifactPreviewTokenError()
	}
	artifact, err := s.artifactForPreview(ctx, claims.ArtifactID, claims.UserID)
	if err != nil {
		return nil, err
	}
	contentType, ok := artifactPreviewMediaType(artifact)
	if !ok {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_PREVIEW_TYPE_UNSUPPORTED", "artifact type does not support inline preview")
	}
	if artifactUsesExternalPreviewURL(artifact) || s.artifactStore == nil || !s.artifactStore.IsConfigured() {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_PREVIEW_UNAVAILABLE", "artifact preview is unavailable")
	}
	normalizedRange, err := normalizeArtifactPreviewRange(rangeHeader)
	if err != nil {
		return nil, err
	}
	releaseStream, err := s.acquireArtifactPreviewStream(claims.UserID)
	if err != nil {
		return nil, err
	}
	upstreamCtx, cancelUpstream := context.WithCancel(ctx)
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			cancelUpstream()
			releaseStream()
		}
	}()
	ttl := time.Until(claims.ExpiresAt)
	if ttl <= 0 {
		return nil, invalidArtifactPreviewTokenError()
	}
	signedURL, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{
		StorageProvider: artifact.StorageProvider,
		Bucket:          artifact.Bucket,
		ObjectKey:       artifact.ObjectKey,
	}, ttl)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(upstreamCtx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "AGENT_ARTIFACT_PREVIEW_REQUEST_FAILED", "artifact preview request failed").WithCause(err)
	}
	if normalizedRange != "" {
		req.Header.Set("Range", normalizedRange)
	}
	req.Header.Set("Accept-Encoding", "identity")
	client := s.previewClient
	if client == nil {
		client = newArtifactPreviewHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "AGENT_ARTIFACT_PREVIEW_REQUEST_FAILED", "artifact preview request failed").WithCause(err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		cancelUpstream()
		releaseStream()
		releaseOnReturn = false
		_ = resp.Body.Close()
		return nil, infraerrors.New(http.StatusBadGateway, "AGENT_ARTIFACT_PREVIEW_UPSTREAM_ERROR", "artifact preview storage request failed")
	}
	acceptRanges := strings.TrimSpace(resp.Header.Get("Accept-Ranges"))
	if acceptRanges == "" {
		acceptRanges = "bytes"
	}
	releaseOnReturn = false
	return &ArtifactPreviewContent{
		StatusCode:    resp.StatusCode,
		Body:          newArtifactPreviewReadCloser(resp.Body, artifactPreviewIdleReadTimeout, cancelUpstream, releaseStream),
		ContentType:   contentType,
		ContentLength: resp.ContentLength,
		ContentRange:  strings.TrimSpace(resp.Header.Get("Content-Range")),
		AcceptRanges:  acceptRanges,
		ETag:          strings.TrimSpace(resp.Header.Get("ETag")),
		LastModified:  strings.TrimSpace(resp.Header.Get("Last-Modified")),
	}, nil
}

func (s *AgentRunService) artifactPreviewExpiresAt(artifact *AgentArtifact) time.Time {
	now := time.Now().UTC()
	ttl := artifactPreviewTokenTTL
	if downloadTTL := s.artifactDownloadTTL(); downloadTTL > 0 && downloadTTL < ttl {
		ttl = downloadTTL
	}
	expiresAt := now.Add(ttl)
	if artifact != nil && artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(expiresAt) {
		expiresAt = artifact.ExpiresAt.UTC()
	}
	return expiresAt
}

func (s *AgentRunService) acquireArtifactPreviewStream(userID int64) (func(), error) {
	if s == nil || userID <= 0 {
		return nil, invalidArtifactPreviewTokenError()
	}
	s.previewMu.Lock()
	if s.previewActiveStreams == nil {
		s.previewActiveStreams = make(map[int64]int)
	}
	if s.previewActiveStreams[userID] >= artifactPreviewMaxStreamsPerUser {
		s.previewMu.Unlock()
		return nil, infraerrors.TooManyRequests("AGENT_ARTIFACT_PREVIEW_CONCURRENCY_LIMIT", "too many concurrent artifact previews")
	}
	s.previewActiveStreams[userID]++
	s.previewMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.previewMu.Lock()
			remaining := s.previewActiveStreams[userID] - 1
			if remaining <= 0 {
				delete(s.previewActiveStreams, userID)
			} else {
				s.previewActiveStreams[userID] = remaining
			}
			s.previewMu.Unlock()
		})
	}, nil
}

type artifactPreviewReadCloser struct {
	io.ReadCloser
	cancel       context.CancelFunc
	release      func()
	idleTimeout  time.Duration
	mu           sync.Mutex
	timer        *time.Timer
	lastActivity time.Time
	closed       bool
	closeOnce    sync.Once
	closeErr     error
}

func newArtifactPreviewReadCloser(body io.ReadCloser, idleTimeout time.Duration, cancel context.CancelFunc, release func()) *artifactPreviewReadCloser {
	c := &artifactPreviewReadCloser{
		ReadCloser:   body,
		cancel:       cancel,
		release:      release,
		idleTimeout:  idleTimeout,
		lastActivity: time.Now(),
	}
	if idleTimeout > 0 {
		c.timer = time.AfterFunc(idleTimeout, c.handleIdleTimeout)
	}
	return c
}

func (c *artifactPreviewReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		c.mu.Lock()
		if !c.closed {
			c.lastActivity = time.Now()
			if c.timer != nil {
				c.timer.Reset(c.idleTimeout)
			}
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *artifactPreviewReadCloser) handleIdleTimeout() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	remaining := c.idleTimeout - time.Since(c.lastActivity)
	if remaining > 0 {
		c.timer.Reset(remaining)
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *artifactPreviewReadCloser) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		if c.timer != nil {
			c.timer.Stop()
		}
		c.mu.Unlock()
		if c.cancel != nil {
			c.cancel()
		}
		if c.release != nil {
			c.release()
		}
		c.closeErr = c.ReadCloser.Close()
	})
	return c.closeErr
}

func (s *AgentRunService) artifactForPreview(ctx context.Context, artifactID, userID int64) (*AgentArtifact, error) {
	if artifactID <= 0 {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_ID_INVALID", "artifact id invalid")
	}
	artifact, err := s.runRepo.GetArtifactByID(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.UserID != userID || artifact.DeletedAt != nil {
		return nil, ErrAgentArtifactNotFound
	}
	if artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(time.Now().UTC()) {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_EXPIRED", "artifact has expired")
	}
	return artifact, nil
}

func (s *AgentRunService) signArtifactPreviewToken(artifactID, userID int64, expiresAt time.Time) (string, error) {
	secret := s.artifactPreviewSigningSecret()
	if secret == "" {
		return "", infraerrors.InternalServer("AGENT_ARTIFACT_PREVIEW_UNAVAILABLE", "artifact preview signing is unavailable")
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", infraerrors.InternalServer("AGENT_ARTIFACT_PREVIEW_UNAVAILABLE", "artifact preview signing is unavailable").WithCause(err)
	}
	payload := fmt.Sprintf(
		"%s:%d:%d:%d:%s",
		artifactPreviewTokenVersion,
		artifactID,
		userID,
		expiresAt.UTC().Unix(),
		base64.RawURLEncoding.EncodeToString(nonce),
	)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := signArtifactPreviewPayload(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *AgentRunService) verifyArtifactPreviewToken(token string) (*artifactPreviewTokenClaims, error) {
	secret := s.artifactPreviewSigningSecret()
	parts := strings.Split(strings.TrimSpace(token), ".")
	if secret == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, invalidArtifactPreviewTokenError()
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(providedSignature, signArtifactPreviewPayload(secret, parts[0])) {
		return nil, invalidArtifactPreviewTokenError()
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, invalidArtifactPreviewTokenError()
	}
	fields := strings.Split(string(rawPayload), ":")
	if len(fields) != 5 || fields[0] != artifactPreviewTokenVersion {
		return nil, invalidArtifactPreviewTokenError()
	}
	artifactID, artifactErr := strconv.ParseInt(fields[1], 10, 64)
	userID, userErr := strconv.ParseInt(fields[2], 10, 64)
	expiresUnix, expiresErr := strconv.ParseInt(fields[3], 10, 64)
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(fields[4])
	if artifactErr != nil || userErr != nil || expiresErr != nil || nonceErr != nil || len(nonce) != 12 || artifactID <= 0 || userID <= 0 {
		return nil, invalidArtifactPreviewTokenError()
	}
	expiresAt := time.Unix(expiresUnix, 0).UTC()
	if !time.Now().UTC().Before(expiresAt) {
		return nil, invalidArtifactPreviewTokenError()
	}
	return &artifactPreviewTokenClaims{ArtifactID: artifactID, UserID: userID, ExpiresAt: expiresAt}, nil
}

func (s *AgentRunService) artifactPreviewSigningSecret() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.JWT.Secret)
}

func signArtifactPreviewPayload(secret, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(artifactPreviewTokenPurpose))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}

func normalizeArtifactPreviewRange(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 128 || !artifactPreviewRangePattern.MatchString(value) {
		return "", infraerrors.New(http.StatusRequestedRangeNotSatisfiable, "AGENT_ARTIFACT_PREVIEW_RANGE_INVALID", "artifact preview range is invalid")
	}
	return value, nil
}

func artifactPreviewMediaType(artifact *AgentArtifact) (string, bool) {
	if artifact == nil {
		return "", false
	}
	values := []string{
		artifact.MimeType,
		mime.TypeByExtension(strings.ToLower(filepath.Ext(strings.TrimSpace(artifact.Name)))),
	}
	for _, value := range values {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err == nil && (strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "audio/")) {
			return mediaType, true
		}
	}
	return "", false
}

func invalidArtifactPreviewTokenError() error {
	return infraerrors.Unauthorized("AGENT_ARTIFACT_PREVIEW_TOKEN_INVALID", "artifact preview token is invalid or expired")
}

func artifactUsesExternalPreviewURL(artifact *AgentArtifact) bool {
	if artifact == nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(artifact.StorageProvider), "external") || strings.TrimSpace(artifact.ObjectKey) == ""
}

func newArtifactPreviewHTTPClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Transport: http.DefaultTransport}
	}
	transport := base.Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: transport}
}
