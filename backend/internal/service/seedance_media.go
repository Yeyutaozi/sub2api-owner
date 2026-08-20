package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	_ "golang.org/x/image/webp"
)

const (
	SeedanceMaxImageBytes         int64 = 30_000_000 // align with Huiqu/MX933 upstream single-image limit
	SeedanceMaxImagePixels              = 40_000_000
	SeedanceMaxImageDimension           = 8192
	SeedanceUploadBodyOverhead    int64 = 2 << 20
	seedanceDefaultVideoBytes     int64 = 512 << 20
	seedanceUploadRecordTTL             = 24 * time.Hour
	seedanceOutputRecordTTL             = 7 * 24 * time.Hour
	seedanceDefaultPresignTTL           = time.Hour
	seedanceImageFetchTimeout           = 60 * time.Second
	seedanceOutputArchiveLockTTL        = 30 * time.Minute
	seedanceObjectCleanupTimeout        = 10 * time.Second
	seedanceMaxConcurrentArchives       = 2
	// Media concurrency is an internal processing limit, separate from the
	// user's submission concurrency. Queued requests wait for a slot instead
	// of immediately surfacing a misleading 429 to end users.
	seedanceMaxMediaConcurrency       = 100
	seedanceMediaAcquireWait          = 30 * time.Second
	seedanceMediaAcquireRetry         = 250 * time.Millisecond
	seedanceMediaLeaseTTL             = 5 * time.Minute
	seedanceMediaLeaseRefreshInterval = time.Minute
	seedanceMaxProcessMediaStreams    = 32
)

const (
	seedanceUploadRecordPrefix      = "seedance:media:upload:"
	seedanceOutputRecordPrefix      = "seedance:media:output:"
	seedanceOutputLockPrefix        = "seedance:media:archive-lock:"
	seedanceMediaIOPrefix           = "seedance:media:io:"
	seedancePublicMediaRecordPrefix = "seedance:media:public:"
	seedancePublicMediaTTL          = 2 * time.Hour
	// Public rehost strategy for Lingdong reference media:
	// 1) temporary third-party hosts (litterbox/catbox/0x0) when reachable
	// 2) gateway public-media as CN-production fallback (Lingdong keeps these URLs)
	// 3) never send bare COS/S3/OSS object URLs (Lingdong silently drops them)
	seedanceLingdongRehostURL       = "https://litterbox.catbox.moe/resources/internals/api.php"
	seedanceLingdongRehostURLCatbox = "https://catbox.moe/user/api.php"
	seedanceLingdongRehostURLAlt    = "https://0x0.st"
	seedanceLingdongRehostTTL       = "24h"
	seedanceLingdongRehostTimeout   = 12 * time.Second // per-host fail-fast; public-media is production fallback
)

const (
	seedanceStoredMediaStartFrame = "start_frame"
	seedanceStoredMediaEndFrame   = "end_frame"
	seedanceStoredMediaImage      = "image_reference"
	seedanceStoredMediaVideo      = "video_reference"
	seedanceStoredMediaAudio      = "audio_reference"
)

var ErrSeedanceOutputArchiveInProgress = errors.New("Seedance output archive is already in progress")

var seedanceTempDirOnce sync.Once
var seedanceTempDirPath string

var seedanceReleaseArchiveLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var seedanceAcquireMediaIOScript = redis.NewScript(`
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
if redis.call("ZCARD", KEYS[1]) >= tonumber(ARGV[2]) then
  return 0
end
redis.call("ZADD", KEYS[1], ARGV[3], ARGV[4])
redis.call("PEXPIRE", KEYS[1], ARGV[5])
return 1
`)

var seedanceRefreshMediaIOScript = redis.NewScript(`
if redis.call("ZSCORE", KEYS[1], ARGV[1]) then
  redis.call("ZADD", KEYS[1], ARGV[2], ARGV[1])
  redis.call("PEXPIRE", KEYS[1], ARGV[3])
  return 1
end
return 0
`)

var seedanceBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

type SeedanceMediaOwner struct {
	UserID   int64
	APIKeyID int64
	GroupID  int64
}

type SeedanceImageUploadInput struct {
	Owner       SeedanceMediaOwner
	Body        io.Reader
	SizeBytes   int64
	ContentType string
	Persistent  bool
	Filename    string
	MediaKind   string
	// SkipSizeLimit is reserved for canvas file uploads. Those requests are
	// already bounded by the server-wide request limit, so the media service
	// must not add a smaller image/video-specific business limit.
	SkipSizeLimit bool
}

type SeedanceImageUpload struct {
	UploadID    string
	MediaURL    string
	MediaType   string
	ContentType string
	SizeBytes   int64
	SHA256      string
	ExpiresAt   time.Time
	record      seedanceMediaRecord
}

type SeedanceMediaStream struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type SeedanceCapturedVideo struct {
	File         *os.File
	SizeBytes    int64
	ContentType  string
	StorageError error
	path         string
}

type SeedanceOutputArchiveLease struct {
	service        *SeedanceMediaService
	ownerTaskKey   string
	releaseArchive func()
	releaseOnce    sync.Once
}

func (l *SeedanceOutputArchiveLease) Close() {
	if l == nil {
		return
	}
	l.releaseOnce.Do(func() {
		if l.releaseArchive != nil {
			l.releaseArchive()
		}
		if l.service != nil && l.service.archiveSlots != nil {
			<-l.service.archiveSlots
		}
	})
}

func (v *SeedanceCapturedVideo) Close() error {
	if v == nil {
		return nil
	}
	var closeErr error
	if v.File != nil {
		closeErr = v.File.Close()
	}
	if v.path != "" {
		_ = os.Remove(v.path)
	}
	return closeErr
}

type SeedanceMaterializedImages struct {
	service  *SeedanceMediaService
	objects  []AgentArtifactObjectLocation
	retained bool
}

func (m *SeedanceMaterializedImages) Retain() {
	if m != nil {
		m.retained = true
	}
}

func (m *SeedanceMaterializedImages) Cleanup(ctx context.Context) {
	if m == nil || m.retained || m.service == nil || m.service.store == nil {
		return
	}
	cleanupBase := context.Background()
	if ctx != nil {
		cleanupBase = context.WithoutCancel(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(cleanupBase, seedanceObjectCleanupTimeout)
	defer cancel()
	for _, location := range m.objects {
		_ = m.service.store.DeleteObject(cleanupCtx, location)
	}
}

type seedanceMediaRecord struct {
	ID              string    `json:"id"`
	UserID          int64     `json:"user_id"`
	APIKeyID        int64     `json:"api_key_id"`
	GroupID         int64     `json:"group_id"`
	StorageProvider string    `json:"storage_provider"`
	Bucket          string    `json:"bucket"`
	ObjectKey       string    `json:"object_key"`
	ContentType     string    `json:"content_type"`
	SizeBytes       int64     `json:"size_bytes"`
	SHA256          string    `json:"sha256"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (r seedanceMediaRecord) location() AgentArtifactObjectLocation {
	return AgentArtifactObjectLocation{
		StorageProvider: r.StorageProvider,
		Bucket:          r.Bucket,
		ObjectKey:       r.ObjectKey,
	}
}

type SeedanceMediaService struct {
	store          AgentArtifactStore
	artifactConfig *AgentArtifactStorageConfigService
	redisClient    *redis.Client
	httpClient     *http.Client
	now            func() time.Time
	presignTTL     time.Duration
	maxVideoBytes  int64
	archiveSlots   chan struct{}
	mediaSlots     chan struct{}
	// lingdongRehostFn overrides temporary public rehost for tests.
	lingdongRehostFn func(ctx context.Context, filename, contentType string, payload []byte) (string, error)
}

func ProvideSeedanceMediaService(
	store AgentArtifactStore,
	cfg *config.Config,
	redisClient *redis.Client,
	artifactConfig *AgentArtifactStorageConfigService,
) *SeedanceMediaService {
	service := NewSeedanceMediaService(store, cfg, redisClient)
	service.artifactConfig = artifactConfig
	return service
}

func NewSeedanceMediaService(store AgentArtifactStore, cfg *config.Config, redisClient *redis.Client) *SeedanceMediaService {
	presignTTL := seedanceDefaultPresignTTL
	maxVideoBytes := seedanceDefaultVideoBytes
	if cfg != nil {
		if cfg.AgentArtifacts.DownloadURLTTLSeconds > 0 {
			presignTTL = time.Duration(cfg.AgentArtifacts.DownloadURLTTLSeconds) * time.Second
		}
		if cfg.AgentArtifacts.MaxUploadBytes > 0 {
			maxVideoBytes = cfg.AgentArtifacts.MaxUploadBytes
		}
	}
	return &SeedanceMediaService{
		store:         store,
		redisClient:   redisClient,
		httpClient:    newSeedanceMediaHTTPClient(),
		now:           time.Now,
		presignTTL:    presignTTL,
		maxVideoBytes: maxVideoBytes,
		archiveSlots:  make(chan struct{}, seedanceMaxConcurrentArchives),
		mediaSlots:    make(chan struct{}, seedanceMaxProcessMediaStreams),
	}
}

func (s *SeedanceMediaService) IsConfigured() bool {
	return s != nil && s.store != nil && s.store.IsConfigured()
}

func (s *SeedanceMediaService) SupportsManagedUploads() bool {
	return s.IsConfigured() && s.redisClient != nil
}

func (s *SeedanceMediaService) SupportsOutputArchive() bool {
	return s.IsConfigured() && s.redisClient != nil
}

func (s *SeedanceMediaService) AcquireMediaIO(ctx context.Context, owner SeedanceMediaOwner, _ int) (func(), error) {
	if !validSeedanceMediaOwner(owner) {
		return nil, infraerrors.BadRequest("invalid_media_owner", "Seedance media owner is invalid")
	}
	if s == nil || s.redisClient == nil {
		return nil, infraerrors.ServiceUnavailable("media_concurrency_unavailable", "Seedance media concurrency control is unavailable")
	}
	// Media preparation is a short-lived internal operation, not a generated
	// task. Reusing the user's submission concurrency here makes valid parallel
	// creates fail before they ever reach the upstream account. The process-wide
	// mediaSlots semaphore remains the hard resource guard.
	limit := seedanceMaxMediaConcurrency
	nowMillis := s.currentTime().UnixMilli()
	expiresMillis := nowMillis + seedanceMediaLeaseTTL.Milliseconds()
	keyDigest := sha256.Sum256([]byte(strconv.FormatInt(owner.UserID, 10)))
	key := seedanceMediaIOPrefix + hex.EncodeToString(keyDigest[:])
	token := uuid.NewString()
	deadline := time.Now().Add(seedanceMediaAcquireWait)
	for {
		nowMillis = s.currentTime().UnixMilli()
		expiresMillis = nowMillis + seedanceMediaLeaseTTL.Milliseconds()
		result, err := seedanceAcquireMediaIOScript.Run(ctx, s.redisClient, []string{key}, nowMillis, limit, expiresMillis, token, (seedanceMediaLeaseTTL + time.Minute).Milliseconds()).Int()
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("media_concurrency_unavailable", "Seedance media concurrency control is unavailable")
		}
		if result == 1 {
			break
		}
		if time.Now().After(deadline) {
			return nil, infraerrors.TooManyRequests("media_concurrency_exceeded", "Seedance media queue is full; retry after the task queue drains")
		}
		timer := time.NewTimer(seedanceMediaAcquireRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if s.mediaSlots != nil {
		deadline := time.NewTimer(seedanceMediaAcquireWait)
		defer deadline.Stop()
		for {
			select {
			case s.mediaSlots <- struct{}{}:
				goto acquired
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline.C:
				releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = s.redisClient.ZRem(releaseCtx, key, token).Err()
				cancel()
				return nil, infraerrors.TooManyRequests("media_concurrency_exceeded", "Seedance media queue is full; retry after the task queue drains")
			default:
				time.Sleep(seedanceMediaAcquireRetry)
			}
		}
	}
acquired:
	heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())
	go s.refreshMediaIOLease(heartbeatCtx, key, token)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			stopHeartbeat()
			if s.mediaSlots != nil {
				<-s.mediaSlots
			}
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.redisClient.ZRem(releaseCtx, key, token).Err()
		})
	}
	return release, nil
}

func (s *SeedanceMediaService) refreshMediaIOLease(ctx context.Context, key, token string) {
	ticker := time.NewTicker(seedanceMediaLeaseRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expiresMillis := time.Now().UTC().Add(seedanceMediaLeaseTTL).UnixMilli()
			result, err := seedanceRefreshMediaIOScript.Run(
				ctx,
				s.redisClient,
				[]string{key},
				token,
				expiresMillis,
				(seedanceMediaLeaseTTL + time.Minute).Milliseconds(),
			).Int()
			if err == nil && result != 1 {
				return
			}
		}
	}
}

func (s *SeedanceMediaService) CanArchiveOutput(ctx context.Context, contentLength int64) bool {
	if !s.SupportsOutputArchive() || contentLength <= 0 {
		return false
	}
	_, maxVideoBytes := s.runtimeStorageLimits(ctx)
	return maxVideoBytes <= 0 || contentLength <= maxVideoBytes
}

func (s *SeedanceMediaService) UploadImage(ctx context.Context, input SeedanceImageUploadInput) (*SeedanceImageUpload, error) {
	input.MediaKind = "image"
	return s.UploadMedia(ctx, input)
}

func (s *SeedanceMediaService) UploadMedia(ctx context.Context, input SeedanceImageUploadInput) (*SeedanceImageUpload, error) {
	if !validSeedanceMediaOwner(input.Owner) {
		return nil, infraerrors.BadRequest("invalid_media_owner", "Seedance media owner is invalid")
	}
	if !s.IsConfigured() {
		return nil, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	if input.Persistent && s.redisClient == nil {
		return nil, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance managed uploads require Redis")
	}
	if input.Body == nil {
		return nil, infraerrors.BadRequest("media_required", "media file is required")
	}
	if !input.SkipSizeLimit {
		maxBytes := seedanceMaxUploadBytesForKind(s, input.MediaKind)
		if input.SizeBytes > 0 && input.SizeBytes > maxBytes {
			return nil, seedanceUploadTooLargeError(input.MediaKind)
		}
	}

	uploadID := "sdupl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	kind := "task"
	if input.Persistent {
		kind = "staged"
	}
	record, err := s.storeSeedanceMedia(ctx, input.Owner, uploadID, kind, input.Body, input.SizeBytes, input.ContentType, input.Filename, input.MediaKind, input.SkipSizeLimit)
	if err != nil {
		return nil, err
	}
	if input.Persistent {
		if err := s.saveRecord(ctx, seedanceUploadRecordPrefix+uploadID, record, seedanceUploadRecordTTL); err != nil {
			s.deleteObjectBestEffort(ctx, record.location())
			return nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to register Seedance media upload").WithCause(err)
		}
	}
	mediaType := strings.TrimSpace(input.MediaKind)
	if mediaType == "" {
		mediaType = mediaKindFromContentType(record.ContentType)
	}
	mediaURL := ""
	if signed, signErr := s.presignRecord(ctx, record); signErr == nil {
		mediaURL = signed
	}
	return &SeedanceImageUpload{
		UploadID:    uploadID,
		MediaURL:    mediaURL,
		MediaType:   mediaType,
		ContentType: record.ContentType,
		SizeBytes:   record.SizeBytes,
		SHA256:      record.SHA256,
		ExpiresAt:   record.ExpiresAt,
		record:      record,
	}, nil
}

func mediaKindFromContentType(contentType string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/"):
		return "video"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "audio/"):
		return "audio"
	default:
		return ""
	}
}

func seedanceImageSizeLimitMessage(prefix string) string {
	return fmt.Sprintf("%s must not exceed %d bytes", prefix, SeedanceMaxImageBytes)
}

func seedanceUploadTooLargeError(mediaKind string) error {
	if strings.EqualFold(strings.TrimSpace(mediaKind), "image") || strings.TrimSpace(mediaKind) == "" {
		return infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", seedanceImageSizeLimitMessage("image"))
	}
	return infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", "media must not exceed the configured size limit")
}

func seedanceUploadSizeMismatchError(mediaKind string) error {
	if strings.EqualFold(strings.TrimSpace(mediaKind), "image") || strings.TrimSpace(mediaKind) == "" {
		return infraerrors.BadRequest("image_size_mismatch", "image size does not match Content-Length")
	}
	return infraerrors.BadRequest("media_size_mismatch", "media size does not match Content-Length")
}

func (s *SeedanceMediaService) UploadDataURI(ctx context.Context, owner SeedanceMediaOwner, value string, persistent bool) (*SeedanceImageUpload, error) {
	mediaType, encoded, err := splitSeedanceImageDataURI(value)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_base64", err.Error())
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(int(SeedanceMaxImageBytes)) {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", seedanceImageSizeLimitMessage("decoded image"))
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_base64", "image Base64 payload is invalid")
	}
	if int64(len(decoded)) > SeedanceMaxImageBytes {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", seedanceImageSizeLimitMessage("decoded image"))
	}
	return s.UploadImage(ctx, SeedanceImageUploadInput{
		Owner:       owner,
		Body:        bytes.NewReader(decoded),
		SizeBytes:   int64(len(decoded)),
		ContentType: mediaType,
		Persistent:  persistent,
	})
}

type seedanceRequestMediaTarget struct {
	url   *string
	kind  string
	slot  string
	index int
}

type seedanceMaterializedReference struct {
	url      string
	location *AgentArtifactObjectLocation
	cleanup  bool
}

func seedanceRequestMediaTargets(info *SeedanceRequestInfo, includeVideoAudio bool) []seedanceRequestMediaTarget {
	if info == nil {
		return nil
	}
	targets := []seedanceRequestMediaTarget{
		{url: &info.StartFrameURL, kind: "image", slot: seedanceStoredMediaStartFrame},
		{url: &info.EndFrameURL, kind: "image", slot: seedanceStoredMediaEndFrame},
	}
	for index := range info.References {
		targets = append(targets, seedanceRequestMediaTarget{
			url: &info.References[index].URL, kind: "image", slot: seedanceStoredMediaImage, index: index,
		})
	}
	if includeVideoAudio {
		for index := range info.VideoReferences {
			targets = append(targets, seedanceRequestMediaTarget{
				url: &info.VideoReferences[index].URL, kind: "video", slot: seedanceStoredMediaVideo, index: index,
			})
		}
		for index := range info.AudioReferences {
			targets = append(targets, seedanceRequestMediaTarget{
				url: &info.AudioReferences[index].URL, kind: "audio", slot: seedanceStoredMediaAudio, index: index,
			})
		}
	}
	return targets
}

func seedanceStoredMediaKey(slot string, index int) string {
	return strings.TrimSpace(slot) + ":" + strconv.Itoa(index)
}

// SnapshotSeedanceTaskMediaCleanup persists only task-owned media that must be
// deleted after terminal settlement. It deliberately omits staged user uploads
// and request content; fallback-capable requests continue to use their full
// fallback snapshot instead.
func SnapshotSeedanceTaskMediaCleanup(info *SeedanceRequestInfo) ([]byte, error) {
	if info == nil {
		return nil, errors.New("seedance request info is required")
	}
	temporary := make([]SeedanceStoredMediaReference, 0, len(info.StoredMedia))
	for _, reference := range info.StoredMedia {
		if reference.DeleteAfterSettlement {
			temporary = append(temporary, reference)
		}
	}
	if len(temporary) == 0 {
		return nil, nil
	}
	return json.Marshal(struct {
		StoredMedia []SeedanceStoredMediaReference `json:"stored_media"`
	}{StoredMedia: temporary})
}

func (s *SeedanceMediaService) MaterializeImages(ctx context.Context, owner SeedanceMediaOwner, info *SeedanceRequestInfo) (*SeedanceMaterializedImages, error) {
	if info == nil {
		return nil, infraerrors.BadRequest("invalid_request", "Seedance request info is required")
	}
	materialized := &SeedanceMaterializedImages{service: s}
	cleanupOnError := func(err error) (*SeedanceMaterializedImages, error) {
		materialized.Cleanup(context.Background())
		return nil, err
	}

	info.StoredMedia = nil
	directHTTP := isHuiquVideoModel(info.Model)
	_, fallbackEligible := SeedanceFallbackModelFor(info.Model, info.Resolution, info.DurationSeconds)
	// Face-ref models may map to Lingdong for video refs; materialize videos so
	// PrepareLingdongPublicMedia can strip-signed COS URLs without re-fetch.
	includeVideoAudio := fallbackEligible || isXimeiVideoModel(info.Model) || isWeijinVideoModel(info.Model)
	for _, target := range seedanceRequestMediaTargets(info, includeVideoAudio) {
		if target.url == nil || strings.TrimSpace(*target.url) == "" {
			continue
		}
		// Weijin face-ref (+ Pixelle mapped multi-modal) can consume public third-party
		// HTTPS media directly. Avoid force-fetching large image/video sets onto COS
		// just to rehost them again for the mapped provider.
		useDirect := directHTTP
		if !useDirect && isWeijinVideoModel(info.Model) && !seedanceMediaURLNeedsLingdongRehost(strings.TrimSpace(*target.url)) {
			useDirect = true
		}
		resolved, err := s.materializeReferenceMedia(ctx, owner, *target.url, target.kind, useDirect)
		if err != nil {
			return cleanupOnError(err)
		}
		*target.url = resolved.url
		if resolved.location != nil {
			info.StoredMedia = append(info.StoredMedia, SeedanceStoredMediaReference{
				Slot: target.slot, Index: target.index,
				StorageProvider:       resolved.location.StorageProvider,
				Bucket:                resolved.location.Bucket,
				ObjectKey:             resolved.location.ObjectKey,
				DeleteAfterSettlement: resolved.cleanup,
			})
			if resolved.cleanup {
				materialized.objects = append(materialized.objects, *resolved.location)
			}
		}
	}
	return materialized, nil
}

func (s *SeedanceMediaService) materializeImage(ctx context.Context, owner SeedanceMediaOwner, source string) (string, *AgentArtifactObjectLocation, error) {
	return s.materializeImageMode(ctx, owner, source, false)
}

func (s *SeedanceMediaService) materializeImageMode(ctx context.Context, owner SeedanceMediaOwner, source string, directHTTP bool) (string, *AgentArtifactObjectLocation, error) {
	resolved, err := s.materializeReferenceMedia(ctx, owner, source, "image", directHTTP)
	if err != nil {
		return "", nil, err
	}
	if resolved.cleanup {
		return resolved.url, resolved.location, nil
	}
	return resolved.url, nil, nil
}

func (s *SeedanceMediaService) materializeReferenceMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	source, mediaKind string,
	directHTTP bool,
) (seedanceMaterializedReference, error) {
	source = strings.TrimSpace(source)
	if uploadID := managedSeedanceUploadID(source); uploadID != "" {
		record, err := s.loadManagedUpload(ctx, owner, uploadID)
		if err != nil {
			return seedanceMaterializedReference{}, err
		}
		if storedKind := mediaKindFromContentType(record.ContentType); storedKind != mediaKind {
			return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_media_type", fmt.Sprintf("reference media must be a %s type", mediaKind))
		}
		signed, err := s.presignRecord(ctx, record)
		if err != nil {
			return seedanceMaterializedReference{}, err
		}
		location := record.location()
		return seedanceMaterializedReference{url: signed, location: &location}, nil
	}
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		if mediaKind != "image" {
			return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_media_url", "video and audio references must use HTTP(S) URLs or managed uploads")
		}
		// Huiqu models re-embed reference media locally (JSON data URLs for H3 /
		// multipart file parts for MX933). Keep validated data URIs as-is so image
		// generation does not require object storage just to round-trip bytes the
		// gateway will re-encode before calling upstream. Private COS uploads remain
		// the primary canvas path when managed storage is configured.
		if directHTTP {
			if _, _, err := splitSeedanceImageDataURI(source); err != nil {
				return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_media_url", err.Error())
			}
			return seedanceMaterializedReference{url: source}, nil
		}
		upload, err := s.UploadDataURI(ctx, owner, source, false)
		if err != nil {
			return seedanceMaterializedReference{}, err
		}
		signed, err := s.presignRecord(ctx, upload.record)
		if err != nil {
			s.deleteObjectBestEffort(ctx, upload.record.location())
			return seedanceMaterializedReference{}, err
		}
		location := upload.record.location()
		return seedanceMaterializedReference{url: signed, location: &location, cleanup: true}, nil
	}
	if !isSeedanceHTTPImageURL(source) {
		if mediaKind == "image" {
			return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_image_url", "image URL must be HTTP(S), a managed upload URL, or a supported Base64 data URI")
		}
		return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_media_url", "reference media URL must use HTTP(S) or a managed upload")
	}
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		if mediaKind == "image" {
			return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_image_url", err.Error())
		}
		return seedanceMaterializedReference{}, infraerrors.BadRequest("invalid_media_url", err.Error())
	}
	if directHTTP {
		return seedanceMaterializedReference{url: validated}, nil
	}
	if location, ok := s.seedanceObjectLocationFromOwnURL(owner, validated); ok {
		signed, signErr := s.presignLocation(ctx, location)
		if signErr != nil {
			return seedanceMaterializedReference{}, signErr
		}
		return seedanceMaterializedReference{url: signed, location: &location}, nil
	}
	if !s.IsConfigured() {
		return seedanceMaterializedReference{url: validated}, nil
	}
	upload, err := s.fetchAndStoreReferenceMedia(ctx, owner, validated, mediaKind)
	if err != nil {
		return seedanceMaterializedReference{}, err
	}
	signed, err := s.presignRecord(ctx, upload.record)
	if err != nil {
		s.deleteObjectBestEffort(ctx, upload.record.location())
		return seedanceMaterializedReference{}, err
	}
	location := upload.record.location()
	return seedanceMaterializedReference{url: signed, location: &location, cleanup: true}, nil
}

func (s *SeedanceMediaService) isOwnPersistentSeedanceUploadURL(owner SeedanceMediaOwner, source string) bool {
	location, ok := s.seedanceObjectLocationFromOwnURL(owner, source)
	return ok && strings.Contains(location.ObjectKey, fmt.Sprintf("seedance/inputs/staged/%d/%d/", owner.UserID, owner.APIKeyID))
}

func seedanceObjectKeyBelongsToOwner(objectKey string, owner SeedanceMediaOwner) bool {
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if objectKey == "" || !validSeedanceMediaOwner(owner) {
		return false
	}
	for _, kind := range []string{"task", "staged"} {
		marker := fmt.Sprintf("seedance/inputs/%s/%d/%d/", kind, owner.UserID, owner.APIKeyID)
		if strings.Contains(objectKey, marker) {
			return true
		}
	}
	return false
}

func seedanceObjectKeyIsTaskOwned(objectKey string, owner SeedanceMediaOwner) bool {
	marker := fmt.Sprintf("seedance/inputs/task/%d/%d/", owner.UserID, owner.APIKeyID)
	return strings.Contains(strings.TrimLeft(strings.TrimSpace(objectKey), "/"), marker)
}

func (s *SeedanceMediaService) seedanceObjectLocationFromOwnURL(owner SeedanceMediaOwner, source string) (AgentArtifactObjectLocation, bool) {
	if s == nil || s.store == nil || !s.store.IsConfigured() || !validSeedanceMediaOwner(owner) {
		return AgentArtifactObjectLocation{}, false
	}
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return AgentArtifactObjectLocation{}, false
	}
	objectKey, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return AgentArtifactObjectLocation{}, false
	}
	objectKey = strings.TrimLeft(objectKey, "/")
	bucket := strings.TrimSpace(s.store.Bucket())
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if bucket != "" && !strings.Contains(host, strings.ToLower(bucket)) && strings.HasPrefix(objectKey, bucket+"/") {
		objectKey = strings.TrimPrefix(objectKey, bucket+"/")
	}
	if !seedanceObjectKeyBelongsToOwner(objectKey, owner) {
		return AgentArtifactObjectLocation{}, false
	}
	return AgentArtifactObjectLocation{
		StorageProvider: strings.TrimSpace(s.store.Provider()),
		Bucket:          bucket,
		ObjectKey:       objectKey,
	}, true
}

// RefreshSeedanceFallbackMediaURLs replaces stored or legacy presigned input
// URLs with fresh signatures immediately before an asynchronous fallback is
// prepared. References that are genuinely external remain unchanged.
func (s *SeedanceMediaService) RefreshSeedanceFallbackMediaURLs(
	ctx context.Context,
	owner SeedanceMediaOwner,
	info *SeedanceRequestInfo,
) error {
	if info == nil {
		return infraerrors.BadRequest("invalid_request", "Seedance request info is required")
	}
	if !validSeedanceMediaOwner(owner) {
		return infraerrors.BadRequest("invalid_media_owner", "Seedance media owner is invalid")
	}
	if !s.IsConfigured() {
		return infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}

	stored := make(map[string]AgentArtifactObjectLocation, len(info.StoredMedia))
	for _, reference := range info.StoredMedia {
		location := AgentArtifactObjectLocation{
			StorageProvider: strings.TrimSpace(reference.StorageProvider),
			Bucket:          strings.TrimSpace(reference.Bucket),
			ObjectKey:       strings.TrimLeft(strings.TrimSpace(reference.ObjectKey), "/"),
		}
		if !seedanceObjectKeyBelongsToOwner(location.ObjectKey, owner) {
			return errors.New("seedance fallback stored media reference is invalid")
		}
		key := seedanceStoredMediaKey(reference.Slot, reference.Index)
		if _, exists := stored[key]; exists {
			return errors.New("seedance fallback stored media reference is duplicated")
		}
		stored[key] = location
	}

	for _, target := range seedanceRequestMediaTargets(info, true) {
		if target.url == nil || strings.TrimSpace(*target.url) == "" {
			continue
		}
		location, ok := stored[seedanceStoredMediaKey(target.slot, target.index)]
		if !ok {
			location, ok = s.seedanceObjectLocationFromOwnURL(owner, *target.url)
		}
		if !ok {
			continue
		}
		signed, err := s.presignLocation(ctx, location)
		if err != nil {
			return err
		}
		*target.url = signed
	}
	return nil
}

// DeleteSeedanceFallbackMedia removes only request-scoped copies created for a
// task. Staged user uploads are deliberately excluded and remain reusable for
// their normal upload lifetime.
func (s *SeedanceMediaService) DeleteSeedanceFallbackMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	snapshot []byte,
) error {
	if len(snapshot) == 0 || s == nil || s.store == nil {
		return nil
	}
	var storedSnapshot seedanceFallbackSnapshot
	if err := json.Unmarshal(snapshot, &storedSnapshot); err != nil {
		// Cleanup must never make an otherwise terminal task permanently
		// unsettled because of a legacy or malformed optional snapshot.
		return nil
	}
	locations := make(map[string]AgentArtifactObjectLocation)
	add := func(location AgentArtifactObjectLocation, explicitlyTemporary bool) {
		location.StorageProvider = strings.TrimSpace(location.StorageProvider)
		location.Bucket = strings.TrimSpace(location.Bucket)
		location.ObjectKey = strings.TrimLeft(strings.TrimSpace(location.ObjectKey), "/")
		if !explicitlyTemporary || !seedanceObjectKeyBelongsToOwner(location.ObjectKey, owner) ||
			!seedanceObjectKeyIsTaskOwned(location.ObjectKey, owner) {
			return
		}
		key := location.StorageProvider + "\x00" + location.Bucket + "\x00" + location.ObjectKey
		locations[key] = location
	}
	for _, reference := range storedSnapshot.StoredMedia {
		add(AgentArtifactObjectLocation{
			StorageProvider: reference.StorageProvider,
			Bucket:          reference.Bucket,
			ObjectKey:       reference.ObjectKey,
		}, reference.DeleteAfterSettlement)
	}
	legacyURLs := []string{storedSnapshot.StartFrameURL, storedSnapshot.EndFrameURL}
	for _, reference := range storedSnapshot.References {
		legacyURLs = append(legacyURLs, reference.URL)
	}
	for _, reference := range storedSnapshot.VideoReferences {
		legacyURLs = append(legacyURLs, reference.URL)
	}
	for _, reference := range storedSnapshot.AudioReferences {
		legacyURLs = append(legacyURLs, reference.URL)
	}
	for _, rawURL := range legacyURLs {
		if location, ok := s.seedanceObjectLocationFromOwnURL(owner, rawURL); ok {
			add(location, seedanceObjectKeyIsTaskOwned(location.ObjectKey, owner))
		}
	}

	var firstErr error
	for _, location := range locations {
		if err := s.store.DeleteObject(ctx, location); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SeedanceMediaService) OpenManagedUpload(ctx context.Context, owner SeedanceMediaOwner, uploadID, rangeHeader string) (*SeedanceMediaStream, error) {
	record, err := s.loadManagedUpload(ctx, owner, strings.TrimSpace(uploadID))
	if err != nil {
		return nil, err
	}
	return s.openRecord(ctx, record, rangeHeader)
}

func (s *SeedanceMediaService) OpenCachedOutput(ctx context.Context, owner SeedanceMediaOwner, taskID, rangeHeader string) (*SeedanceMediaStream, bool, error) {
	if s == nil || s.redisClient == nil || !s.IsConfigured() {
		return nil, false, nil
	}
	key := seedanceOutputRecordKey(owner, taskID)
	payload, err := s.redisClient.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var record seedanceMediaRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		_ = s.redisClient.Del(ctx, key).Err()
		return nil, false, nil
	}
	if record.ID != strings.TrimSpace(taskID) || record.UserID != owner.UserID || record.APIKeyID != owner.APIKeyID || record.GroupID != owner.GroupID {
		_ = s.redisClient.Del(ctx, key).Err()
		return nil, false, nil
	}
	stream, err := s.openRecord(ctx, record, rangeHeader)
	if err != nil {
		return nil, false, err
	}
	return stream, true, nil
}

func (s *SeedanceMediaService) CaptureAndStoreOutput(ctx context.Context, owner SeedanceMediaOwner, taskID, contentType string, contentLength int64, body io.Reader) (*SeedanceCapturedVideo, error) {
	lease, acquired := s.BeginOutputArchive(ctx, owner, taskID)
	if !acquired {
		return nil, ErrSeedanceOutputArchiveInProgress
	}
	defer lease.Close()
	return s.CaptureAndStoreOutputWithLease(ctx, lease, owner, taskID, contentType, contentLength, body)
}

func (s *SeedanceMediaService) CaptureAndStoreOutputWithLease(ctx context.Context, lease *SeedanceOutputArchiveLease, owner SeedanceMediaOwner, taskID, contentType string, contentLength int64, body io.Reader) (*SeedanceCapturedVideo, error) {
	if body == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream video body is empty")
	}
	ownerTaskKey := seedanceOutputRecordKey(owner, taskID)
	if lease == nil || lease.service != s || lease.ownerTaskKey != ownerTaskKey {
		return nil, infraerrors.InternalServer("invalid_archive_lease", "Seedance output archive lease is invalid")
	}
	_, maxVideoBytes := s.runtimeStorageLimits(ctx)
	if contentLength > maxVideoBytes && maxVideoBytes > 0 {
		return nil, infraerrors.New(http.StatusBadGateway, "video_too_large", "Seedance upstream video exceeds the configured media limit")
	}
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "video-*.mp4")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	result := &SeedanceCapturedVideo{File: tmp, path: path, ContentType: "video/mp4"}
	failed := true
	defer func() {
		if failed {
			_ = result.Close()
		}
	}()

	hasher := sha256.New()
	reader := io.Reader(body)
	if maxVideoBytes > 0 {
		reader = io.LimitReader(body, maxVideoBytes+1)
	}
	written, err := io.Copy(tmp, io.TeeReader(reader, hasher))
	if err != nil {
		return nil, fmt.Errorf("read Seedance upstream video: %w", err)
	}
	if maxVideoBytes > 0 && written > maxVideoBytes {
		return nil, infraerrors.New(http.StatusBadGateway, "video_too_large", "Seedance upstream video exceeds the configured media limit")
	}
	if err := validateSeedanceMP4(tmp, written); err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	result.SizeBytes = written

	if s.IsConfigured() {
		taskDigest := sha256.Sum256([]byte(strings.TrimSpace(taskID)))
		objectKey := fmt.Sprintf("seedance/outputs/%d/%d/%s.mp4", owner.UserID, owner.APIKeyID, hex.EncodeToString(taskDigest[:]))
		put, putErr := s.store.Put(ctx, AgentArtifactStorePutInput{
			Key:         objectKey,
			Body:        tmp,
			ContentType: "video/mp4",
			SizeBytes:   written,
			Metadata: map[string]string{
				"media-kind": "seedance-output",
				"user-id":    strconv.FormatInt(owner.UserID, 10),
				"api-key-id": strconv.FormatInt(owner.APIKeyID, 10),
				"task-id":    taskID,
			},
		})
		if putErr == nil && put == nil {
			putErr = errors.New("artifact store returned an empty result")
		}
		if putErr != nil {
			result.StorageError = putErr
		} else {
			record := seedanceMediaRecord{
				ID:              taskID,
				UserID:          owner.UserID,
				APIKeyID:        owner.APIKeyID,
				GroupID:         owner.GroupID,
				StorageProvider: put.Provider,
				Bucket:          put.Bucket,
				ObjectKey:       put.ObjectKey,
				ContentType:     "video/mp4",
				SizeBytes:       written,
				SHA256:          hex.EncodeToString(hasher.Sum(nil)),
				ExpiresAt:       s.currentTime().Add(seedanceOutputRecordTTL),
			}
			if s.redisClient == nil {
				result.StorageError = errors.New("Redis is unavailable")
				s.deleteObjectBestEffort(ctx, record.location())
			} else if err := s.saveRecord(ctx, seedanceOutputRecordKey(owner, taskID), record, seedanceOutputRecordTTL); err != nil {
				result.StorageError = err
				s.deleteObjectBestEffort(ctx, record.location())
			}
		}
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	failed = false
	return result, nil
}

func (s *SeedanceMediaService) storeSeedanceMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	uploadID,
	kind string,
	body io.Reader,
	sizeHint int64,
	declaredType string,
	filename string,
	mediaKind string,
	skipSizeLimit bool,
) (seedanceMediaRecord, error) {
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "media-*")
	if err != nil {
		return seedanceMediaRecord{}, err
	}
	path := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(path)
	}()

	hasher := sha256.New()
	reader := body
	maxBytes := int64(0)
	if !skipSizeLimit {
		maxBytes = seedanceMaxUploadBytesForKind(s, mediaKind)
		reader = io.LimitReader(body, maxBytes+1)
	}
	written, err := io.Copy(tmp, io.TeeReader(reader, hasher))
	if err != nil {
		return seedanceMediaRecord{}, fmt.Errorf("read Seedance media: %w", err)
	}
	if maxBytes > 0 && written > maxBytes {
		return seedanceMediaRecord{}, seedanceUploadTooLargeError(mediaKind)
	}
	if sizeHint > 0 && written != sizeHint {
		return seedanceMediaRecord{}, seedanceUploadSizeMismatchError(mediaKind)
	}
	mediaType, extension, err := inspectSeedanceMedia(tmp, declaredType, filename, mediaKind)
	if err != nil {
		return seedanceMediaRecord{}, err
	}
	if strings.TrimSpace(extension) == "" {
		extension = "bin"
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return seedanceMediaRecord{}, err
	}
	objectKey := fmt.Sprintf("seedance/inputs/%s/%d/%d/%s.%s", kind, owner.UserID, owner.APIKeyID, uploadID, extension)
	put, err := s.store.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        tmp,
		ContentType: mediaType,
		SizeBytes:   written,
		Metadata: map[string]string{
			"media-kind": "seedance-input",
			"user-id":    strconv.FormatInt(owner.UserID, 10),
			"api-key-id": strconv.FormatInt(owner.APIKeyID, 10),
		},
	})
	if err != nil {
		return seedanceMediaRecord{}, infraerrors.ServiceUnavailable("media_storage_error", "failed to store Seedance media").WithCause(err)
	}
	if put == nil {
		return seedanceMediaRecord{}, infraerrors.ServiceUnavailable("media_storage_error", "failed to store Seedance media")
	}
	now := s.currentTime()
	return seedanceMediaRecord{
		ID:              uploadID,
		UserID:          owner.UserID,
		APIKeyID:        owner.APIKeyID,
		GroupID:         owner.GroupID,
		StorageProvider: put.Provider,
		Bucket:          put.Bucket,
		ObjectKey:       put.ObjectKey,
		ContentType:     mediaType,
		SizeBytes:       written,
		SHA256:          hex.EncodeToString(hasher.Sum(nil)),
		ExpiresAt:       now.Add(seedanceUploadRecordTTL),
	}, nil
}

func seedanceMaxUploadBytesForKind(s *SeedanceMediaService, mediaKind string) int64 {
	switch strings.ToLower(strings.TrimSpace(mediaKind)) {
	case "image", "":
		return SeedanceMaxImageBytes
	default:
		if s != nil {
			_, maxVideoBytes := s.runtimeStorageLimits(context.Background())
			if maxVideoBytes > 0 {
				return maxVideoBytes
			}
		}
		return seedanceDefaultVideoBytes
	}
}

func inspectSeedanceMedia(file *os.File, declaredType, filename, mediaKind string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(mediaKind)) {
	case "", "image":
		return inspectSeedanceImage(file, declaredType)
	case "video":
		return normalizeSeedanceGenericMedia(file, declaredType, filename, "video")
	case "audio":
		return normalizeSeedanceGenericMedia(file, declaredType, filename, "audio")
	default:
		return "", "", infraerrors.BadRequest("invalid_media_type", "media kind must be image, video, or audio")
	}
}

func normalizeSeedanceGenericMedia(file *os.File, declaredType, filename, family string) (string, string, error) {
	if file == nil {
		return "", "", infraerrors.BadRequest("invalid_media", "media file is required")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("inspect Seedance media: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(header[:n])))
	declared := strings.ToLower(strings.TrimSpace(declaredType))
	if declared == "" {
		declared = detected
	}
	switch family {
	case "video":
		if strings.HasPrefix(declared, "video/") || declared == "application/octet-stream" || strings.HasPrefix(detected, "video/") {
			mediaType := declared
			if !strings.HasPrefix(mediaType, "video/") {
				mediaType = detected
			}
			extension := mediaExtensionForContentType(mediaType, filename, "mp4")
			return mediaType, extension, nil
		}
	case "audio":
		if strings.HasPrefix(declared, "audio/") || declared == "application/octet-stream" || strings.HasPrefix(detected, "audio/") {
			mediaType := declared
			if !strings.HasPrefix(mediaType, "audio/") {
				mediaType = detected
			}
			extension := mediaExtensionForContentType(mediaType, filename, "mp3")
			return mediaType, extension, nil
		}
	}
	return "", "", infraerrors.BadRequest("invalid_media_type", fmt.Sprintf("media content type must be a %s type", family))
}

func mediaExtensionForContentType(contentType, filename, fallback string) string {
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(filename))), "."); ext != "" {
		return ext
	}
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		for _, ext := range exts {
			trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
			if trimmed != "" {
				return trimmed
			}
		}
	}
	if ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fallback)), "."); ext != "" {
		return ext
	}
	return "bin"
}

func (s *SeedanceMediaService) fetchAndStoreImage(ctx context.Context, owner SeedanceMediaOwner, source string) (*SeedanceImageUpload, error) {
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_url", err.Error())
	}
	fetchCtx, cancel := context.WithTimeout(ctx, seedanceImageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_url", "image URL is invalid")
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "image_fetch_failed", "failed to download reference image").WithCause(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, infraerrors.New(http.StatusBadGateway, "image_fetch_failed", fmt.Sprintf("reference image returned HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > SeedanceMaxImageBytes {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", seedanceImageSizeLimitMessage("image"))
	}
	return s.UploadImage(ctx, SeedanceImageUploadInput{
		Owner:       owner,
		Body:        resp.Body,
		SizeBytes:   resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
		Persistent:  false,
	})
}

func (s *SeedanceMediaService) fetchAndStoreReferenceMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	source, mediaKind string,
) (*SeedanceImageUpload, error) {
	if mediaKind == "image" {
		return s.fetchAndStoreImage(ctx, owner, source)
	}
	limit := huiquMaxVideoBytes
	if mediaKind == "audio" {
		limit = huiquMaxAudioBytes
	}
	downloaded, err := s.downloadHuiquMedia(ctx, owner, nil, source, mediaKind, "fallback-"+mediaKind, 0, "", limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(downloaded.Path) }()
	file, err := os.Open(downloaded.Path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return s.UploadMedia(ctx, SeedanceImageUploadInput{
		Owner:       owner,
		Body:        file,
		SizeBytes:   downloaded.SizeBytes,
		ContentType: downloaded.ContentType,
		Filename:    downloaded.Filename,
		MediaKind:   mediaKind,
		Persistent:  false,
	})
}

func (s *SeedanceMediaService) loadManagedUpload(ctx context.Context, owner SeedanceMediaOwner, uploadID string) (seedanceMediaRecord, error) {
	if s == nil || s.redisClient == nil || !strings.HasPrefix(uploadID, "sdupl_") {
		return seedanceMediaRecord{}, infraerrors.NotFound("upload_not_found", "Seedance image upload not found")
	}
	payload, err := s.redisClient.Get(ctx, seedanceUploadRecordPrefix+uploadID).Bytes()
	if errors.Is(err, redis.Nil) {
		return seedanceMediaRecord{}, infraerrors.NotFound("upload_not_found", "Seedance image upload not found")
	}
	if err != nil {
		return seedanceMediaRecord{}, infraerrors.ServiceUnavailable("media_storage_error", "failed to load Seedance image upload").WithCause(err)
	}
	var record seedanceMediaRecord
	if err := json.Unmarshal(payload, &record); err != nil || record.ID != uploadID || record.UserID != owner.UserID || record.APIKeyID != owner.APIKeyID || record.GroupID != owner.GroupID {
		return seedanceMediaRecord{}, infraerrors.NotFound("upload_not_found", "Seedance image upload not found")
	}
	return record, nil
}

func (s *SeedanceMediaService) saveRecord(ctx context.Context, key string, record seedanceMediaRecord, ttl time.Duration) error {
	if s == nil || s.redisClient == nil {
		return errors.New("Redis is unavailable")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, key, payload, ttl).Err()
}

func (s *SeedanceMediaService) presignRecord(ctx context.Context, record seedanceMediaRecord) (string, error) {
	return s.presignLocation(ctx, record.location())
}

func (s *SeedanceMediaService) presignLocation(ctx context.Context, location AgentArtifactObjectLocation) (string, error) {
	if !s.IsConfigured() {
		return "", infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	if strings.TrimSpace(location.ObjectKey) == "" {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "Seedance media object location is invalid")
	}
	presignTTL, _ := s.runtimeStorageLimits(ctx)
	signed, err := s.store.PresignGetObject(ctx, location, presignTTL)
	if err != nil {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "failed to sign Seedance media URL")
	}
	if !isSeedanceHTTPImageURL(signed) {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "Seedance media storage did not return an HTTP(S) URL")
	}
	return signed, nil
}

func (s *SeedanceMediaService) openRecord(ctx context.Context, record seedanceMediaRecord, rangeHeader string) (*SeedanceMediaStream, error) {
	if reader, ok := s.store.(AgentArtifactObjectReader); ok {
		result, err := reader.ReadObject(ctx, record.location(), rangeHeader)
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to read stored Seedance media").WithCause(err)
		}
		if result == nil || result.Body == nil {
			return nil, infraerrors.ServiceUnavailable("media_storage_error", "stored Seedance media is unavailable")
		}
		if result.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return &SeedanceMediaStream{StatusCode: result.StatusCode, Header: result.Header.Clone(), Body: result.Body}, nil
		}
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			_ = result.Body.Close()
			return nil, fmt.Errorf("stored Seedance media returned HTTP %d", result.StatusCode)
		}
		return &SeedanceMediaStream{StatusCode: result.StatusCode, Header: result.Header.Clone(), Body: result.Body}, nil
	}
	signed, err := s.presignRecord(ctx, record)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signed, nil)
	if err != nil {
		return nil, err
	}
	if value := strings.TrimSpace(rangeHeader); value != "" {
		req.Header.Set("Range", value)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to read stored Seedance media")
	}
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return &SeedanceMediaStream{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: resp.Body}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("stored Seedance media returned HTTP %d", resp.StatusCode)
	}
	return &SeedanceMediaStream{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: resp.Body}, nil
}

func (s *SeedanceMediaService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *SeedanceMediaService) runtimeStorageLimits(ctx context.Context) (time.Duration, int64) {
	presignTTL := seedanceDefaultPresignTTL
	maxVideoBytes := seedanceDefaultVideoBytes
	if s == nil {
		return presignTTL, maxVideoBytes
	}
	if s.presignTTL > 0 {
		presignTTL = s.presignTTL
	}
	if s.maxVideoBytes > 0 {
		maxVideoBytes = s.maxVideoBytes
	}
	if s.artifactConfig == nil {
		return presignTTL, maxVideoBytes
	}
	runtimeConfig, ok, err := s.artifactConfig.CurrentRuntimeConfig(ctx)
	if err != nil || !ok {
		return presignTTL, maxVideoBytes
	}
	if runtimeConfig.DownloadURLTTLSeconds > 0 {
		presignTTL = time.Duration(runtimeConfig.DownloadURLTTLSeconds) * time.Second
	}
	if runtimeConfig.MaxUploadBytes > 0 {
		maxVideoBytes = runtimeConfig.MaxUploadBytes
	}
	return presignTTL, maxVideoBytes
}

func seedanceTempDirectory() string {
	seedanceTempDirOnce.Do(func() {
		seedanceTempDirPath = filepath.Join(os.TempDir(), "sub2api-seedance")
		if err := os.MkdirAll(seedanceTempDirPath, 0o700); err == nil {
			cleanupStaleSeedanceTempFiles(seedanceTempDirPath, time.Now().Add(-24*time.Hour), 1000)
		}
	})
	return seedanceTempDirPath
}

func cleanupStaleSeedanceTempFiles(directory string, olderThan time.Time, limit int) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	removed := 0
	for _, entry := range entries {
		if limit > 0 && removed >= limit {
			return
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "image-") && !strings.HasPrefix(name, "video-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.IsDir() || !info.ModTime().Before(olderThan) {
			continue
		}
		if os.Remove(filepath.Join(directory, name)) == nil {
			removed++
		}
	}
}

func (s *SeedanceMediaService) deleteObjectBestEffort(ctx context.Context, location AgentArtifactObjectLocation) {
	if s == nil || s.store == nil || strings.TrimSpace(location.ObjectKey) == "" {
		return
	}
	cleanupBase := context.Background()
	if ctx != nil {
		cleanupBase = context.WithoutCancel(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(cleanupBase, seedanceObjectCleanupTimeout)
	defer cancel()
	_ = s.store.DeleteObject(cleanupCtx, location)
}

func (s *SeedanceMediaService) acquireOutputArchive(ctx context.Context, owner SeedanceMediaOwner, taskID string) (func(), bool) {
	if s == nil || s.redisClient == nil {
		return nil, false
	}
	key := seedanceOutputLockPrefix + strings.TrimPrefix(seedanceOutputRecordKey(owner, taskID), seedanceOutputRecordPrefix)
	token := uuid.NewString()
	acquired, err := s.redisClient.SetNX(ctx, key, token, seedanceOutputArchiveLockTTL).Result()
	if err != nil {
		return nil, false
	}
	if !acquired {
		return nil, false
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = seedanceReleaseArchiveLockScript.Run(releaseCtx, s.redisClient, []string{key}, token).Result()
	}
	return release, true
}

func (s *SeedanceMediaService) BeginOutputArchive(ctx context.Context, owner SeedanceMediaOwner, taskID string) (*SeedanceOutputArchiveLease, bool) {
	if s == nil || !s.SupportsOutputArchive() || !validSeedanceMediaOwner(owner) || strings.TrimSpace(taskID) == "" {
		return nil, false
	}
	if s.archiveSlots != nil {
		select {
		case s.archiveSlots <- struct{}{}:
		default:
			return nil, false
		}
	}
	releaseArchive, acquired := s.acquireOutputArchive(ctx, owner, taskID)
	if !acquired {
		if s.archiveSlots != nil {
			<-s.archiveSlots
		}
		return nil, false
	}
	return &SeedanceOutputArchiveLease{
		service:        s,
		ownerTaskKey:   seedanceOutputRecordKey(owner, taskID),
		releaseArchive: releaseArchive,
	}, true
}

func inspectSeedanceImage(file *os.File, declaredType string) (string, string, error) {
	if file == nil {
		return "", "", infraerrors.BadRequest("invalid_image", "image is invalid")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	// Content sniffing is the source of truth. Browser/client Content-Type is often
	// wrong (renamed extensions, clipboard, chat apps, WeChat/QQ saves, etc.). Rejecting
	// on declared/detected mismatch caused false image_type_mismatch for valid media.
	detected := normalizeSeedanceInlineImageMediaType(http.DetectContentType(header[:n]))
	if detected == "" {
		return "", "", infraerrors.BadRequest("unsupported_image_type", "image must be PNG, JPEG, or WebP")
	}
	// Declared Content-Type from clients is ignored (advisory only).
	_ = declaredType
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	imageConfig, _, err := image.DecodeConfig(file)
	if err != nil {
		return "", "", infraerrors.BadRequest("invalid_image", "image data cannot be decoded")
	}
	if imageConfig.Width <= 0 || imageConfig.Height <= 0 || imageConfig.Width > SeedanceMaxImageDimension || imageConfig.Height > SeedanceMaxImageDimension || int64(imageConfig.Width)*int64(imageConfig.Height) > SeedanceMaxImagePixels {
		return "", "", infraerrors.BadRequest("image_dimensions_invalid", "image dimensions must be at most 8192x8192 and 40 megapixels")
	}
	extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[detected]
	return detected, extension, nil
}

func normalizeSeedanceDeclaredImageType(value string) (string, error) {
	value = strings.TrimSpace(strings.Split(value, ";")[0])
	if value == "" || strings.EqualFold(value, "application/octet-stream") {
		return "", nil
	}
	normalized := normalizeSeedanceInlineImageMediaType(value)
	if normalized == "" {
		return "", infraerrors.BadRequest("unsupported_image_type", "declared image type must be image/png, image/jpeg, or image/webp")
	}
	return normalized, nil
}

func validateSeedanceMP4(file *os.File, size int64) error {
	if file == nil || size < 12 {
		return infraerrors.New(http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream did not return a valid MP4")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return err
	}
	if string(header[4:8]) != "ftyp" {
		return infraerrors.New(http.StatusBadGateway, "invalid_upstream_response", "Seedance upstream did not return a valid MP4")
	}
	return nil
}

func managedSeedanceUploadID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Accept absolute URLs and path-only managed upload references.
	if strings.HasPrefix(value, "/") {
		value = "https://local.invalid" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = parsed.Path
	}
	prefixes := []string{SeedanceOfficialUploadsEndpoint + "/", SeedancePublicUploadsEndpoint + "/"}
	prefix := ""
	for _, candidate := range prefixes {
		if strings.HasPrefix(path, candidate) {
			prefix = candidate
			break
		}
	}
	if prefix == "" {
		return ""
	}
	id, err := url.PathUnescape(strings.TrimPrefix(path, prefix))
	if err != nil || strings.Contains(id, "/") || !strings.HasPrefix(id, "sdupl_") {
		return ""
	}
	return id
}

func validSeedanceMediaOwner(owner SeedanceMediaOwner) bool {
	return owner.UserID > 0 && owner.APIKeyID > 0 && owner.GroupID > 0
}

func seedanceOutputRecordKey(owner SeedanceMediaOwner, taskID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%s", owner.UserID, owner.APIKeyID, owner.GroupID, strings.TrimSpace(taskID))))
	return seedanceOutputRecordPrefix + hex.EncodeToString(sum[:])
}

func validateSeedanceMediaRemoteURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", errors.New("image URL is invalid")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("image URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("image URL must not include credentials or a fragment")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", errors.New("image URL host is not allowed")
	}
	port := parsed.Port()
	if port != "" && port != "80" && port != "443" {
		return "", errors.New("image URL port must be 80 or 443")
	}
	if parsed.Scheme == "https" && port == "80" {
		return "", errors.New("HTTPS image URL cannot use port 80")
	}
	if parsed.Scheme == "http" && port == "443" {
		return "", errors.New("HTTP image URL cannot use port 443")
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedSeedanceMediaIP(ip) {
		return "", errors.New("image URL host is not allowed")
	}
	return parsed.String(), nil
}

func newSeedanceMediaHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   4,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, fmt.Errorf("resolve media host: %w", err)
		}
		for _, address := range addresses {
			if isBlockedSeedanceMediaIP(address.IP) {
				return nil, fmt.Errorf("resolved media IP %s is not allowed", address.IP.String())
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
	client := &http.Client{Transport: transport}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many image redirects")
		}
		if _, err := validateSeedanceMediaRemoteURL(req.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("HTTPS image URL cannot redirect to HTTP")
		}
		return nil
	}
	return client
}

// newSeedanceLingdongRehostHTTPClient is used only for outbound uploads to
// temporary public hosts. It intentionally skips the SSRF-hardened media fetch
// transport: the destination host is fixed by us, and large multipart uploads
// need longer header timeouts than reference fetches.
func newSeedanceLingdongRehostHTTPClient() *http.Client {
	return &http.Client{
		Timeout: seedanceLingdongRehostTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   4,
			ForceAttemptHTTP2:     true,
		},
	}
}

func isBlockedSeedanceMediaIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return true
	}
	for _, prefix := range seedanceBlockedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

type seedancePublicMediaRecord struct {
	StorageProvider string    `json:"storage_provider"`
	Bucket          string    `json:"bucket"`
	ObjectKey       string    `json:"object_key"`
	ContentType     string    `json:"content_type,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (r seedancePublicMediaRecord) location() AgentArtifactObjectLocation {
	return AgentArtifactObjectLocation{
		StorageProvider: r.StorageProvider,
		Bucket:          r.Bucket,
		ObjectKey:       r.ObjectKey,
	}
}

// seedanceMediaURLNeedsPublicProxy reports whether an upstream vendor is known to
// reject or silently drop this media URL (private COS signatures / managed auth paths).
func seedanceMediaURLNeedsPublicProxy(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	if managedSeedanceUploadID(source) != "" {
		return true
	}
	lower := strings.ToLower(source)
	// Never treat gateway public-media as an acceptable Lingdong upstream URL.
	if strings.Contains(lower, SeedancePublicMediaEndpoint+"/") || strings.Contains(lower, "/v1/videos/public-media/") {
		return true
	}
	// Signed/private cloud object URLs need rewrite. Unsigned public-read COS URLs can pass through
	// for general use, but Lingdong has a separate stricter check below.
	if strings.Contains(lower, "x-amz-signature=") ||
		strings.Contains(lower, "x-amz-algorithm=") ||
		strings.Contains(lower, "x-amz-credential=") ||
		strings.Contains(lower, "q-sign-algorithm=") ||
		strings.Contains(lower, "q-signature=") {
		return true
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return strings.HasPrefix(source, SeedancePublicUploadsEndpoint+"/") ||
			strings.HasPrefix(source, SeedanceOfficialUploadsEndpoint+"/")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = parsed.Path
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimRight(path, "/")), "/local-media") {
		return true
	}
	if strings.HasPrefix(path, SeedancePublicUploadsEndpoint+"/") ||
		strings.HasPrefix(path, SeedanceOfficialUploadsEndpoint+"/") {
		return true
	}
	return false
}

// seedanceMediaURLIsCloudObjectStorage reports whether the URL points at common
// private/public object-storage hosts. Empirically Lingdong accepts the create
// call but silently drops these materials (images/videos become empty arrays).
func seedanceMediaURLIsCloudObjectStorage(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return false
	}
	if strings.Contains(host, "myqcloud.com") ||
		strings.Contains(host, "amazonaws.com") ||
		strings.Contains(host, "aliyuncs.com") ||
		strings.Contains(host, "digitaloceanspaces.com") ||
		strings.Contains(host, "r2.cloudflarestorage.com") ||
		strings.Contains(host, ".cos.") ||
		strings.HasPrefix(host, "cos.") {
		return true
	}
	return false
}

// seedanceMediaURLNeedsLingdongRehost reports whether a reference must be rewritten
// before the mapped multi-modal provider (Pixelle) create call.
// Bare public-read COS/S3/OSS URLs are left as-is (Pixelle can fetch them).
// Auth-gated managed uploads and signed query URLs still need rewriting.
func seedanceMediaURLNeedsLingdongRehost(source string) bool {
	return seedanceMediaURLNeedsPublicProxy(source)
}

// seedanceMediaURLAcceptableForMappedUpstream reports whether a rewritten media
// URL may be sent to the mapped multi-modal provider (Pixelle). Bare public-read
// COS and temporary third-party hosts are accepted. Gateway public-media is also
// accepted as a last-resort rehost output. Signed/managed upload URLs are not.
func seedanceMediaURLAcceptableForMappedUpstream(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" || !isSeedanceHTTPImageURL(source) {
		return false
	}
	if managedSeedanceUploadID(source) != "" {
		return false
	}
	lower := strings.ToLower(source)
	if strings.Contains(lower, "x-amz-signature=") ||
		strings.Contains(lower, "x-amz-algorithm=") ||
		strings.Contains(lower, "x-amz-credential=") ||
		strings.Contains(lower, "q-sign-algorithm=") ||
		strings.Contains(lower, "q-signature=") {
		return false
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = parsed.Path
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimRight(path, "/")), "/local-media") {
		return false
	}
	if strings.HasPrefix(path, SeedancePublicUploadsEndpoint+"/") ||
		strings.HasPrefix(path, SeedanceOfficialUploadsEndpoint+"/") {
		return false
	}
	return true
}

func normalizeSeedancePublicBaseURL(publicBase string) string {
	return strings.TrimRight(strings.TrimSpace(publicBase), "/")
}

func (s *SeedanceMediaService) IssuePublicMediaURL(
	ctx context.Context,
	publicBase string,
	location AgentArtifactObjectLocation,
	contentType string,
) (string, error) {
	if s == nil || s.redisClient == nil {
		return "", infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	publicBase = normalizeSeedancePublicBaseURL(publicBase)
	if publicBase == "" {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "public media base URL is required")
	}
	objectKey := strings.TrimSpace(location.ObjectKey)
	if objectKey == "" {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "Seedance media object location is invalid")
	}
	token := "sdpub_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	expiresAt := s.currentTime().Add(seedancePublicMediaTTL)
	record := seedancePublicMediaRecord{
		StorageProvider: strings.TrimSpace(location.StorageProvider),
		Bucket:          strings.TrimSpace(location.Bucket),
		ObjectKey:       objectKey,
		ContentType:     strings.TrimSpace(contentType),
		ExpiresAt:       expiresAt,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if err := s.redisClient.Set(ctx, seedancePublicMediaRecordPrefix+token, payload, seedancePublicMediaTTL).Err(); err != nil {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "failed to issue public media token").WithCause(err)
	}
	return publicBase + SeedancePublicMediaEndpoint + "/" + url.PathEscape(token), nil
}

func (s *SeedanceMediaService) OpenPublicMedia(ctx context.Context, token, rangeHeader string) (*SeedanceMediaStream, error) {
	if s == nil || s.redisClient == nil || !s.IsConfigured() {
		return nil, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" || !strings.HasPrefix(token, "sdpub_") || strings.Contains(token, "/") {
		return nil, infraerrors.NotFound("media_not_found", "public media token not found")
	}
	payload, err := s.redisClient.Get(ctx, seedancePublicMediaRecordPrefix+token).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, infraerrors.NotFound("media_not_found", "public media token not found")
	}
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to load public media token").WithCause(err)
	}
	var record seedancePublicMediaRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		_ = s.redisClient.Del(ctx, seedancePublicMediaRecordPrefix+token).Err()
		return nil, infraerrors.NotFound("media_not_found", "public media token not found")
	}
	if strings.TrimSpace(record.ObjectKey) == "" {
		return nil, infraerrors.NotFound("media_not_found", "public media token not found")
	}
	mediaRecord := seedanceMediaRecord{
		StorageProvider: record.StorageProvider,
		Bucket:          record.Bucket,
		ObjectKey:       record.ObjectKey,
		ContentType:     record.ContentType,
	}
	stream, err := s.openRecord(ctx, mediaRecord, rangeHeader)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, infraerrors.ServiceUnavailable("media_storage_error", "stored Seedance media is unavailable")
	}
	if stream.Header == nil {
		stream.Header = make(http.Header)
	}
	// Normalize headers for upstream fetchers (some probe HEAD and reject attachment).
	if strings.TrimSpace(stream.Header.Get("Content-Type")) == "" && strings.TrimSpace(record.ContentType) != "" {
		stream.Header.Set("Content-Type", record.ContentType)
	}
	stream.Header.Set("Content-Disposition", "inline")
	if strings.TrimSpace(stream.Header.Get("Accept-Ranges")) == "" {
		stream.Header.Set("Accept-Ranges", "bytes")
	}
	return stream, nil
}

func (s *SeedanceMediaService) resolveLingdongPublicMediaSource(
	ctx context.Context,
	owner SeedanceMediaOwner,
	source, mediaKind string,
) (seedanceMediaRecord, bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return seedanceMediaRecord{}, false, nil
	}
	if uploadID := managedSeedanceUploadID(source); uploadID != "" {
		record, err := s.loadManagedUpload(ctx, owner, uploadID)
		if err != nil {
			return seedanceMediaRecord{}, false, err
		}
		if mediaKind != "" {
			if storedKind := mediaKindFromContentType(record.ContentType); storedKind != "" && storedKind != mediaKind {
				return seedanceMediaRecord{}, false, infraerrors.BadRequest("invalid_media_type", fmt.Sprintf("reference media must be a %s type", mediaKind))
			}
		}
		return record, false, nil
	}
	if location, ok := s.seedanceObjectLocationFromOwnURL(owner, source); ok {
		contentType := ""
		switch mediaKind {
		case "image":
			contentType = "image/png"
		case "video":
			contentType = "video/mp4"
		case "audio":
			contentType = "audio/mpeg"
		}
		return seedanceMediaRecord{
			StorageProvider: location.StorageProvider,
			Bucket:          location.Bucket,
			ObjectKey:       location.ObjectKey,
			ContentType:     contentType,
		}, false, nil
	}
	if !seedanceMediaURLNeedsLingdongRehost(source) {
		return seedanceMediaRecord{}, false, nil
	}
	// Signed/private/cloud remote media that we cannot map to a stored object:
	// fetch onto managed storage so third-party rehost can stream the bytes.
	if !isSeedanceHTTPImageURL(source) {
		return seedanceMediaRecord{}, false, infraerrors.BadRequest("invalid_media_url", "reference media URL must use HTTP(S) or a managed upload")
	}
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		return seedanceMediaRecord{}, false, infraerrors.BadRequest("invalid_media_url", err.Error())
	}
	upload, err := s.fetchAndStoreReferenceMedia(ctx, owner, validated, mediaKind)
	if err != nil {
		return seedanceMediaRecord{}, false, err
	}
	return upload.record, true, nil
}

// PrepareLingdongPublicMedia rewrites reference media that the mapped multi-modal
// provider cannot fetch into publicly fetchable URLs. Preference order:
//  1. already-public URLs including bare public-read COS are left alone
//  2. auth-gated managed uploads / signed URLs are rehosted (third-party then public-media)
//
// Pixelle accepts public-read COS; do not force-rehost bare cloud object URLs.
func (s *SeedanceMediaService) PrepareLingdongPublicMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	info *SeedanceRequestInfo,
	publicBase string,
) (*SeedanceMaterializedImages, error) {
	return s.preparePublicUpstreamMedia(ctx, owner, info, publicBase)
}

// PrepareZhi168PublicMedia prevents zhi168 from receiving gateway-local signed
// URLs that its media fetcher cannot access.
func (s *SeedanceMediaService) PrepareZhi168PublicMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	info *SeedanceRequestInfo,
	publicBase string,
) (*SeedanceMaterializedImages, error) {
	return s.preparePublicUpstreamMedia(ctx, owner, info, publicBase)
}

func (s *SeedanceMediaService) preparePublicUpstreamMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	info *SeedanceRequestInfo,
	publicBase string,
) (*SeedanceMaterializedImages, error) {
	if info == nil {
		return nil, infraerrors.BadRequest("invalid_request", "Seedance request info is required")
	}
	// Include videos for the mapped Lingdong path even when MaterializeImages
	// skipped them (face-ref models are not ximei/fallback-eligible).
	targets := seedanceRequestMediaTargets(info, true)
	needsRehost := false
	for _, target := range targets {
		if target.url == nil {
			continue
		}
		source := strings.TrimSpace(*target.url)
		if source == "" {
			continue
		}
		if seedanceMediaURLNeedsLingdongRehost(source) {
			needsRehost = true
			break
		}
	}
	// Already-public third-party media can pass through without managed storage.
	if s == nil || !s.IsConfigured() || s.redisClient == nil {
		if needsRehost {
			return nil, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
		}
		return nil, nil
	}
	publicBase = normalizeSeedancePublicBaseURL(publicBase)
	materialized := &SeedanceMaterializedImages{service: s}
	cleanupOnError := func(err error) (*SeedanceMaterializedImages, error) {
		materialized.Cleanup(context.Background())
		return nil, err
	}
	stored := make(map[string]AgentArtifactObjectLocation, len(info.StoredMedia))
	for _, reference := range info.StoredMedia {
		location := AgentArtifactObjectLocation{
			StorageProvider: strings.TrimSpace(reference.StorageProvider),
			Bucket:          strings.TrimSpace(reference.Bucket),
			ObjectKey:       strings.TrimLeft(strings.TrimSpace(reference.ObjectKey), "/"),
		}
		if !seedanceObjectKeyBelongsToOwner(location.ObjectKey, owner) {
			return cleanupOnError(infraerrors.BadRequest("invalid_media", "stored reference media is invalid"))
		}
		key := seedanceStoredMediaKey(reference.Slot, reference.Index)
		if _, exists := stored[key]; exists {
			return cleanupOnError(infraerrors.BadRequest("invalid_media", "stored reference media is duplicated"))
		}
		stored[key] = location
	}

	for _, target := range targets {
		if target.url == nil {
			continue
		}
		source := strings.TrimSpace(*target.url)
		if source == "" {
			continue
		}
		if !seedanceMediaURLNeedsLingdongRehost(source) {
			continue
		}
		var record seedanceMediaRecord
		temporary := false
		if location, ok := stored[seedanceStoredMediaKey(target.slot, target.index)]; ok {
			record = seedanceMediaRecord{
				StorageProvider: location.StorageProvider,
				Bucket:          location.Bucket,
				ObjectKey:       location.ObjectKey,
			}
		} else {
			var err error
			record, temporary, err = s.resolveLingdongPublicMediaSource(ctx, owner, source, target.kind)
			if err != nil {
				return cleanupOnError(err)
			}
		}
		if strings.TrimSpace(record.ObjectKey) == "" {
			// Needs rewrite but could not resolve storage: fail loud rather than
			// submitting COS/auth-gated links that Lingdong silently drops.
			return cleanupOnError(infraerrors.ServiceUnavailable("media_rehost_failed", "failed to rehost reference media for upstream"))
		}
		if temporary {
			location := record.location()
			materialized.objects = append(materialized.objects, location)
			info.StoredMedia = append(info.StoredMedia, SeedanceStoredMediaReference{
				Slot:                  target.slot,
				Index:                 target.index,
				StorageProvider:       location.StorageProvider,
				Bucket:                location.Bucket,
				ObjectKey:             location.ObjectKey,
				DeleteAfterSettlement: true,
			})
		}

		// Prefer bare public-read COS for Pixelle; otherwise third-party / public-media.
		// Signed cloud URLs must never remain on the outbound create payload.
		publicURL, rehostedLoc, rehostErr := s.rehostLingdongPublicMedia(ctx, record, target.kind, publicBase)
		if rehostErr != nil {
			return cleanupOnError(rehostErr)
		}
		if strings.TrimSpace(publicURL) == "" {
			return cleanupOnError(infraerrors.ServiceUnavailable("media_rehost_failed", "failed to rehost reference media for upstream"))
		}
		if !seedanceMediaURLAcceptableForMappedUpstream(publicURL) {
			return cleanupOnError(infraerrors.ServiceUnavailable("media_rehost_failed", "failed to rehost reference media for upstream"))
		}
		if rehostedLoc != nil && strings.TrimSpace(rehostedLoc.ObjectKey) != "" {
			materialized.objects = append(materialized.objects, *rehostedLoc)
			info.StoredMedia = append(info.StoredMedia, SeedanceStoredMediaReference{
				Slot:                  target.slot,
				Index:                 target.index,
				StorageProvider:       rehostedLoc.StorageProvider,
				Bucket:                rehostedLoc.Bucket,
				ObjectKey:             rehostedLoc.ObjectKey,
				DeleteAfterSettlement: true,
			})
		}
		*target.url = publicURL
	}
	if len(materialized.objects) == 0 {
		return nil, nil
	}
	return materialized, nil
}

// unsignedPublicObjectURL returns an unsigned HTTPS object URL assuming the
// underlying bucket/object is public-read. Built by presigning then stripping
// query credentials so Lingdong receives a bare public link.
func (s *SeedanceMediaService) unsignedPublicObjectURL(ctx context.Context, record seedanceMediaRecord) (string, error) {
	if s == nil {
		return "", infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	if strings.TrimSpace(record.ObjectKey) == "" {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "Seedance media object location is invalid")
	}
	if normalizeAgentArtifactProvider(record.StorageProvider) == "local" {
		return "", infraerrors.ServiceUnavailable("media_rehost_failed", "local media storage does not provide public object URLs")
	}
	signed, err := s.presignLocation(ctx, record.location())
	if err != nil {
		return "", err
	}
	publicURL := stripSeedanceSignedQuery(signed)
	if !isSeedanceHTTPImageURL(publicURL) {
		return "", infraerrors.ServiceUnavailable("media_rehost_failed", "failed to build public object URL")
	}
	low := strings.ToLower(publicURL)
	if strings.Contains(low, "x-amz-signature=") || strings.Contains(low, "q-signature=") {
		return "", infraerrors.ServiceUnavailable("media_rehost_failed", "public object URL still contains a signature")
	}
	return publicURL, nil
}

func stripSeedanceSignedQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// rehostLingdongPublicMedia turns auth-gated stored media into URLs the mapped
// multi-modal provider (Pixelle) can fetch without credentials. Preference:
//  1. bare public-read COS/S3/OSS object URL (strip signature; production default)
//  2. temporary third-party hosts (litterbox/catbox) when reachable
//  3. gateway public-media token URL (last-resort fallback)
func (s *SeedanceMediaService) rehostLingdongPublicMedia(
	ctx context.Context,
	record seedanceMediaRecord,
	mediaKind string,
	publicBase string,
) (string, *AgentArtifactObjectLocation, error) {
	if s == nil {
		return "", nil, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}

	var failures []string

	// Primary path for Pixelle: public-read object storage bare URL.
	// Avoid re-uploading large images/videos to third-party hosts when the bucket
	// is already public-read private-write.
	if cosURL, cosErr := s.unsignedPublicObjectURL(ctx, record); cosErr == nil {
		cosURL = strings.TrimSpace(cosURL)
		if cosURL != "" && seedanceMediaURLAcceptableForMappedUpstream(cosURL) {
			return cosURL, nil, nil
		}
		if cosURL != "" {
			failures = append(failures, "public-cos: still auth-gated after strip")
		}
	} else if cosErr != nil {
		failures = append(failures, "public-cos: "+cosErr.Error())
	}

	stream, err := s.openRecord(ctx, record, "")
	if err != nil {
		return "", nil, err
	}
	if stream == nil || stream.Body == nil {
		return "", nil, infraerrors.ServiceUnavailable("media_storage_error", "stored Seedance media is unavailable")
	}
	defer func() { _ = stream.Body.Close() }()

	limit := s.maxVideoBytes
	if limit <= 0 {
		limit = seedanceDefaultVideoBytes
	}
	switch mediaKind {
	case "image":
		limit = SeedanceMaxImageBytes
	case "audio":
		limit = huiquMaxAudioBytes
	}
	limited := io.LimitReader(stream.Body, limit+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return "", nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to read stored Seedance media").WithCause(err)
	}
	if int64(len(payload)) > limit {
		return "", nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", "reference media exceeds rehost size limit")
	}
	if len(payload) == 0 {
		return "", nil, infraerrors.BadRequest("invalid_media", "reference media is empty")
	}

	contentType := strings.TrimSpace(record.ContentType)
	if contentType == "" && stream.Header != nil {
		contentType = strings.TrimSpace(stream.Header.Get("Content-Type"))
	}
	if contentType == "" {
		contentType = seedanceDefaultContentTypeForKind(mediaKind)
	}
	filename := seedanceRehostFilename(mediaKind, contentType, record.ObjectKey)

	// Secondary: third-party hosts (or test hook).
	if s.lingdongRehostFn != nil {
		hookURL, hookErr := s.lingdongRehostFn(ctx, filename, contentType, payload)
		if hookErr != nil {
			failures = append(failures, "third-party: "+hookErr.Error())
		} else if strings.TrimSpace(hookURL) == "" || !seedanceMediaURLAcceptableForMappedUpstream(hookURL) {
			failures = append(failures, "third-party: empty or auth-gated url")
		} else {
			return hookURL, nil, nil
		}
	} else {
		publicURL, extErr := s.uploadLingdongLitterbox(ctx, filename, contentType, payload)
		if extErr == nil && strings.TrimSpace(publicURL) != "" && seedanceMediaURLAcceptableForMappedUpstream(publicURL) {
			return publicURL, nil, nil
		}
		if extErr != nil {
			failures = append(failures, "third-party: "+extErr.Error())
		} else {
			failures = append(failures, "third-party: empty or auth-gated url")
		}
	}

	// Last resort: short-lived public-media URL on our gateway.
	publicBase = normalizeSeedancePublicBaseURL(publicBase)
	if publicBase != "" {
		loc := record.location()
		mediaURL, issueErr := s.IssuePublicMediaURL(ctx, publicBase, loc, contentType)
		if issueErr == nil && strings.TrimSpace(mediaURL) != "" && seedanceMediaURLAcceptableForMappedUpstream(mediaURL) {
			return mediaURL, nil, nil
		}
		if issueErr != nil {
			failures = append(failures, "public-media: "+issueErr.Error())
		} else {
			failures = append(failures, "public-media: empty or unusable url")
		}
	} else {
		failures = append(failures, "public-media: public base url missing")
	}

	detail := strings.Join(failures, "; ")
	if detail == "" {
		detail = "all rehost strategies failed"
	}
	return "", nil, infraerrors.ServiceUnavailable("media_rehost_failed", "failed to rehost reference media for upstream").WithCause(fmt.Errorf("%s", detail))
}

// uploadLingdongPublicObject stores a public-read copy under seedance/public-rehost/
// and returns an unsigned HTTPS URL for upstream fetch.
func (s *SeedanceMediaService) uploadLingdongPublicObject(
	ctx context.Context,
	filename, contentType string,
	payload []byte,
) (string, AgentArtifactObjectLocation, error) {
	var zero AgentArtifactObjectLocation
	if s == nil || s.store == nil || !s.store.IsConfigured() {
		return "", zero, infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "media.bin"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	objectKey := path.Join("seedance", "public-rehost", strings.ReplaceAll(uuid.NewString(), "-", ""), filename)
	result, err := s.store.Put(ctx, AgentArtifactStorePutInput{
		Key:         objectKey,
		Body:        bytes.NewReader(payload),
		ContentType: contentType,
		SizeBytes:   int64(len(payload)),
		PublicRead:  true,
		Metadata: map[string]string{
			"purpose": "lingdong-public-rehost",
		},
	})
	if err != nil {
		return "", zero, err
	}
	if result == nil {
		return "", zero, infraerrors.ServiceUnavailable("media_rehost_failed", "public rehost put returned empty result")
	}
	publicURL := strings.TrimSpace(result.ObjectURL)
	if !isSeedanceHTTPImageURL(publicURL) {
		return "", zero, infraerrors.ServiceUnavailable("media_rehost_failed", "public rehost did not produce an HTTP URL")
	}
	low := strings.ToLower(publicURL)
	if strings.Contains(low, "x-amz-signature=") || strings.Contains(low, "q-signature=") {
		return "", zero, infraerrors.ServiceUnavailable("media_rehost_failed", "public rehost URL still contains a signature")
	}
	loc := AgentArtifactObjectLocation{
		StorageProvider: strings.TrimSpace(result.Provider),
		Bucket:          strings.TrimSpace(result.Bucket),
		ObjectKey:       strings.TrimSpace(result.ObjectKey),
	}
	if loc.ObjectKey == "" {
		loc.ObjectKey = objectKey
	}
	return publicURL, loc, nil
}

func seedanceDefaultContentTypeForKind(mediaKind string) string {
	switch mediaKind {
	case "image":
		return "image/png"
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func seedanceRehostFilename(mediaKind, contentType, objectKey string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(objectKey)))
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		switch mediaKind {
		case "image":
			ext = ".png"
		case "video":
			ext = ".mp4"
		case "audio":
			ext = ".mp3"
		default:
			ext = ".bin"
		}
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return "seedance-" + mediaKind + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12] + ext
}

func (s *SeedanceMediaService) uploadLingdongLitterbox(
	ctx context.Context,
	filename, contentType string,
	payload []byte,
) (string, error) {
	if s == nil {
		return "", infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "media.bin"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	hosts := []struct {
		name string
		url  string
		mode string
	}{
		// Prefer litterbox (empirically accepted by Lingdong multi-modal refs).
		{name: "litterbox", url: seedanceLingdongRehostURL, mode: "litterbox"},
		// Permanent catbox when temporary litterbox is blocked.
		{name: "catbox", url: seedanceLingdongRehostURLCatbox, mode: "catbox"},
		// Secondary public host when catbox family is unreachable from the server region.
		{name: "0x0", url: seedanceLingdongRehostURLAlt, mode: "0x0"},
	}
	var failures []string
	for _, host := range hosts {
		url, err := s.postLingdongRehostMultipart(ctx, host.url, filename, contentType, payload, host.mode)
		if err == nil && strings.TrimSpace(url) != "" {
			return url, nil
		}
		if err != nil {
			failures = append(failures, host.name+": "+err.Error())
			continue
		}
		failures = append(failures, host.name+": empty url")
	}
	detail := strings.Join(failures, "; ")
	if detail == "" {
		detail = "all rehost hosts failed"
	}
	return "", infraerrors.ServiceUnavailable("media_rehost_failed", "failed to rehost reference media for upstream").WithCause(fmt.Errorf("%s", detail))
}

func (s *SeedanceMediaService) postLingdongRehostMultipart(
	ctx context.Context,
	endpoint, filename, contentType string,
	payload []byte,
	mode string,
) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fieldName := "file"
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "litterbox":
		_ = writer.WriteField("reqtype", "fileupload")
		_ = writer.WriteField("time", seedanceLingdongRehostTTL)
		fieldName = "fileToUpload"
	case "catbox":
		_ = writer.WriteField("reqtype", "fileupload")
		fieldName = "fileToUpload"
	default:
		// 0x0.st style: single file field.
		fieldName = "file"
	}
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(payload); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	reqCtx, cancel := context.WithTimeout(ctx, seedanceLingdongRehostTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "text/plain, */*")
	req.ContentLength = int64(body.Len())
	client := newSeedanceLingdongRehostHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	text := strings.TrimSpace(string(raw))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("rehost HTTP %d: %s", resp.StatusCode, truncateSeedanceLog(text, 120))
	}
	if !strings.HasPrefix(strings.ToLower(text), "http://") && !strings.HasPrefix(strings.ToLower(text), "https://") {
		return "", fmt.Errorf("rehost did not return URL: %s", truncateSeedanceLog(text, 120))
	}
	if i := strings.IndexAny(text, "\r\n\t "); i >= 0 {
		text = text[:i]
	}
	if !isSeedanceHTTPImageURL(text) {
		return "", fmt.Errorf("rehost returned invalid URL: %s", truncateSeedanceLog(text, 120))
	}
	return text, nil
}

func truncateSeedanceLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
