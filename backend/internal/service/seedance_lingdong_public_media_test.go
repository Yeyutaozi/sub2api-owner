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
	// Bare public-read COS is fine for general proxy checks (not Lingdong).
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://bucket.cos.ap-hongkong.myqcloud.com/seedance/public-rehost/abc/x.png"))
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://litter.catbox.moe/example.png"))
	require.False(t, seedanceMediaURLNeedsPublicProxy("https://files.catbox.moe/example.mp4"))
}

func TestSeedanceMediaURLNeedsLingdongRehost(t *testing.T) {
	require.True(t, seedanceMediaURLNeedsLingdongRehost("https://bucket.cos.ap-hongkong.myqcloud.com/seedance/inputs/staged/1/2/sdupl_x.png?X-Amz-Signature=abc"))
	// Bare COS must rehost for Lingdong — upstream silently drops these hosts.
	require.True(t, seedanceMediaURLNeedsLingdongRehost("https://bucket.cos.ap-hongkong.myqcloud.com/seedance/public-rehost/abc/x.png"))
	require.True(t, seedanceMediaURLNeedsLingdongRehost("https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/agent-artifacts/seedance/inputs/staged/1/2/a.png"))
	require.True(t, seedanceMediaURLNeedsLingdongRehost("https://tkcreazy.top/v1/videos/uploads/sdupl_abc123"))
	require.True(t, seedanceMediaURLNeedsLingdongRehost("https://tkcreazy.top/v1/videos/public-media/sdpub_abc123"))
	require.False(t, seedanceMediaURLNeedsLingdongRehost("https://litter.catbox.moe/example.png"))
	require.False(t, seedanceMediaURLNeedsLingdongRehost("https://files.catbox.moe/example.mp4"))
	require.False(t, seedanceMediaURLNeedsLingdongRehost("https://httpbin.org/image/png"))
	require.True(t, seedanceMediaURLIsCloudObjectStorage("https://bucket.s3.amazonaws.com/key"))
	require.False(t, seedanceMediaURLIsCloudObjectStorage("https://litter.catbox.moe/a.png"))
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
	readStore := &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}

	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 9, APIKeyID: 8, GroupID: 7}
	// Force deterministic third-party rehost (no network).
	var rehostCalls int
	svc.lingdongRehostFn = func(ctx context.Context, filename, contentType string, payload []byte) (string, error) {
		rehostCalls++
		require.NotEmpty(t, payload)
		if strings.HasSuffix(filename, ".mp4") || strings.Contains(contentType, "video") {
			return "https://litter.catbox.moe/rehosted-vid.mp4", nil
		}
		return fmt.Sprintf("https://litter.catbox.moe/rehosted-%d.png", rehostCalls), nil
	}

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
	_ = extra

	// COS must be rehosted away from myqcloud, not merely stripped.
	require.True(t, strings.HasPrefix(info.References[0].URL, "https://litter.catbox.moe/"), info.References[0].URL)
	require.NotContains(t, info.References[0].URL, "myqcloud.com")
	require.NotContains(t, info.References[0].URL, "X-Amz-Signature")
	require.Equal(t, publicCatbox, info.References[1].URL)
	require.Equal(t, "https://litter.catbox.moe/rehosted-vid.mp4", info.VideoReferences[0].URL)
	require.Equal(t, "https://files.catbox.moe/keep-vid.mp4", info.VideoReferences[1].URL)
	require.Equal(t, "https://files.catbox.moe/third.mp4", info.VideoReferences[2].URL)
	require.GreaterOrEqual(t, rehostCalls, 2)

	body, err := buildLingdongVideoCreateRequest(info, DefaultLingdongUpstreamModel)
	require.NoError(t, err)
	require.NotContains(t, string(body), "myqcloud.com")
	require.NotContains(t, string(body), "cos.example.com")
	require.Contains(t, string(body), "litter.catbox.moe")
	require.Contains(t, string(body), publicCatbox)
	require.NotContains(t, string(body), "X-Amz-Signature")
	require.NotContains(t, string(body), "/v1/videos/public-media/")
	// videos truncated to 2
	require.Contains(t, string(body), "keep-vid.mp4")
	require.NotContains(t, string(body), "third.mp4")
}

func TestPrepareLingdongPublicMediaRehostsSignedCOS(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "zntcenter-1326757433"
	memory.provider = "cos"
	imgKey := "agent-artifacts/seedance/inputs/staged/1/2/sdupl_x.png"
	vidKey := "agent-artifacts/seedance/inputs/staged/1/2/a.mp4"
	// object key must belong to owner for own-URL resolve
	imgKey = "seedance/inputs/staged/1/2/sdupl_x.png"
	vidKey = "seedance/inputs/staged/1/2/a.mp4"
	memory.objects[imgKey] = []byte("png")
	memory.objects[vidKey] = []byte("mp4")
	readStore := &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}
	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 1, APIKeyID: 2, GroupID: 3}
	svc.lingdongRehostFn = func(ctx context.Context, filename, contentType string, payload []byte) (string, error) {
		if strings.Contains(contentType, "video") || strings.HasSuffix(filename, ".mp4") {
			return "https://litter.catbox.moe/vid.mp4", nil
		}
		return "https://litter.catbox.moe/img.png", nil
	}

	signed := "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/" + imgKey + "?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef"
	info := &SeedanceRequestInfo{
		Model: SeedanceWeijinFaceRef720pModel, Prompt: "strip", DurationSeconds: 5,
		Resolution: VideoBillingResolution720P, AspectRatio: "16:9",
		References:      []SeedanceReferenceImage{{URL: signed}},
		VideoReferences: []SeedanceReferenceVideo{{URL: "https://zntcenter-1326757433.cos.ap-hongkong.myqcloud.com/" + vidKey + "?X-Amz-Signature=abc"}},
	}
	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.NoError(t, err)
	_ = extra
	require.Equal(t, "https://litter.catbox.moe/img.png", info.References[0].URL)
	require.Equal(t, "https://litter.catbox.moe/vid.mp4", info.VideoReferences[0].URL)
	require.NotContains(t, info.References[0].URL, "myqcloud")
}

func TestPrepareLingdongPublicMediaFailsLoudWhenRehostUnavailable(t *testing.T) {
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	memory := newSeedanceMediaMemoryStore()
	memory.bucket = "seedance-test"
	memory.provider = "cos"
	imgKey := "seedance/inputs/staged/9/8/sdupl_img_fail.png"
	memory.objects[imgKey] = []byte("png-fail")
	readStore := &seedanceFailPublicRehostStore{seedancePublicMediaReadStore: &seedancePublicMediaReadStore{seedanceMediaMemoryStore: memory}}

	svc := NewSeedanceMediaService(readStore, nil, redisClient)
	owner := SeedanceMediaOwner{UserID: 9, APIKeyID: 8, GroupID: 7}
	svc.lingdongRehostFn = func(ctx context.Context, filename, contentType string, payload []byte) (string, error) {
		return "", fmt.Errorf("third-party unavailable")
	}
	cosImage := "https://seedance-test.cos.ap-hongkong.myqcloud.com/" + imgKey + "?X-Amz-Signature=deadbeef"
	info := &SeedanceRequestInfo{
		Model:           SeedanceWeijinFaceRef720pModel,
		Prompt:          "fail",
		DurationSeconds: 5,
		Resolution:      VideoBillingResolution720P,
		AspectRatio:     "16:9",
		References:      []SeedanceReferenceImage{{URL: cosImage}},
	}
	extra, err := svc.PrepareLingdongPublicMedia(context.Background(), owner, info, "https://tkcreazy.top")
	require.Error(t, err)
	require.Nil(t, extra)
	require.False(t, strings.Contains(info.References[0].URL, "/v1/videos/public-media/"), info.References[0].URL)
	require.Contains(t, strings.ToLower(err.Error()), "rehost")
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

type seedanceFailPublicRehostStore struct {
	*seedancePublicMediaReadStore
}

func (s *seedanceFailPublicRehostStore) PresignGetObject(context.Context, AgentArtifactObjectLocation, time.Duration) (string, error) {
	return "", fmt.Errorf("presign unavailable")
}

func (s *seedanceFailPublicRehostStore) Put(context.Context, AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	return nil, fmt.Errorf("put unavailable")
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
