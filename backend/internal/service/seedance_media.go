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
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
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
	SeedanceMaxImageBytes             int64 = 10 << 20
	SeedanceMaxImagePixels                  = 40_000_000
	SeedanceMaxImageDimension               = 8192
	SeedanceUploadBodyOverhead        int64 = 2 << 20
	seedanceDefaultVideoBytes         int64 = 512 << 20
	seedanceUploadRecordTTL                 = 24 * time.Hour
	seedanceOutputRecordTTL                 = 7 * 24 * time.Hour
	seedanceDefaultPresignTTL               = time.Hour
	seedanceImageFetchTimeout               = 60 * time.Second
	seedanceOutputArchiveLockTTL            = 30 * time.Minute
	seedanceObjectCleanupTimeout            = 10 * time.Second
	seedanceMaxConcurrentArchives           = 2
	seedanceDefaultMediaConcurrency         = 2
	seedanceMaxMediaConcurrency             = 8
	seedanceMediaLeaseTTL                   = 5 * time.Minute
	seedanceMediaLeaseRefreshInterval       = time.Minute
	seedanceMaxProcessMediaStreams          = 32
)

const (
	seedanceUploadRecordPrefix = "seedance:media:upload:"
	seedanceOutputRecordPrefix = "seedance:media:output:"
	seedanceOutputLockPrefix   = "seedance:media:archive-lock:"
	seedanceMediaIOPrefix      = "seedance:media:io:"
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
}

type SeedanceImageUpload struct {
	UploadID    string
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

func (s *SeedanceMediaService) AcquireMediaIO(ctx context.Context, owner SeedanceMediaOwner, requestedConcurrency int) (func(), error) {
	if !validSeedanceMediaOwner(owner) {
		return nil, infraerrors.BadRequest("invalid_media_owner", "Seedance media owner is invalid")
	}
	if s == nil || s.redisClient == nil {
		return nil, infraerrors.ServiceUnavailable("media_concurrency_unavailable", "Seedance media concurrency control is unavailable")
	}
	limit := requestedConcurrency
	if limit <= 0 {
		limit = seedanceDefaultMediaConcurrency
	}
	if limit > seedanceMaxMediaConcurrency {
		limit = seedanceMaxMediaConcurrency
	}
	nowMillis := s.currentTime().UnixMilli()
	expiresMillis := nowMillis + seedanceMediaLeaseTTL.Milliseconds()
	keyDigest := sha256.Sum256([]byte(strconv.FormatInt(owner.UserID, 10)))
	key := seedanceMediaIOPrefix + hex.EncodeToString(keyDigest[:])
	token := uuid.NewString()
	result, err := seedanceAcquireMediaIOScript.Run(
		ctx,
		s.redisClient,
		[]string{key},
		nowMillis,
		limit,
		expiresMillis,
		token,
		(seedanceMediaLeaseTTL + time.Minute).Milliseconds(),
	).Int()
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("media_concurrency_unavailable", "Seedance media concurrency control is unavailable")
	}
	if result != 1 {
		return nil, infraerrors.TooManyRequests("media_concurrency_exceeded", "Too many concurrent Seedance media requests")
	}
	if s.mediaSlots != nil {
		select {
		case s.mediaSlots <- struct{}{}:
		default:
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.redisClient.ZRem(releaseCtx, key, token).Err()
			cancel()
			return nil, infraerrors.TooManyRequests("media_concurrency_exceeded", "Too many concurrent Seedance media requests")
		}
	}
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
		return nil, infraerrors.BadRequest("image_required", "image file is required")
	}
	if input.SizeBytes > SeedanceMaxImageBytes {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", "image must not exceed 10 MiB")
	}

	uploadID := "sdupl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	kind := "task"
	if input.Persistent {
		kind = "staged"
	}
	record, err := s.storeImage(ctx, input.Owner, uploadID, kind, input.Body, input.SizeBytes, input.ContentType)
	if err != nil {
		return nil, err
	}
	if input.Persistent {
		if err := s.saveRecord(ctx, seedanceUploadRecordPrefix+uploadID, record, seedanceUploadRecordTTL); err != nil {
			s.deleteObjectBestEffort(ctx, record.location())
			return nil, infraerrors.ServiceUnavailable("media_storage_error", "failed to register Seedance image upload").WithCause(err)
		}
	}
	return &SeedanceImageUpload{
		UploadID:    uploadID,
		ContentType: record.ContentType,
		SizeBytes:   record.SizeBytes,
		SHA256:      record.SHA256,
		ExpiresAt:   record.ExpiresAt,
		record:      record,
	}, nil
}

func (s *SeedanceMediaService) UploadDataURI(ctx context.Context, owner SeedanceMediaOwner, value string, persistent bool) (*SeedanceImageUpload, error) {
	mediaType, encoded, err := splitSeedanceImageDataURI(value)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_base64", err.Error())
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(int(SeedanceMaxImageBytes)) {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", "decoded image must not exceed 10 MiB")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_image_base64", "image Base64 payload is invalid")
	}
	if int64(len(decoded)) > SeedanceMaxImageBytes {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", "decoded image must not exceed 10 MiB")
	}
	return s.UploadImage(ctx, SeedanceImageUploadInput{
		Owner:       owner,
		Body:        bytes.NewReader(decoded),
		SizeBytes:   int64(len(decoded)),
		ContentType: mediaType,
		Persistent:  persistent,
	})
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

	values := []*string{&info.StartFrameURL, &info.EndFrameURL}
	for i := range info.References {
		values = append(values, &info.References[i].URL)
	}
	for _, target := range values {
		if target == nil || strings.TrimSpace(*target) == "" {
			continue
		}
		resolved, location, err := s.materializeImage(ctx, owner, *target)
		if err != nil {
			return cleanupOnError(err)
		}
		*target = resolved
		if location != nil {
			materialized.objects = append(materialized.objects, *location)
		}
	}
	return materialized, nil
}

func (s *SeedanceMediaService) materializeImage(ctx context.Context, owner SeedanceMediaOwner, source string) (string, *AgentArtifactObjectLocation, error) {
	source = strings.TrimSpace(source)
	if uploadID := managedSeedanceUploadID(source); uploadID != "" {
		record, err := s.loadManagedUpload(ctx, owner, uploadID)
		if err != nil {
			return "", nil, err
		}
		signed, err := s.presignRecord(ctx, record)
		return signed, nil, err
	}
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		upload, err := s.UploadDataURI(ctx, owner, source, false)
		if err != nil {
			return "", nil, err
		}
		signed, err := s.presignRecord(ctx, upload.record)
		if err != nil {
			s.deleteObjectBestEffort(ctx, upload.record.location())
			return "", nil, err
		}
		location := upload.record.location()
		return signed, &location, nil
	}
	if !isSeedanceHTTPImageURL(source) {
		return "", nil, infraerrors.BadRequest("invalid_image_url", "image URL must be HTTP(S), a managed upload URL, or a supported Base64 data URI")
	}
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		return "", nil, infraerrors.BadRequest("invalid_image_url", err.Error())
	}
	if !s.IsConfigured() {
		return validated, nil, nil
	}
	upload, err := s.fetchAndStoreImage(ctx, owner, validated)
	if err != nil {
		return "", nil, err
	}
	signed, err := s.presignRecord(ctx, upload.record)
	if err != nil {
		s.deleteObjectBestEffort(ctx, upload.record.location())
		return "", nil, err
	}
	location := upload.record.location()
	return signed, &location, nil
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

func (s *SeedanceMediaService) storeImage(ctx context.Context, owner SeedanceMediaOwner, uploadID, kind string, body io.Reader, sizeHint int64, declaredType string) (seedanceMediaRecord, error) {
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "image-*")
	if err != nil {
		return seedanceMediaRecord{}, err
	}
	path := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(path)
	}()

	hasher := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(io.LimitReader(body, SeedanceMaxImageBytes+1), hasher))
	if err != nil {
		return seedanceMediaRecord{}, fmt.Errorf("read Seedance image: %w", err)
	}
	if written > SeedanceMaxImageBytes {
		return seedanceMediaRecord{}, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", "image must not exceed 10 MiB")
	}
	if sizeHint > 0 && written != sizeHint {
		return seedanceMediaRecord{}, infraerrors.BadRequest("image_size_mismatch", "image size does not match Content-Length")
	}
	mediaType, extension, err := inspectSeedanceImage(tmp, declaredType)
	if err != nil {
		return seedanceMediaRecord{}, err
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
		return seedanceMediaRecord{}, infraerrors.ServiceUnavailable("media_storage_error", "failed to store Seedance image").WithCause(err)
	}
	if put == nil {
		return seedanceMediaRecord{}, infraerrors.ServiceUnavailable("media_storage_error", "failed to store Seedance image")
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
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "image_too_large", "image must not exceed 10 MiB")
	}
	return s.UploadImage(ctx, SeedanceImageUploadInput{
		Owner:       owner,
		Body:        resp.Body,
		SizeBytes:   resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
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
	if !s.IsConfigured() {
		return "", infraerrors.ServiceUnavailable("media_storage_not_configured", "Seedance media storage is not configured")
	}
	presignTTL, _ := s.runtimeStorageLimits(ctx)
	signed, err := s.store.PresignGetObject(ctx, record.location(), presignTTL)
	if err != nil {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "failed to sign Seedance media URL")
	}
	if !isSeedanceHTTPImageURL(signed) {
		return "", infraerrors.ServiceUnavailable("media_storage_error", "Seedance media storage did not return an HTTP(S) URL")
	}
	return signed, nil
}

func (s *SeedanceMediaService) openRecord(ctx context.Context, record seedanceMediaRecord, rangeHeader string) (*SeedanceMediaStream, error) {
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
	detected := normalizeSeedanceInlineImageMediaType(http.DetectContentType(header[:n]))
	if detected == "" {
		return "", "", infraerrors.BadRequest("unsupported_image_type", "image must be PNG, JPEG, or WebP")
	}
	declared, err := normalizeSeedanceDeclaredImageType(declaredType)
	if err != nil {
		return "", "", err
	}
	if declared != "" && declared != detected {
		return "", "", infraerrors.BadRequest("image_type_mismatch", "declared image type does not match file content")
	}
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
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return ""
	}
	prefix := SeedanceOfficialUploadsEndpoint + "/"
	if !strings.HasPrefix(parsed.EscapedPath(), prefix) {
		return ""
	}
	id, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), prefix))
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
