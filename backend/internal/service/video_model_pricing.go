package service

import (
	"fmt"
	"math"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func normalizeVideoModelPrices(platform string, prices VideoModelPrices) (VideoModelPrices, error) {
	if !IsFFLinkVideoPlatform(platform) || len(prices) == 0 {
		return VideoModelPrices{}, nil
	}

	out := make(VideoModelPrices, len(prices))
	for rawModel, rawPrice := range prices {
		model := strings.ToLower(strings.TrimSpace(rawModel))
		if model == "" {
			return nil, infraerrors.BadRequest("SEEDANCE_VIDEO_MODEL_REQUIRED", "video model name is required")
		}
		if err := ValidateFFLinkVideoModelPlatform(platform, model); err != nil {
			return nil, infraerrors.BadRequest("FFLINK_VIDEO_MODEL_PLATFORM_MISMATCH", err.Error())
		}
		if _, exists := out[model]; exists {
			return nil, infraerrors.BadRequest(
				"SEEDANCE_VIDEO_MODEL_DUPLICATE",
				fmt.Sprintf("duplicate video model pricing: %s", model),
			)
		}

		price := VideoModelPrice{}
		var err error
		profile, _ := ffLinkVideoModelProfileFor(model)
		if err := validateConfiguredVideoResolutionPrices(model, profile, rawPrice); err != nil {
			return nil, err
		}
		if price.Price480P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution480P, rawPrice.Price480P); err != nil {
			return nil, err
		}
		if price.Price720P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution720P, rawPrice.Price720P); err != nil {
			return nil, err
		}
		if price.Price1080P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution1080P, rawPrice.Price1080P); err != nil {
			return nil, err
		}
		if price.Price1440P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution1440P, rawPrice.Price1440P); err != nil {
			return nil, err
		}
		if price.Price2160P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution2160P, rawPrice.Price2160P); err != nil {
			return nil, err
		}
		if price.BillingUnit, err = normalizeVideoModelBillingUnitOverride(platform, rawPrice.BillingUnit); err != nil {
			return nil, infraerrors.BadRequest(
				"SEEDANCE_VIDEO_BILLING_UNIT_INVALID",
				fmt.Sprintf("video model %s: %v", model, err),
			)
		}
		if price.Price480P == nil && price.Price720P == nil && price.Price1080P == nil && price.Price1440P == nil && price.Price2160P == nil {
			return nil, infraerrors.BadRequest(
				"SEEDANCE_VIDEO_PRICE_REQUIRED",
				fmt.Sprintf("video model %s must configure at least one resolution price", model),
			)
		}
		out[model] = price
	}
	return out, nil
}

func validateConfiguredVideoResolutionPrices(model string, profile ffLinkVideoModelProfile, price VideoModelPrice) error {
	configured := map[string]*float64{
		VideoBillingResolution480P: price.Price480P, VideoBillingResolution720P: price.Price720P,
		VideoBillingResolution1080P: price.Price1080P, VideoBillingResolution1440P: price.Price1440P,
		VideoBillingResolution2160P: price.Price2160P,
	}
	for resolution, value := range configured {
		if value == nil {
			continue
		}
		if _, ok := profile.AllowedResolutions[resolution]; !ok {
			return infraerrors.BadRequest(
				"FFLINK_VIDEO_RESOLUTION_PRICE_UNSUPPORTED",
				fmt.Sprintf("video model %s does not support %s pricing", model, resolution),
			)
		}
	}
	return nil
}

func normalizeVideoModelUnitPrice(model, resolution string, price *float64) (*float64, error) {
	if price == nil {
		return nil, nil
	}
	if math.IsNaN(*price) || math.IsInf(*price, 0) || *price < 0 {
		return nil, infraerrors.BadRequest(
			"SEEDANCE_VIDEO_PRICE_INVALID",
			fmt.Sprintf("video model %s %s price must be a finite number >= 0", model, resolution),
		)
	}
	value := *price
	return &value, nil
}

func cloneVideoModelPrices(prices VideoModelPrices) VideoModelPrices {
	if len(prices) == 0 {
		return VideoModelPrices{}
	}
	out := make(VideoModelPrices, len(prices))
	for model, price := range prices {
		out[model] = VideoModelPrice{
			BillingUnit: price.BillingUnit,
			Price480P:   cloneFloat64Pointer(price.Price480P),
			Price720P:   cloneFloat64Pointer(price.Price720P),
			Price1080P:  cloneFloat64Pointer(price.Price1080P),
			Price1440P:  cloneFloat64Pointer(price.Price1440P),
			Price2160P:  cloneFloat64Pointer(price.Price2160P),
		}
	}
	return out
}

func normalizeVideoModelBillingUnitOverride(platform, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	return normalizeVideoBillingUnit(platform, value)
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
