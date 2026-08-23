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
	"strconv"
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
				"duration":   5,
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
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 5, StartFrameURL: dataURI},
		},
		{
			name: "first and last frames",
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 5, StartFrameURL: "https://images.example.com/start.png", EndFrameURL: dataURI},
		},
		{
			name: "reference image",
			info: SeedanceRequestInfo{Prompt: "test", Resolution: "720p", DurationSeconds: 5, References: []SeedanceReferenceImage{{URL: dataURI, Strength: "MID"}}},
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

func TestSeedanceMediaUploadTrustsSniffedImageTypeOverDeclared(t *testing.T) {
	pngBytes := seedanceMediaTestImage(t, "png", 2, 2)
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)

	// Browser often labels PNG as image/jpeg after rename / clipboard / chat app export.
	upload, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
		Owner:       seedanceMediaTestOwner(),
		Body:        bytes.NewReader(pngBytes),
		SizeBytes:   int64(len(pngBytes)),
		ContentType: "image/jpeg",
	})
	require.NoError(t, err)
	require.Equal(t, "image/png", upload.ContentType)
	require.Len(t, store.puts, 1)
	require.Equal(t, "image/png", store.puts[0].ContentType)
	require.True(t, strings.HasSuffix(store.puts[0].Key, ".png"))

	// Unsupported declared MIME must not reject a valid PNG/JPEG/WebP body.
	store2 := newSeedanceMediaMemoryStore()
	service2 := NewSeedanceMediaService(store2, nil, nil)
	upload2, err := service2.UploadImage(context.Background(), SeedanceImageUploadInput{
		Owner:       seedanceMediaTestOwner(),
		Body:        bytes.NewReader(pngBytes),
		SizeBytes:   int64(len(pngBytes)),
		ContentType: "image/gif",
	})
	require.NoError(t, err)
	require.Equal(t, "image/png", upload2.ContentType)
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

func TestSeedanceCanvasMediaUploadSkipsImageAndVideoBusinessSizeLimits(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		store := newSeedanceMediaMemoryStore()
		service := NewSeedanceMediaService(store, nil, nil)
		pngBytes := seedanceMediaTestImage(t, "png", 2, 2)
		totalSize := SeedanceMaxImageBytes + 1
		body := io.MultiReader(
			bytes.NewReader(pngBytes),
			io.LimitReader(seedanceMediaZeroReader{}, totalSize-int64(len(pngBytes))),
		)

		upload, err := service.UploadImage(context.Background(), SeedanceImageUploadInput{
			Owner:         seedanceMediaTestOwner(),
			Body:          body,
			SizeBytes:     totalSize,
			ContentType:   "image/png",
			SkipSizeLimit: true,
		})
		require.NoError(t, err)
		require.Equal(t, totalSize, upload.SizeBytes)
		require.Len(t, store.puts, 1)
	})

	t.Run("video", func(t *testing.T) {
		store := newSeedanceMediaMemoryStore()
		service := NewSeedanceMediaService(store, nil, nil)
		service.maxVideoBytes = 4
		videoBytes := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}

		upload, err := service.UploadMedia(context.Background(), SeedanceImageUploadInput{
			Owner:         seedanceMediaTestOwner(),
			Body:          bytes.NewReader(videoBytes),
			SizeBytes:     int64(len(videoBytes)),
			ContentType:   "video/mp4",
			Filename:      "reference.mp4",
			MediaKind:     "video",
			SkipSizeLimit: true,
		})
		require.NoError(t, err)
		require.Equal(t, int64(len(videoBytes)), upload.SizeBytes)
		require.Len(t, store.puts, 1)
	})
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

func TestSeedanceMaterializeImagesResignsOwnPersistentUploadURL(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	owner := seedanceMediaTestOwner()
	source := "https://seedance-test.cos.ap-hongkong.myqcloud.com/agent-artifacts/seedance/inputs/staged/101/202/sdupl_abc123.png?X-Amz-Signature=test"
	info := &SeedanceRequestInfo{StartFrameURL: source}

	materialized, err := service.MaterializeImages(context.Background(), owner, info)
	require.NoError(t, err)
	require.NotNil(t, materialized)
	require.NotEqual(t, source, info.StartFrameURL)
	require.Contains(t, info.StartFrameURL, "agent-artifacts%2Fseedance%2Finputs%2Fstaged%2F101%2F202%2Fsdupl_abc123.png")
	require.Equal(t, []SeedanceStoredMediaReference{{
		Slot: seedanceStoredMediaStartFrame, StorageProvider: "cos", Bucket: "seedance-test",
		ObjectKey: "agent-artifacts/seedance/inputs/staged/101/202/sdupl_abc123.png",
	}}, info.StoredMedia)
	require.Empty(t, materialized.objects)
	require.Len(t, store.puts, 0)
}

func TestSeedanceMaterializeImagesResignsOwnLocalMediaURLWithoutHTTPFetch(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	store.provider = "local"
	store.bucket = "local-media"
	store.presignURL = "https://gateway.example.com/api/v1/local-media?key=fresh&expires=1893456000&signature=fresh"
	service := NewSeedanceMediaService(store, nil, nil)
	fetchCalls := 0
	service.httpClient = &http.Client{Transport: seedanceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		fetchCalls++
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("not found")),
			Request:    request,
		}, nil
	})}

	owner := seedanceMediaTestOwner()
	objectKey := "media/seedance/inputs/staged/101/202/sdupl_local.png"
	encodedKey := base64.RawURLEncoding.EncodeToString([]byte(objectKey))
	source := "https://gateway.example.com/api/v1/local-media?key=" + encodedKey + "&expires=1893456000&signature=old"
	info := &SeedanceRequestInfo{Model: SeedanceTianyueSD20FastModel, References: []SeedanceReferenceImage{{URL: source}}}
	otherOwner := owner
	otherOwner.UserID++
	_, belongsToOtherOwner := service.seedanceObjectLocationFromOwnURL(otherOwner, source)
	require.False(t, belongsToOtherOwner)

	materialized, err := service.MaterializeImages(context.Background(), owner, info)
	require.NoError(t, err)
	require.NotNil(t, materialized)
	require.Zero(t, fetchCalls, "own local-media URLs must be resolved from storage instead of fetched over HTTP")
	require.Equal(t, store.presignURL, info.References[0].URL)
	require.Equal(t, []SeedanceStoredMediaReference{{
		Slot: seedanceStoredMediaImage, StorageProvider: "local", Bucket: "local-media", ObjectKey: objectKey,
	}}, info.StoredMedia)
	require.Empty(t, materialized.objects)
	require.Empty(t, store.puts)
}

func TestSeedanceMaterializeImagesArchivesFallbackVideoAndAudioAndRefreshesSignatures(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	mp4 := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	wav := []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x44\xac\x00\x00\x88\x58\x01\x00\x02\x00\x10\x00data\x00\x00\x00\x00")
	service.httpClient = &http.Client{Transport: seedanceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := mp4
		contentType := "video/mp4"
		if strings.HasSuffix(request.URL.Path, ".wav") {
			body = wav
			contentType = "audio/wav"
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{contentType}, "Content-Length": []string{strconv.Itoa(len(body))}},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       request,
		}, nil
	})}
	info := &SeedanceRequestInfo{
		Model: "seedance-2.0", Prompt: "preserve every reference", Resolution: "720p",
		DurationSeconds: 10, AspectRatio: "16:9",
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://93.184.216.34/reference.mp4"}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://93.184.216.34/reference.wav"}},
	}

	materialized, err := service.MaterializeImages(context.Background(), seedanceMediaTestOwner(), info)
	require.NoError(t, err)
	require.Len(t, materialized.objects, 2)
	require.Len(t, store.puts, 2)
	require.Len(t, info.StoredMedia, 2)
	require.Equal(t, seedanceStoredMediaVideo, info.StoredMedia[0].Slot)
	require.Equal(t, seedanceStoredMediaAudio, info.StoredMedia[1].Slot)
	require.True(t, info.StoredMedia[0].DeleteAfterSettlement)
	require.True(t, info.StoredMedia[1].DeleteAfterSettlement)
	require.Contains(t, info.VideoReferences[0].URL, "seedance%2Finputs%2Ftask")
	require.Contains(t, info.AudioReferences[0].URL, "seedance%2Finputs%2Ftask")

	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)
	restored, err := RestoreSeedanceFallbackRequest(snapshot, SeedanceMX933Model)
	require.NoError(t, err)
	restored.VideoReferences[0].URL = "https://expired.example.com/video"
	restored.AudioReferences[0].URL = "https://expired.example.com/audio"
	presignsBeforeRefresh := len(store.presigned)
	require.NoError(t, service.RefreshSeedanceFallbackMediaURLs(context.Background(), seedanceMediaTestOwner(), restored))
	require.Len(t, store.presigned, presignsBeforeRefresh+2)
	require.Contains(t, restored.VideoReferences[0].URL, "seedance%2Finputs%2Ftask")
	require.Contains(t, restored.AudioReferences[0].URL, "seedance%2Finputs%2Ftask")

	materialized.Cleanup(context.Background())
	require.Len(t, store.deleted, 2)
}

func TestSeedanceMaterializeImagesArchivesXimeiVideoAndAudio(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	mp4 := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	wav := []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x44\xac\x00\x00\x88\x58\x01\x00\x02\x00\x10\x00data\x00\x00\x00\x00")
	service.httpClient = &http.Client{Transport: seedanceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := mp4
		contentType := "video/mp4"
		if strings.HasSuffix(request.URL.Path, ".wav") {
			body = wav
			contentType = "audio/wav"
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{contentType}, "Content-Length": []string{strconv.Itoa(len(body))}},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       request,
		}, nil
	})}
	info := &SeedanceRequestInfo{
		Model: SeedanceXimeiSD20Model, Prompt: "preserve every reference", Resolution: "480p",
		DurationSeconds: 5, AspectRatio: "16:9", GenerateAudio: true,
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://93.184.216.34/reference.mp4", DurationSeconds: 5}},
		AudioReferences: []SeedanceReferenceAudio{{URL: "https://93.184.216.34/reference.wav", DurationSeconds: 5}},
	}

	materialized, err := service.MaterializeImages(context.Background(), seedanceMediaTestOwner(), info)
	require.NoError(t, err)
	require.Len(t, materialized.objects, 2)
	require.Len(t, store.puts, 2)
	require.Len(t, info.StoredMedia, 2)
	require.Equal(t, seedanceStoredMediaVideo, info.StoredMedia[0].Slot)
	require.Equal(t, seedanceStoredMediaAudio, info.StoredMedia[1].Slot)
	require.Contains(t, info.VideoReferences[0].URL, "seedance%2Finputs%2Ftask")
	require.Contains(t, info.AudioReferences[0].URL, "seedance%2Finputs%2Ftask")

	materialized.Cleanup(context.Background())
	require.Len(t, store.deleted, 2)
}

func TestSeedanceRefreshFallbackMediaURLsSupportsLegacySnapshotURLs(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	legacyURL := "https://seedance-test.cos.ap-hongkong.myqcloud.com/agent-artifacts/seedance/inputs/task/101/202/sdupl_legacy.wav?X-Amz-Expires=1&X-Amz-Signature=expired"
	info := &SeedanceRequestInfo{AudioReferences: []SeedanceReferenceAudio{{URL: legacyURL}}}

	require.NoError(t, service.RefreshSeedanceFallbackMediaURLs(context.Background(), seedanceMediaTestOwner(), info))
	require.NotEqual(t, legacyURL, info.AudioReferences[0].URL)
	require.Len(t, store.presigned, 1)
	require.Equal(t, "agent-artifacts/seedance/inputs/task/101/202/sdupl_legacy.wav", store.presigned[0].ObjectKey)
}

func TestSeedanceDeleteFallbackMediaDeletesTaskCopiesButKeepsStagedUploads(t *testing.T) {
	store := newSeedanceMediaMemoryStore()
	service := NewSeedanceMediaService(store, nil, nil)
	owner := seedanceMediaTestOwner()
	info := &SeedanceRequestInfo{
		Model: "seedance-2.0", Prompt: "preserve references", Resolution: "720p", DurationSeconds: 5, AspectRatio: "16:9",
		StoredMedia: []SeedanceStoredMediaReference{
			{
				Slot: seedanceStoredMediaVideo, StorageProvider: "cos", Bucket: "seedance-test",
				ObjectKey: "agent-artifacts/seedance/inputs/task/101/202/fallback-video.mp4", DeleteAfterSettlement: true,
			},
			{
				Slot: seedanceStoredMediaAudio, StorageProvider: "cos", Bucket: "seedance-test",
				ObjectKey: "agent-artifacts/seedance/inputs/staged/101/202/user-audio.wav",
			},
		},
	}
	snapshot, err := SnapshotSeedanceFallbackRequest(info)
	require.NoError(t, err)

	require.NoError(t, service.DeleteSeedanceFallbackMedia(context.Background(), owner, snapshot))
	require.Equal(t, []AgentArtifactObjectLocation{{
		StorageProvider: "cos", Bucket: "seedance-test",
		ObjectKey: "agent-artifacts/seedance/inputs/task/101/202/fallback-video.mp4",
	}}, store.deleted)
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

func TestSeedanceMediaIOConcurrencyDoesNotReuseSubmissionLimit(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	service := NewSeedanceMediaService(newSeedanceMediaMemoryStore(), nil, redisClient)
	owner := seedanceMediaTestOwner()

	release, err := service.AcquireMediaIO(context.Background(), owner, 1)
	require.NoError(t, err)
	secondRelease, err := service.AcquireMediaIO(context.Background(), owner, 1)
	require.NoError(t, err)

	otherKeyRelease, err := service.AcquireMediaIO(context.Background(), SeedanceMediaOwner{
		UserID: owner.UserID, APIKeyID: owner.APIKeyID + 1, GroupID: owner.GroupID,
	}, 1)
	require.NoError(t, err)

	otherRelease, err := service.AcquireMediaIO(context.Background(), SeedanceMediaOwner{
		UserID: owner.UserID + 1, APIKeyID: owner.APIKeyID + 1, GroupID: owner.GroupID,
	}, 1)
	require.NoError(t, err)
	otherRelease()
	otherKeyRelease()
	secondRelease()
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

func TestSeedanceOpenRecordPrefersDirectArtifactRead(t *testing.T) {
	store := &seedanceMediaDirectReadStore{
		seedanceMediaMemoryStore: newSeedanceMediaMemoryStore(),
		result: &AgentArtifactObjectReadResult{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":  []string{"image/png"},
				"Content-Range": []string{"bytes 0-2/3"},
			},
			Body: io.NopCloser(strings.NewReader("png")),
		},
	}
	store.presignURL = "http://198.18.1.40/blocked-by-ssrf-policy"
	service := NewSeedanceMediaService(store, nil, nil)

	stream, err := service.openRecord(context.Background(), seedanceMediaRecord{
		StorageProvider: store.provider,
		Bucket:          store.bucket,
		ObjectKey:       "seedance/inputs/staged/reference.png",
	}, "bytes=0-2")
	require.NoError(t, err)
	require.Equal(t, 1, store.reads)
	require.Equal(t, http.StatusPartialContent, stream.StatusCode)
	require.Equal(t, "bytes 0-2/3", stream.Header.Get("Content-Range"))
	require.NoError(t, stream.Body.Close())
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
	presigned  []AgentArtifactObjectLocation
	deleteErr  error
}

type seedanceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f seedanceRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type seedanceMediaDirectReadStore struct {
	*seedanceMediaMemoryStore
	result *AgentArtifactObjectReadResult
	err    error
	reads  int
}

func (s *seedanceMediaDirectReadStore) ReadObject(_ context.Context, _ AgentArtifactObjectLocation, _ string) (*AgentArtifactObjectReadResult, error) {
	s.reads++
	return s.result, s.err
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
	publicURL := "https://public-rehost.example/" + strings.TrimLeft(input.Key, "/")
	if !input.PublicRead {
		publicURL = ""
	}
	return &AgentArtifactStorePutResult{
		Provider:  s.provider,
		Bucket:    s.bucket,
		ObjectKey: input.Key,
		ObjectURL: publicURL,
		SizeBytes: int64(len(body)),
	}, nil
}

func (s *seedanceMediaMemoryStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetObject(ctx, AgentArtifactObjectLocation{StorageProvider: s.provider, Bucket: s.bucket, ObjectKey: key}, ttl)
}

func (s *seedanceMediaMemoryStore) PresignGetObject(_ context.Context, location AgentArtifactObjectLocation, _ time.Duration) (string, error) {
	s.mu.Lock()
	s.presigned = append(s.presigned, location)
	presignURL := s.presignURL
	s.mu.Unlock()
	if presignURL != "" {
		return presignURL, nil
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
	return s.deleteErr
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
