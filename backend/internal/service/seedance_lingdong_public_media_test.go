package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSeedanceMediaURLNeedsPublicProxy(t *testing.T) {
	require.True(t, seedanceMediaURLNeedsPublicProxy("https://bucket.cos.ap-hongkong.myqcloud.com/seedance/inputs/staged/1/2/sdupl_x.png?X-Amz-Signature=abc"))
	require.True(t, seedanceMediaURLNeedsPublicProxy("https://example.com/obj?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=abc"))
	require.True(t, seedanceMediaURLNeedsPublicProxy("https://tkcreazy.top/v1/videos/uploads/sdupl_abc123"))
	require.True(t, seedanceMediaURLNeedsPublicProxy("/v1/videos/uploads/sdupl_abc123"))
	require.True(t, seedanceMediaURLNeedsPublicProxy("https://tkcreazy.top/v1/videos/public-media/sdpub_abc123"))
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://bucket.cos.ap-hongkong.myqcloud.com/seedance/public-rehost/abc/x.png"))
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://litter.catbox.moe/example.png"))
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://files.catbox.moe/example.mp4"))
}

func TestPrepareLingdongPublicMediaRewritesCOSAndLeavesPublic(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "seedance-test"
	memory.provider = "cos"
	imgKey := "seedance/inputs/staged/9/8/sdupl_img1.png"
	vidKey := "seedance/inputs/staged/9/8/sdupl_vid1.mp4"
	memory.objects[imgKey] = []byte("png-bytes")
	memory.objects[vidKey] = []byte("mp4-bytes")

	store := &seedanceMediaDirectReadStore{
		seedanceMediaMemoryStore: memory,
		result: &AgentArtifactObjectReadResult{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("png-bytes")),
		},
	}
	// Dynamic body per key for OpenPublicMedia.
	store.result = nil
	// custom ReadObject via wrapper below - override by using anonymous approach:
	// Re-create store with ReadObject that looks up objects.
	readStore := &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}

	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 9, APIKeyID: 8, GroupID: 7}

	now := time.Now().UTC()
	require.NoError(t, svc.saveRecord(context.Background(), seedanceUploadRecordPrefix+"sdupl_vid1", seedanceMediaRecord{
		ID: "sdupl_vid1", UserID: 9, APIKeyID: 8, GroupID: 7,
		StorageProvider: "cos", Bucket: "seedance-test",
		ObjectKey: vidKey, ContentType: "video/mp4", SizeBytes: 9, ExpiresAt: now.Add(time.Hour),
	}, time.Hour))

	cosImage := "https://seedance-test.cos.ap-hongkong.myqcloud.com/" + imgKey + "?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef"
	publicCatbox := "https://litter.catbox.moe/keep-me.png"
	managedVideo := "https://tkcreazy.top/v1/videos/uploads/sdupl_vid1"

	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "test lingdong materials",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References: []SeedanceReferenceImage{
			{URL: cosImage},
			{URL: publicCatbox},
		},
		VideoReferences: []SeedanceReferenceVideo{
			{URL: managedVideo},
			{URL: "https://files.catbox.moe/keep-vid.mp4"},
			{URL: "https://files.catbox.moe/third.mp4"},
		},
	}

	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.NoError(t, err)
	require.Nil(t, extra)

	// Signed COS/S3 URLs are stripped in-place (public-read bucket fast path).
	require.Equal(t, "https://seedance-test.cos.ap-hongkong.myqcloud.com/"+imgKey, info.References[0].URL)
	require.NotContains(t, info.References[0].URL, "X-Amz-Signature")
	require.Equal(t, publicCatbox, info.References[1].URL)
	// Managed uploads still resolve through storage and become unsigned public object URLs.
	require.True(t, strings.HasPrefix(info.VideoReferences[0].URL, "https://cos.example.com/"), info.VideoReferences[0].URL)
	require.NotContains(t, info.VideoReferences[0].URL, "X-Amz-Signature")
	require.Equal(t, "https://files.catbox.moe/keep-vid.mp4", info.VideoReferences[1].URL)
	require.Equal(t, "https://files.catbox.moe/third.mp4", info.VideoReferences[2].URL)

	// Lingdong create body keeps rewritten media and truncates videos to 2.
	body, err := buildLingdongVideoCreateRequest(info, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	require.Contains(t, string(body), "seedance-test.cos.ap-hongkong.myqcloud.com")
	require.Contains(t, string(body), "cos.example.com")
	require.Contains(t, string(body), publicCatbox)
	require.NotContains(t, string(body), "X-Amz-Signature")
	require.NotContains(t, string(body), "/v1/videos/public-media/")
	require.NotContains(t, string(body), "third.mp4")
}

func TestPrepareLingdongPublicMediaStripsSignedCOSQuery(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	svc := NewSeedanceMediaService(newSeedanceMediaMemoryStore(), nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 1, APIKeyID: 2, GroupID: 3}
	signed := "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/agent-artifacts/seedance/inputs/staged/1/341/sdupl_x.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef"
	info := &SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef720pModel, Prompt: "strip", DurationSeconds: 5,
		Resolution: VideoBillingResolution720P, AspectRatio: "16:9",
		References: []SeedanceReferenceImage{{URL: signed}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/a.mp4?X-Amz-Signature=abc"}},
	}
	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.NoError(t, err)
	require.Nil(t, extra)
	require.Equal(t, "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/agent-artifacts/seedance/inputs/staged/1/341/sdupl_x.png", info.References[0].URL)
	require.Equal(t, "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/a.mp4", info.VideoReferences[0].URL)
}

func TestPrepareLingdongPublicMediaUsesPublicReadObject(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "seedance-test"
	memory.provider = "cos"
	imgKey := "seedance/inputs/staged/9/8/sdupl_img_public.png"
	memory.objects[imgKey] = []byte("png-public")
	readStore := &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}

	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 9, APIKeyID: 8, GroupID: 7}
	cosImage := "https://seedance-test.cos.ap-hongkong.myqcloud.com/" + imgKey + "?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef"
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "public cos",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: cosImage}},
	}
	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.NoError(t, err)
	// No temporary materialization needed when only stripping signatures.
	require.Nil(t, extra)
	require.Equal(t, "https://seedance-test.cos.ap-hongkong.myqcloud.com/"+imgKey, info.References[0].URL)
	require.NotContains(t, info.References[0].URL, "X-Amz-Signature")
	require.NotContains(t, info.References[0].URL, "/v1/videos/public-media/")
}

func TestPrepareLingdongPublicMediaFailsLoudWhenRehostUnavailable(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "seedance-test"
	memory.provider = "cos"
	imgKey := "seedance/inputs/staged/9/8/sdupl_img_fallback.png"
	memory.objects[imgKey] = []byte("png-fallback")
	failStore := &seedanceFailPublicRehostStore{seedancePublicMediaReadStore: &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}}

	svc := NewSeedanceMediaService(failStore, nil, redisClient)
	svc.lingdongRehostFn = func(context.Context, string, string, []byte) (string, error) {
		return "", fmt.Errorf("rehost unavailable")
	}
	owner := SeedanceMediaOwner{UserID: 9, APIKeyID: 8, GroupID: 7}
	// Managed upload cannot strip signatures in-place; it must resolve via storage
	// then rehost. With presign/put/rehost all failing, fail loud (no public-media).
	now := time.Now().UTC()
	require.NoError(t, svc.saveRecord(context.Background(), seedanceUploadRecordPrefix+"sdupl_fail1", seedanceMediaRecord{
		ID: "sdupl_fail1", UserID: 9, APIKeyID: 8, GroupID: 7,
		StorageProvider: "cos", Bucket: "seedance-test",
		ObjectKey: imgKey, ContentType: "image/png", SizeBytes: 12, ExpiresAt: now.Add(time.Hour),
	}, time.Hour))
	managed := "https://tkcreazy.top/v1/videos/uploads/sdupl_fail1"
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "fallback",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: managed}},
	}
	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.Error(t, err)
	require.Nil(t, extra)
	// Must not silently fall back to gateway public-media for Lingdong upstream.
	require.False(t, strings.Contains(info.References[0].URL, "/v1/videos/public-media/"), info.References[0].URL)
	require.Contains(t, strings.ToLower(err.Error()), "rehost")
}

type seedanceFailPublicRehostStore struct {
	*seedancePublicMediaReadStore
}

func (s *seedanceFailPublicRehostStore) PresignGetObject(context.Context, AgentArtifactObjectLocation, time.Duration) (string, error) {
	return "", fmt.Errorf("presign unavailable")
}

func (s *seedanceFailPublicRehostStore) Put(context.Context, AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	return nil, fmt.Errorf("put unavailable")
}

func TestOpenPublicMediaStillServesInlineForProxyEndpoint(t *testing.T) {
	// public-media remains available for debugging / other uses, but must not be
	// used as Lingdong upstream input after rehost failure.
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "seedance-test"
	memory.provider = "cos"
	imgKey := "seedance/inputs/staged/9/8/sdupl_img_proxy.png"
	memory.objects[imgKey] = []byte("png-proxy")
	readStore := &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}

	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	url, err := svc.IssuePublicMediaURL(context.Background(), "https://tkcreazy.top", AgentArtifactObjectLocation{
		StorageProvider: "cos",
		Bucket:          "seedance-test",
		ObjectKey:       imgKey,
	}, "image/png")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(url, "https://tkcreazy.top/v1/videos/public-media/sdpub_"), url)

	token := strings.TrimPrefix(url, "https://tkcreazy.top/v1/videos/public-media/")
	stream, err := svc.OpenPublicMedia(context.Background(), token, "")
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Body.Close()
	require.Equal(t, "inline", stream.Header.Get("Content-Disposition"))
	require.Equal(t, "bytes", stream.Header.Get("Accept-Ranges"))
	payload, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("png-proxy"), payload)
}

type seedancePublicMediaReadStore struct {
	*seedanceMediaMemoryStore
}

func (s *seedancePublicMediaReadStore) ReadObject(_ context.Context, location AgentArtifactObjectLocation, _ string) (*AgentArtifactObjectReadResult, error) {
	s.mu.Lock()
	data, ok := s.objects[location.ObjectKey]
	s.mu.Unlock()
	if !ok {
		return nil, io.EOF
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/octet-stream")
	return &AgentArtifactObjectReadResult{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}
