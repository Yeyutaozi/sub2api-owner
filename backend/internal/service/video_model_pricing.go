package service

import (
	"fmt"
	"math"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func normalizeVideoModelPrices(platform string, prices VideoModelPrices) (VideoModelPrices, error) {
	if platform != PlatformSeedance || len(prices) == 0 {
		return VideoModelPrices{}, nil
	}

	out := make(VideoModelPrices, len(prices))
	for rawModel, rawPrice := range prices {
		model := strings.ToLower(strings.TrimSpace(rawModel))
		if model == "" {
			return nil, infraerrors.BadRequest("SEEDANCE_VIDEO_MODEL_REQUIRED", "video model name is required")
		}
		if _, exists := out[model]; exists {
			return nil, infraerrors.BadRequest(
				"SEEDANCE_VIDEO_MODEL_DUPLICATE",
				fmt.Sprintf("duplicate video model pricing: %s", model),
			)
		}

		price := VideoModelPrice{}
		var err error
		if price.Price480P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution480P, rawPrice.Price480P); err != nil {
			return nil, err
		}
		if price.Price720P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution720P, rawPrice.Price720P); err != nil {
			return nil, err
		}
		if price.Price1080P, err = normalizeVideoModelUnitPrice(model, VideoBillingResolution1080P, rawPrice.Price1080P); err != nil {
			return nil, err
		}
		if price.Price480P == nil && price.Price720P == nil && price.Price1080P == nil {
			return nil, infraerrors.BadRequest(
				"SEEDANCE_VIDEO_PRICE_REQUIRED",
				fmt.Sprintf("video model %s must configure at least one resolution price", model),
			)
		}
		out[model] = price
	}
	return out, nil
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
			Price480P:  cloneFloat64Pointer(price.Price480P),
			Price720P:  cloneFloat64Pointer(price.Price720P),
			Price1080P: cloneFloat64Pointer(price.Price1080P),
		}
	}
	return out
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
