package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	creazyCanvasPlaybackTokenPurpose      = "creazy-canvas-playback"
	creazyCanvasPlaybackTokenVersion      = "v1"
	creazyCanvasPlaybackTokenTTL          = 10 * time.Minute
	creazyCanvasPlaybackMaxStreamsPerUser = 6
	creazyCanvasPlaybackIdleTimeout       = 2 * time.Minute
)

type creazyCanvasPlaybackClaims struct {
	WorkID    int64
	UserID    int64
	ExpiresAt time.Time
}

// GetPlaybackURL returns a browser-native video URL. Unlike the download
// helper, the signed fallback can be used directly by <video> without exposing
// either the user's JWT or API key.
func (s *CreazyCanvasService) GetPlaybackURL(ctx context.Context, userID, workID int64) (*CreazyCanvasDownloadURL, error) {
	work, err := s.creazyCanvasPlaybackWork(ctx, userID, workID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(work.ObjectKey) != "" && s.artifactStore != nil && s.artifactStore.IsConfigured() {
		url, err := s.artifactStore.PresignGetObject(ctx, AgentArtifactObjectLocation{
			StorageProvider: work.StorageProvider,
			Bucket:          work.Bucket,
			ObjectKey:       work.ObjectKey,
		}, creazyCanvasPlaybackTokenTTL)
		if err != nil {
			return nil, infraerrors.InternalServer("CREAZY_CANVAS_PLAYBACK_PRESIGN_FAILED", "Failed to create video playback URL: "+err.Error())
		}
		return &CreazyCanvasDownloadURL{
			WorkID:    work.ID,
			URL:       url,
			ExpiresAt: time.Now().UTC().Add(creazyCanvasPlaybackTokenTTL).Format(time.RFC3339),
			Source:    "object",
		}, nil
	}
	if creazyCanvasWorkHasGatewayContent(work) {
		expiresAt := time.Now().UTC().Add(creazyCanvasPlaybackTokenTTL)
		if !work.ExpiresAt.IsZero() && work.ExpiresAt.Before(expiresAt) {
			expiresAt = work.ExpiresAt.UTC()
		}
		if !time.Now().UTC().Before(expiresAt) {
			return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_EXPIRED", "Work has expired")
		}
		token, err := s.signCreazyCanvasPlaybackToken(work.ID, work.UserID, expiresAt)
		if err != nil {
			return nil, err
		}
		return &CreazyCanvasDownloadURL{
			WorkID:    work.ID,
			URL:       fmt.Sprintf("/creazy-canvas/works/%d/playback?token=%s", work.ID, url.QueryEscape(token)),
			ExpiresAt: expiresAt.Format(time.RFC3339),
			Source:    "playback",
		}, nil
	}
	if mediaURL := creazyCanvasPublicMediaURL(work); mediaURL != "" {
		return &CreazyCanvasDownloadURL{WorkID: work.ID, URL: mediaURL, Source: "object"}, nil
	}
	return nil, infraerrors.BadRequest("CREAZY_CANVAS_PLAYBACK_UNAVAILABLE", "Video playback is unavailable")
}

// OpenPlayback validates a short-lived work-bound token and opens a Range-aware
// stream. Gateway-backed videos use the playback marker so the first byte is
// not blocked by the lazy full-file archive.
func (s *CreazyCanvasService) OpenPlayback(ctx context.Context, workID int64, token, rangeHeader string) (*CreazyCanvasWorkContent, error) {
	claims, err := s.verifyCreazyCanvasPlaybackToken(token)
	if err != nil || claims.WorkID != workID {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	if _, err := s.creazyCanvasPlaybackWork(ctx, claims.UserID, claims.WorkID); err != nil {
		return nil, err
	}
	normalizedRange, err := normalizeCreazyCanvasPlaybackRange(rangeHeader)
	if err != nil {
		return nil, err
	}
	release, err := s.acquireCreazyCanvasPlaybackStream(claims.UserID)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			cancel()
			release()
		}
	}()

	content, err := s.openWorkContent(streamCtx, claims.UserID, claims.WorkID, normalizedRange, true)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_PLAYBACK_UNAVAILABLE", "Video playback is unavailable")
	}
	if content.RedirectURL != "" || content.Body == nil {
		return content, nil
	}
	if strings.TrimSpace(content.ContentType) == "" {
		content.ContentType = "video/mp4"
	}
	content.Body = newArtifactPreviewReadCloser(content.Body, creazyCanvasPlaybackIdleTimeout, cancel, release)
	releaseOnReturn = false
	return content, nil
}

func (s *CreazyCanvasService) creazyCanvasPlaybackWork(ctx context.Context, userID, workID int64) (*CreazyCanvasWork, error) {
	work, err := s.GetWork(ctx, userID, workID)
	if err != nil {
		return nil, err
	}
	if work.Kind != CreazyCanvasWorkKindVideo {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_PLAYBACK_KIND_INVALID", "Only video works support playback")
	}
	if work.Status == CreazyCanvasWorkStatusExpired || (!work.ExpiresAt.IsZero() && !time.Now().UTC().Before(work.ExpiresAt)) {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_EXPIRED", "Work has expired")
	}
	if work.Status != CreazyCanvasWorkStatusSucceeded {
		return nil, infraerrors.BadRequest("CREAZY_CANVAS_WORK_NOT_READY", "Video work is not ready")
	}
	return work, nil
}

func (s *CreazyCanvasService) signCreazyCanvasPlaybackToken(workID, userID int64, expiresAt time.Time) (string, error) {
	secret := s.creazyCanvasPlaybackSigningSecret()
	if secret == "" {
		return "", infraerrors.InternalServer("CREAZY_CANVAS_PLAYBACK_UNAVAILABLE", "Video playback signing is unavailable")
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", infraerrors.InternalServer("CREAZY_CANVAS_PLAYBACK_UNAVAILABLE", "Video playback signing is unavailable").WithCause(err)
	}
	payload := fmt.Sprintf("%s:%d:%d:%d:%s",
		creazyCanvasPlaybackTokenVersion,
		workID,
		userID,
		expiresAt.UTC().Unix(),
		base64.RawURLEncoding.EncodeToString(nonce),
	)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := signCreazyCanvasPlaybackPayload(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *CreazyCanvasService) verifyCreazyCanvasPlaybackToken(token string) (*creazyCanvasPlaybackClaims, error) {
	secret := s.creazyCanvasPlaybackSigningSecret()
	parts := strings.Split(strings.TrimSpace(token), ".")
	if secret == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(provided, signCreazyCanvasPlaybackPayload(secret, parts[0])) {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	fields := strings.Split(string(raw), ":")
	if len(fields) != 5 || fields[0] != creazyCanvasPlaybackTokenVersion {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	workID, workErr := strconv.ParseInt(fields[1], 10, 64)
	userID, userErr := strconv.ParseInt(fields[2], 10, 64)
	expiresUnix, expiresErr := strconv.ParseInt(fields[3], 10, 64)
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(fields[4])
	if workErr != nil || userErr != nil || expiresErr != nil || nonceErr != nil || len(nonce) != 12 || workID <= 0 || userID <= 0 {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	expiresAt := time.Unix(expiresUnix, 0).UTC()
	if !time.Now().UTC().Before(expiresAt) {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	return &creazyCanvasPlaybackClaims{WorkID: workID, UserID: userID, ExpiresAt: expiresAt}, nil
}

func (s *CreazyCanvasService) creazyCanvasPlaybackSigningSecret() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.JWT.Secret)
}

func signCreazyCanvasPlaybackPayload(secret, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(creazyCanvasPlaybackTokenPurpose))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}

func normalizeCreazyCanvasPlaybackRange(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 128 || !artifactPreviewRangePattern.MatchString(value) {
		return "", infraerrors.New(http.StatusRequestedRangeNotSatisfiable, "CREAZY_CANVAS_PLAYBACK_RANGE_INVALID", "Video playback range is invalid")
	}
	return value, nil
}

func invalidCreazyCanvasPlaybackTokenError() error {
	return infraerrors.Unauthorized("CREAZY_CANVAS_PLAYBACK_TOKEN_INVALID", "Video playback token is invalid or expired")
}

func (s *CreazyCanvasService) acquireCreazyCanvasPlaybackStream(userID int64) (func(), error) {
	if s == nil || userID <= 0 {
		return nil, invalidCreazyCanvasPlaybackTokenError()
	}
	s.playbackMu.Lock()
	if s.playbackStreams == nil {
		s.playbackStreams = make(map[int64]int)
	}
	if s.playbackStreams[userID] >= creazyCanvasPlaybackMaxStreamsPerUser {
		s.playbackMu.Unlock()
		return nil, infraerrors.TooManyRequests("CREAZY_CANVAS_PLAYBACK_CONCURRENCY_LIMIT", "Too many concurrent video previews")
	}
	s.playbackStreams[userID]++
	s.playbackMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.playbackMu.Lock()
			remaining := s.playbackStreams[userID] - 1
			if remaining <= 0 {
				delete(s.playbackStreams, userID)
			} else {
				s.playbackStreams[userID] = remaining
			}
			s.playbackMu.Unlock()
		})
	}, nil
}
