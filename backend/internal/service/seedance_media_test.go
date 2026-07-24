package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestParseSeedanceCreateRequestAcceptsInlineImageForms(t *testing.T) {
	pngBytes := seedanceMediaTestImage(t, "png", 2, 2)
	encoded := base64.StdEncoding.EncodeToString(pngBytes)
	dataURI := "data:image/png;base64," + encoded

	tests := []struct {
		name       string
		imageValue any
	}{
		{name: "data URI string", imageValue: dataURI},
		{name: "base64 object", imageValue: map[string]any{"base64": encoded, "media_type": "image/png"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"model":      "seedance-2.0",
				"duration":   4,
				"resolution": "720p",
				"content": []any{
					map[string]any{"type": "text", "text": "A paper boat crosses a puddle."},
					map[string]any{"type": "image_url", "image_url": tt.imageValue},
				},
			})
			require.NoError(t, err)

			info, err := ParseSeedanceCreateRequest(body)
			require.NoError(t, err)
			require.Equal(t, dataURI, info.StartFrameURL)
			require.True(t, info.HasInlineImages())
		})
	}
}

func TestParseSeedanceCreateRequestRejectsURLAndBase64Together(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"model": "seedance-2.0",
		"content": []any{
			map[string]any{"type": "text", "text": "Animate the reference."},
			map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url":        "https://images.example.com/reference.png",
					"base64":     "aGVsbG8=",
					"media_type": "image/png",
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = ParseSeedanceCreateRequest(body)
	require.EqualError(t, err, "image_url.url and image_url.base64 are mutually exclusive")
}

func TestParseSeedanceCreateRequestRejectsTrailingJSONAndUnknownImageFields(t *testing.T) {
	valid := `{"model":"seedance-2.0","content":[{"type":"text","text":"Animate it"}]}`
	_, err := ParseSeedanceCreateRequest([]byte(valid + `{}`))
	require.EqualError(t, err, "request body must contain exactly one JSON object")

	unknownImageField := `{"model":"seedance-2.0","content":[{"type":"text","text":"Animate it"},{"type":"image_url","image_url":{"url":"https://images.example.com/input.png","unexpected":"value"}}]}`
	_, err = ParseSeedanceCreateRequest([]byte(unknownImageField))
	require.EqualError(t, err, "image_url must be a URL/data URI string or an object containing url or base64")
}

func TestSeedanceUpstreamBodyRejectsUnmaterializedInlineImages(t *testing.T) {
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(seedanceMediaTestImage(t, "png", 1, 1))
	tests := []struct {
		name string
		info SeedanceRequestInfo
	}{
		{
			name: "first frame",
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 4, StartFrameURL: dataURI},
		},
		{
			name: "first and last frames",
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 4, StartFrameURL: "https://images.example.com/start.png", EndFrameURL: dataURI},
		},
		{
			name: "reference image",
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 4, References: []SeedanceReferenceImage{{URL: dataURI, Strength: "MID"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.info.UpstreamBody("seedance-2.0")
			require.Error(t, err)
			require.Contains(t, err.Error(), "must be uploaded before forwarding")
		})
	}
}

func TestSeedanceMediaUploadAcceptsPNGJPEGAndWebP(t *testing.T) {
	webpBytes, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)

	tests := []struct {
		name        string
		contentType string
		extension   string
		body        []byte
	}{
		{name: "PNG", contentType: "image/png", extension: ".png", body: seedanceMediaTestImage(t, "png", 2, 2)},
		{name: "JPEG", contentType: "image/jpeg", extension: ".jpg", body: seedanceMediaTestImage(t, "jpeg", 2, 2)},
		{name: "WebP", contentType: "image/webp", extension: ".webp", body: webpBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newSeedanceMediaMemoryStore()
			service := NewSeedanceMediaService(store, nil, nil)
			upload, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
				Owner:       seedanceMediaTestOwner(),
				Body:        bytes.NewReader(tt.body),
				SizeBytes:   int64(len(tt.body)),
				ContentType: tt.contentType,
			})
			require.NoError(t, err)
			require.Equal(t, tt.contentType, upload.ContentType)
			require.Equal(t, int64(len(tt.body)), upload.SizeBytes)
			require.Len(t, store.puts, 1)
			require.Equal(t, tt.contentType, store.puts[0].ContentType)
			require.True(t, strings.HasPrefix(store.puts[0].Key, "seedance/inputs/task/"))
			require.True(t, strings.HasSuffix(store.puts[0].Key, tt.extension))
			require.Equal(t, tt.body, store.objects[store.puts[0].Key])
		})
	}
}

func TestSeedanceMediaUploadRejectsInvalidImages(t *testing.T) {
	pngBytes := seedanceMediaTestImage(t, "png", 2, 2)
	tooWidePNG := seedanceMediaTestImage(t, "png", SeedanceMaxImageDimension+1, 1)
	corruptPNG := append([]byte(nil), pngBytes[:16]...)

	tests := []struct {
		name       string
		body       []byte
		size       int64
		declared   string
		wantCode   int
		wantReason string
	}{
		{
			name:       "declared MIME mismatch",
			body:       pngBytes,
			size:       int64(len(pngBytes)),
			declared:   "image/jpeg",
			wantCode:   http.StatusBadRequest,
			wantReason: "image_type_mismatch",
		},
		{
			name:       "unsupported declared MIME",
			body:       pngBytes,
			size:       int64(len(pngBytes)),
			declared:   "image/gif",
			wantCode:   http.StatusBadRequest,
			wantReason: "unsupported_image_type",
		},
		{
			name:       "corrupt image",
			body:       corruptPNG,
			size:       int64(len(corruptPNG)),
			declared:   "image/png",
			wantCode:   http.StatusBadRequest,
			wantReason: "invalid_image",
		},
		{
			name:       "oversized image",
			body:       pngBytes,
			size:       SeedanceMaxImageBytes + 1,
			declared:   "image/png",
			wantCode:   http.StatusRequestEntityTooLarge,
			wantReason: "image_too_large",
		},
		{
			name:       "invalid dimensions",
			body:       tooWidePNG,
			size:       int64(len(tooWidePNG)),
			declared:   "image/png",
			wantCode:   http.StatusBadRequest,
			wantReason: "image_dimensions_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newSeedanceMediaMemoryStore()
			service := NewSeedanceMediaService(store, nil, nil)
			_, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
				Owner:       seedanceMediaTestOwner(),
				Body:        bytes.NewReader(tt.body),
				SizeBytes:   tt.size,
				ContentType: tt.declared,
			})
			require.Error(t, err)
			require.Equal(t, tt.wantCode, infraerrors.Code(err))
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
			require.Empty(t, store.puts)
		})
	}
}

func TestSeedanceMediaUploadRejectsStreamingBodyOverLimit(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)

	_, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
		Owner: seedanceMediaTestOwner(),
		Body:  io.LimitReader(seedanceMediaZeroReader{}, SeedanceMaxImageBytes+1),
	})
	require.Error(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, infraerrors.Code(err))
	require.Equal(t, "image_too_large", infraerrors.Reason(err))
	require.Empty(t, store.puts)
}

func TestSeedanceMediaUploadDataURIStrictBase64(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	pngBytes := seedanceMediaTestImage(t, "png", 2, 2)

	upload, err := service.UploadDataURI(
		context.Background(),
		seedanceMediaTestOwner(),
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngBytes),
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "image/png", upload.ContentType)

	_, err = service.UploadDataURI(context.Background(), seedanceMediaTestOwner(), "data:image/png;base64,not valid base64", false)
	require.Error(t, err)
	require.Equal(t, "invalid_image_base64", infraerrors.Reason(err))
}

func TestSeedanceMediaUploadDataURIBoundaryUsesDecodedSize(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	pngBytes := seedanceMediaTestImage(t, "png", 1, 1)
	exactLimit := make([]byte, int(SeedanceMaxImageBytes))
	copy(exactLimit, pngBytes)

	upload, err := service.UploadDataURI(
		context.Background(),
		seedanceMediaTestOwner(),
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(exactLimit),
		false,
	)
	require.NoError(t, err)
	require.Equal(t, SeedanceMaxImageBytes, upload.SizeBytes)

	overLimit := append(exactLimit, 0)
	_, err = service.UploadDataURI(
		context.Background(),
		seedanceMediaTestOwner(),
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(overLimit),
		false,
	)
	require.Error(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, infraerrors.Code(err))
	require.Equal(t, "image_too_large", infraerrors.Reason(err))
}

func TestSeedanceManagedUploadRequiresExactOwner(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, redisClient)
	owner := seedanceMediaTestOwner()
	pngBytes := seedanceMediaTestImage(t, "png", 2, 2)
	upload, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
		Owner:       owner,
		Body:        bytes.NewReader(pngBytes),
		SizeBytes:   int64(len(pngBytes)),
		ContentType: "image/png",
		Persistent:  true,
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(upload.UploadID, "sdupl_"))

	record, err := service.loadManagedUpload(context.Background(), owner, upload.UploadID)
	require.NoError(t, err)
	require.Equal(t, owner.UserID, record.UserID)
	require.Equal(t, owner.APIKeyID, record.APIKeyID)
	require.Equal(t, owner.GroupID, record.GroupID)

	otherOwners := []SeedanceMediaOwner{
		{UserID: owner.UserID + 1, APIKeyID: owner.APIKeyID, GroupID: owner.GroupID},
		{UserID: owner.UserID, APIKeyID: owner.APIKeyID + 1, GroupID: owner.GroupID},
		{UserID: owner.UserID, APIKeyID: owner.APIKeyID, GroupID: owner.GroupID + 1},
	}
	for _, other := range otherOwners {
		_, err := service.loadManagedUpload(context.Background(), other, upload.UploadID)
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
		require.Equal(t, "upload_not_found", infraerrors.Reason(err))
	}

	managedURL := "https://gateway.example.com/api/v3/contents/generations/uploads/" + upload.UploadID
	resolved, location, err := service.materializeImage(context.Background(), owner, managedURL)
	require.NoError(t, err)
	require.Nil(t, location)
	require.Contains(t, resolved, url.PathEscape(record.ObjectKey))

	_, _, err = service.materializeImage(context.Background(), otherOwners[0], managedURL)
	require.Error(t, err)
	require.Equal(t, "upload_not_found", infraerrors.Reason(err))
}

func TestSeedanceMaterializeImagesStoresEveryInlineImage(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(seedanceMediaTestImage(t, "png", 2, 2))
	info := &SeedanceRequestInfo{
		StartFrameURL: dataURI,
		EndFrameURL:   dataURI,
	}

	materialized, err := service.MaterializeImages(context.Background(), seedanceMediaTestOwner(), info)
	require.NoError(t, err)
	require.True(t, isSeedanceHTTPImageURL(info.StartFrameURL))
	require.True(t, isSeedanceHTTPImageURL(info.EndFrameURL))
	require.Len(t, materialized.objects, 2)
	require.Len(t, store.puts, 2)

	materialized.Cleanup(context.Background())
	require.Len(t, store.deleted, 2)
}

func TestSeedanceCaptureOutputUsesDeterministicOwnerScopedObjectKey(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, redisClient)
	owner := seedanceMediaTestOwner()
	taskID := "vidjob_sensitive_123"
	video := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}

	for range 2 {
		captured, err := service.CaptureAndStoreOutput(
			context.Background(),
			owner,
			taskID,
			"video/mp4",
			int64(len(video)),
			bytes.NewReader(video),
		)
		require.NoError(t, err)
		require.NoError(t, captured.StorageError)
		require.NoError(t, captured.Close())
	}

	require.Len(t, store.puts, 2)
	taskDigest := sha256.Sum256([]byte(taskID))
	wantKey := "seedance/outputs/101/202/" + hex.EncodeToString(taskDigest[:]) + ".mp4"
	require.Equal(t, wantKey, store.puts[0].Key)
	require.Equal(t, wantKey, store.puts[1].Key)
	require.NotContains(t, wantKey, taskID)
}

func TestSeedanceCaptureOutputArchiveLeasePreventsDuplicateBodyConsumption(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, redisClient)
	owner := seedanceMediaTestOwner()
	taskID := "vidjob_archive_lease"
	video := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	body := &seedanceMediaCountingReader{reader: bytes.NewReader(video)}

	releaseArchive, acquired := service.acquireOutputArchive(context.Background(), owner, taskID)
	require.True(t, acquired)
	require.NotNil(t, releaseArchive)

	captured, err := service.CaptureAndStoreOutput(
		context.Background(),
		owner,
		taskID,
		"video/mp4",
		int64(len(video)),
		body,
	)
	require.Nil(t, captured)
	require.ErrorIs(t, err, ErrSeedanceOutputArchiveInProgress)
	require.Zero(t, body.readCalls)
	require.Zero(t, body.bytesRead)
	require.Empty(t, store.puts)

	lockKey := seedanceOutputLockPrefix + strings.TrimPrefix(seedanceOutputRecordKey(owner, taskID), seedanceOutputRecordPrefix)
	exists, err := redisClient.Exists(context.Background(), lockKey).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists)

	releaseArchive()
	exists, err = redisClient.Exists(context.Background(), lockKey).Result()
	require.NoError(t, err)
	require.Zero(t, exists)

	captured, err = service.CaptureAndStoreOutput(
		context.Background(),
		owner,
		taskID,
		"video/mp4",
		int64(len(video)),
		body,
	)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NoError(t, captured.StorageError)
	require.NoError(t, captured.Close())
	require.Positive(t, body.readCalls)
	require.Equal(t, len(video), body.bytesRead)
	require.Len(t, store.puts, 1)
}

func TestNewSeedanceMediaHTTPClientHasNoTotalTimeout(t *testing.T) {
	client := newSeedanceMediaHTTPClient()
	require.NotNil(t, client)
	require.Zero(t, client.Timeout)
}

func TestSeedanceMediaIOConcurrencyIsOwnerScoped(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	service := NewSeedanceMediaService(newSeedanceMediaMemoryStore(), nil, redisClient)
	owner := seedanceMediaTestOwner()

	release, err := service.AcquireMediaIO(context.Background(), owner, 1)
	require.NoError(t, err)
	_, err = service.AcquireMediaIO(context.Background(), owner, 1)
	require.Error(t, err)
	require.Equal(t, http.StatusTooManyRequests, infraerrors.Code(err))
	require.Equal(t, "media_concurrency_exceeded", infraerrors.Reason(err))

	_, err = service.AcquireMediaIO(context.Background(), SeedanceMediaOwner{
		UserID: owner.UserID, APIKeyID: owner.APIKeyID + 1, GroupID: owner.GroupID,
	}, 1)
	require.Error(t, err)
	require.Equal(t, "media_concurrency_exceeded", infraerrors.Reason(err))

	otherRelease, err := service.AcquireMediaIO(context.Background(), SeedanceMediaOwner{
		UserID: owner.UserID + 1, APIKeyID: owner.APIKeyID + 1, GroupID: owner.GroupID,
	}, 1)
	require.NoError(t, err)
	otherRelease()
	release()

	releaseAgain, err := service.AcquireMediaIO(context.Background(), owner, 1)
	require.NoError(t, err)
	releaseAgain()
}

func TestSeedanceArchiveSlotsBoundTemporaryVideos(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	service := NewSeedanceMediaService(newSeedanceMediaMemoryStore(), nil, redisClient)
	owner := seedanceMediaTestOwner()

	first, acquired := service.BeginOutputArchive(context.Background(), owner, "task-first")
	require.True(t, acquired)
	second, acquired := service.BeginOutputArchive(context.Background(), owner, "task-second")
	require.True(t, acquired)
	_, acquired = service.BeginOutputArchive(context.Background(), owner, "task-third")
	require.False(t, acquired)

	first.Close()
	third, acquired := service.BeginOutputArchive(context.Background(), owner, "task-third")
	require.True(t, acquired)
	third.Close()
	second.Close()
}

func TestSanitizeSeedanceUpstreamErrorBodyRedactsSignedQueries(t *testing.T) {
	body := []byte(`{"error":{"message":"bad https://cos.example.com/input.png?X-Amz-Credential=credential-value&X-Amz-Signature=aws-secret&q-signature=cos-secret&ordinary=visible"}}`)
	sanitized := string(sanitizeSeedanceUpstreamErrorBody(body))
	require.NotContains(t, sanitized, "credential-value")
	require.NotContains(t, sanitized, "aws-secret")
	require.NotContains(t, sanitized, "cos-secret")
	require.Contains(t, sanitized, "ordinary=visible")
	require.Contains(t, sanitized, "X-Amz-Signature=***")
}

func TestCleanupStaleSeedanceTempFilesIsPrefixAndAgeScoped(t *testing.T) {
	directory := t.TempDir()
	oldImage := filepath.Join(directory, "image-old")
	freshVideo := filepath.Join(directory, "video-fresh.mp4")
	unrelated := filepath.Join(directory, "keep.txt")
	require.NoError(t, os.WriteFile(oldImage, []byte("old"), 0o600))
	require.NoError(t, os.WriteFile(freshVideo, []byte("fresh"), 0o600))
	require.NoError(t, os.WriteFile(unrelated, []byte("keep"), 0o600))
	oldTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(oldImage, oldTime, oldTime))

	cleanupStaleSeedanceTempFiles(directory, time.Now().Add(-24*time.Hour), 100)

	_, err := os.Stat(oldImage)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(freshVideo)
	require.NoError(t, err)
	_, err = os.Stat(unrelated)
	require.NoError(t, err)
}

func TestValidateSeedanceMediaRemoteURLRejectsSSRFTargets(t *testing.T) {
	blocked := []string{
		"http://localhost/image.png",
		"http://assets.localhost/image.png",
		"http://127.0.0.1/image.png",
		"http://10.0.0.1/image.png",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/image.png",
		"http://[::ffff:127.0.0.1]/image.png",
		"https://user:password@images.example.com/image.png",
		"https://images.example.com:8443/image.png",
		"https://images.example.com/image.png#fragment",
	}
	for _, target := range blocked {
		t.Run(target, func(t *testing.T) {
			_, err := validateSeedanceMediaRemoteURL(target)
			require.Error(t, err)
		})
	}

	validated, err := validateSeedanceMediaRemoteURL("https://images.example.com/reference.png?version=2")
	require.NoError(t, err)
	require.Equal(t, "https://images.example.com/reference.png?version=2", validated)
}

func TestSeedanceOpenRecordForwardsRangeStatuses(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		contentRange string
	}{
		{name: "full response", status: http.StatusOK},
		{name: "partial response", status: http.StatusPartialContent, contentRange: "bytes 0-1/3"},
		{name: "unsatisfiable range", status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedRange string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				receivedRange = request.Header.Get("Range")
				if tt.contentRange != "" {
					w.Header().Set("Content-Range", tt.contentRange)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("video"))
			}))
			t.Cleanup(server.Close)

			store := newSeedanceMediaMemoryStore()
			store.presignURL = server.URL + "/object.mp4"
			service := NewSeedanceMediaService(store, nil, nil)
			service.httpClient = server.Client()
			stream, err := service.openRecord(context.Background(), seedanceMediaRecord{
				StorageProvider: store.provider,
				Bucket:          store.bucket,
				ObjectKey:       "seedance/outputs/task.mp4",
			}, "bytes=0-1")
			require.NoError(t, err)
			require.NotNil(t, stream)
			t.Cleanup(func() { require.NoError(t, stream.Body.Close()) })
			require.Equal(t, "bytes=0-1", receivedRange)
			require.Equal(t, tt.status, stream.StatusCode)
			require.Equal(t, tt.contentRange, stream.Header.Get("Content-Range"))
		})
	}
}

type seedanceMediaTestPut struct {
	Key         string
	ContentType string
	SizeBytes   int64
	Metadata    map[string]string
}

type seedanceMediaZeroReader struct{}

func (seedanceMediaZeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

type seedanceMediaCountingReader struct {
	reader    *bytes.Reader
	readCalls int
	bytesRead int
}

func (r *seedanceMediaCountingReader) Read(buffer []byte) (int, error) {
	r.readCalls++
	n, err := r.reader.Read(buffer)
	r.bytesRead += n
	return n, err
}

type seedanceMediaMemoryStore struct {
	mu         sync.Mutex
	configured bool
	provider   string
	bucket     string
	presignURL string
	objects    map[string][]byte
	puts       []seedanceMediaTestPut
	deleted    []AgentArtifactObjectLocation
}

func newSeedanceMediaMemoryStore() *seedanceMediaMemoryStore {
	return &seedanceMediaMemoryStore{
		configured: true,
		provider:   "cos",
		bucket:     "seedance-test",
		objects:    make(map[string][]byte),
	}
}

func (s *seedanceMediaMemoryStore) IsConfigured() bool { return s != nil && s.configured }
func (s *seedanceMediaMemoryStore) Provider() string   { return s.provider }
func (s *seedanceMediaMemoryStore) Bucket() string     { return s.bucket }

func (s *seedanceMediaMemoryStore) Put(_ context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[input.Key] = append([]byte(nil), body...)
	s.puts = append(s.puts, seedanceMediaTestPut{
		Key:         input.Key,
		ContentType: input.ContentType,
		SizeBytes:   input.SizeBytes,
		Metadata:    input.Metadata,
	})
	return &AgentArtifactStorePutResult{
		Provider:  s.provider,
		Bucket:    s.bucket,
		ObjectKey: input.Key,
		SizeBytes: int64(len(body)),
	}, nil
}

func (s *seedanceMediaMemoryStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetObject(ctx, AgentArtifactObjectLocation{StorageProvider: s.provider, Bucket: s.bucket, ObjectKey: key}, ttl)
}

func (s *seedanceMediaMemoryStore) PresignGetObject(_ context.Context, location AgentArtifactObjectLocation, _ time.Duration) (string, error) {
	if s.presignURL != "" {
		return s.presignURL, nil
	}
	return "https://cos.example.com/" + url.PathEscape(location.ObjectKey), nil
}

func (s *seedanceMediaMemoryStore) Delete(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, AgentArtifactObjectLocation{StorageProvider: s.provider, Bucket: s.bucket, ObjectKey: key})
}

func (s *seedanceMediaMemoryStore) DeleteObject(_ context.Context, location AgentArtifactObjectLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, location.ObjectKey)
	s.deleted = append(s.deleted, location)
	return nil
}

func seedanceMediaTestOwner() SeedanceMediaOwner {
	return SeedanceMediaOwner{UserID: 101, APIKeyID: 202, GroupID: 303}
}

func seedanceMediaTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x35, G: 0x8a, B: 0xd8, A: 0xff})
	var output bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&output, img)
	case "jpeg":
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 90})
	default:
		t.Fatalf("unsupported Seedance test image format %q", format)
	}
	require.NoError(t, err)
	return output.Bytes()
}
