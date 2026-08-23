package service

import (
	"math"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// Canonical video price family keys used in groups.video_model_prices JSONB.
const (
	VideoPriceFamilyGrokImagineVideo   = "grok-imagine-video"
	VideoPriceFamilyGrokImagineVideo15 = "grok-imagine-video-1.5"
)

// CanonicalGrokImagineVideoPriceFamily normalizes model aliases / preview / legacy
// IDs onto the price-family keys stored in video_model_prices.
func CanonicalGrokImagineVideoPriceFamily(model string) string {
	if model == "" {
		return ""
	}
	// Prefer shared xAI helper for known aliases. Keep future native Imagine
	// models distinct so operators can assign them independent prices.
	if c := xai.CanonicalImagineVideoModel(model); c != "" {
		switch c {
		case xai.DefaultImagineVideo15Model:
			return VideoPriceFamilyGrokImagineVideo15
		case xai.DefaultImagineVideoModel:
			return VideoPriceFamilyGrokImagineVideo
		}
		if strings.HasPrefix(c, "grok-imagine-video-") {
			return c
		}
	}
	m := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"xai/", "x-ai/", "grok/"} {
		if strings.HasPrefix(m, prefix) {
			m = strings.TrimPrefix(m, prefix)
			break
		}
	}
	switch {
	case m == "grok-imagine-video-1.5" || m == "grok-imagine-video-1.5-preview" ||
		m == "grok-video-1.5" || strings.Contains(m, "video-1.5"):
		return VideoPriceFamilyGrokImagineVideo15
	case m == "grok-imagine-video" || m == "grok-imagine-video-preview" ||
		m == "grok-video" || m == "grok-video-latest":
		return VideoPriceFamilyGrokImagineVideo
	default:
		return ""
	}
}

// NormalizeVideoModelPrices cleans and canonicalizes a per-model resolution map.
// Keys become price families; tiers use 480p/720p/1080p. Negative prices dropped.
//
// Model keys are walked in sorted order rather than in Go map order: several
// aliases can canonicalize onto the same family, and an unordered walk would
// make the winning price for a conflicting tier vary between processes.
// Unrecognized tiers are dropped with a warning instead of silently collapsing
// into the 480p bucket.
func NormalizeVideoModelPrices(in VideoModelPrices) VideoModelPrices {
	if len(in) == 0 {
		return nil
	}
	modelKeys := make([]string, 0, len(in))
	for modelKey := range in {
		modelKeys = append(modelKeys, modelKey)
	}
	sort.Strings(modelKeys)
	out := make(VideoModelPrices)
	for _, modelKey := range modelKeys {
		price := in[modelKey]
		family := CanonicalGrokImagineVideoPriceFamily(modelKey)
		if family == "" {
			key := strings.ToLower(strings.TrimSpace(modelKey))
			switch key {
			case VideoPriceFamilyGrokImagineVideo, VideoPriceFamilyGrokImagineVideo15:
				family = key
			default:
				if key == "" {
					continue
				}
				family = key
			}
		}
		normalized := out[family]
		normalized.BillingUnit = strings.ToLower(strings.TrimSpace(price.BillingUnit))
		mergeNormalizedVideoPrice(&normalized.Price480P, price.Price480P)
		mergeNormalizedVideoPrice(&normalized.Price720P, price.Price720P)
		mergeNormalizedVideoPrice(&normalized.Price1080P, price.Price1080P)
		mergeNormalizedVideoPrice(&normalized.Price1440P, price.Price1440P)
		mergeNormalizedVideoPrice(&normalized.Price2160P, price.Price2160P)
		if normalized.Price480P != nil || normalized.Price720P != nil || normalized.Price1080P != nil ||
			normalized.Price1440P != nil || normalized.Price2160P != nil {
			out[family] = normalized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeNormalizedVideoPrice(target **float64, source *float64) {
	if source == nil || math.IsNaN(*source) || math.IsInf(*source, 0) || *source < 0 {
		return
	}
	value := *source
	*target = &value
}

// LookupVideoModelPrice returns a price from a model and resolution card, or nil.
func LookupVideoModelPrice(prices VideoModelPrices, model, resolution string) *float64 {
	if len(prices) == 0 {
		return nil
	}
	family := CanonicalGrokImagineVideoPriceFamily(model)
	if family == "" {
		family = strings.ToLower(strings.TrimSpace(model))
	}
	if family == "" {
		return nil
	}
	price, ok := prices[family]
	if !ok {
		return nil
	}
	return videoModelPriceForResolution(price, resolution)
}
