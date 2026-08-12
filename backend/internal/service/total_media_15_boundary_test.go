package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeBoundaryRefs(n int) []SeedanceReferenceImage {
	out := make([]SeedanceReferenceImage, n)
	for i := 0; i < n; i++ {
		out[i] = SeedanceReferenceImage{URL: fmt.Sprintf("https://example.com/img-%d.png", i+1)}
	}
	return out
}

func makeBoundaryVids(n int) []SeedanceReferenceVideo {
	out := make([]SeedanceReferenceVideo, n)
	for i := 0; i < n; i++ {
		out[i] = SeedanceReferenceVideo{URL: fmt.Sprintf("https://example.com/v-%d.mp4", i+1), DurationSeconds: 1}
	}
	return out
}

func makeBoundaryAuds(n int) []SeedanceReferenceAudio {
	out := make([]SeedanceReferenceAudio, n)
	for i := 0; i < n; i++ {
		out[i] = SeedanceReferenceAudio{URL: fmt.Sprintf("https://example.com/a-%d.mp3", i+1), DurationSeconds: 1}
	}
	return out
}

func TestTotalMedia15BoundaryFor933(t *testing.T) {
	ximei15 := &SeedanceRequestInfo{
		Model:           SeedanceXimeiSD20Model,
		Prompt:          "boundary test total media 15",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		GenerateAudio:   true,
		References:      makeBoundaryRefs(9),
		VideoReferences: makeBoundaryVids(3),
		AudioReferences: makeBoundaryAuds(3),
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(ximei15), "ximei sd-2.0-mx933 should allow 15 total")

	huiqu13 := &SeedanceRequestInfo{
		Model:           SeedanceMX933Model,
		Prompt:          "boundary test total media 13 huiqu",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		GenerateAudio:   true,
		References:      makeBoundaryRefs(7),
		VideoReferences: makeBoundaryVids(3),
		AudioReferences: makeBoundaryAuds(3),
	}
	err := validateFFLinkVideoRequestInfo(huiqu13)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 12 total")
	t.Logf("huiqu sd2-mx933 rejects 13: %v", err)

	huiqu15 := &SeedanceRequestInfo{
		Model:           SeedanceMX933Model,
		Prompt:          "boundary test total media 15 huiqu",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		GenerateAudio:   true,
		References:      makeBoundaryRefs(9),
		VideoReferences: makeBoundaryVids(3),
		AudioReferences: makeBoundaryAuds(3),
	}
	err = validateFFLinkVideoRequestInfo(huiqu15)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 12 total")
	t.Logf("huiqu sd2-mx933 rejects 15: %v", err)

	huiqu12 := &SeedanceRequestInfo{
		Model:           SeedanceMX933Model,
		Prompt:          "boundary test total media 12 huiqu",
		Resolution:      VideoBillingResolution720P,
		DurationSeconds: 5,
		GenerateAudio:   true,
		References:      makeBoundaryRefs(6),
		VideoReferences: makeBoundaryVids(3),
		AudioReferences: makeBoundaryAuds(3),
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(huiqu12))

	ximei13 := *ximei15
	ximei13.References = makeBoundaryRefs(7)
	require.NoError(t, validateFFLinkVideoRequestInfo(&ximei13))
}

func TestXimeiSD20BuildRequestAllows15TotalMedia(t *testing.T) {
	info := &SeedanceRequestInfo{
		Model:           SeedanceXimeiSD20Model,
		Prompt:          "test 15 media create body",
		Resolution:      VideoBillingResolution480P,
		DurationSeconds: 5,
		GenerateAudio:   true,
		AspectRatio:     "16:9",
		References:      makeBoundaryRefs(9),
		VideoReferences: makeBoundaryVids(3),
		AudioReferences: makeBoundaryAuds(3),
	}
	require.NoError(t, validateFFLinkVideoRequestInfo(info))
	product, err := ximeiVideoProductFor(SeedanceXimeiSD20Model, VideoBillingResolution480P)
	require.NoError(t, err)
	require.NoError(t, validateXimeiReferenceDurations(info, product))

	body, route, err := buildXimeiVideoCreateRequest(info)
	require.NoError(t, err)
	require.Equal(t, "kele_pool", route)

	var payload ximeiVideoCreateRequest
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, 9, len(payload.ImageURLs))
	require.Equal(t, 3, len(payload.VideoURLs))
	require.Equal(t, 3, len(payload.AudioURLs))
	require.Equal(t, 15, len(payload.ImageURLs)+len(payload.VideoURLs)+len(payload.AudioURLs))
	t.Logf("ximei create body accepts 15 assets route=%s images=%d videos=%d audios=%d", route, len(payload.ImageURLs), len(payload.VideoURLs), len(payload.AudioURLs))
}
