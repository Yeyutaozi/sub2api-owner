package service

import "strings"

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &ImagePriceConfig{
		Price1K: apiKey.Group.ImagePrice1K,
		Price2K: apiKey.Group.ImagePrice2K,
		Price4K: apiKey.Group.ImagePrice4K,
	}
}

func apiKeyHasConfiguredImagePrice(apiKey *APIKey, imageSize string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetImagePrice(imageSize) != nil
}

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	return videoPriceConfigFromAPIKeyForModel(apiKey, "")
}

func videoPriceConfigFromAPIKeyForModel(apiKey *APIKey, model string) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	if IsFFLinkVideoPlatform(apiKey.Group.Platform) && len(apiKey.Group.VideoModelPrices) > 0 {
		price, ok := apiKey.Group.VideoModelPrices[strings.ToLower(strings.TrimSpace(model))]
		if !ok {
			return nil
		}
		return &VideoPriceConfig{
			Price480P: price.Price480P, Price720P: price.Price720P, Price1080P: price.Price1080P,
			Price1440P: price.Price1440P, Price2160P: price.Price2160P,
		}
	}
	return &VideoPriceConfig{
		Price480P:  apiKey.Group.VideoPrice480P,
		Price720P:  apiKey.Group.VideoPrice720P,
		Price1080P: apiKey.Group.VideoPrice1080P,
	}
}

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPrice(resolution) != nil
}

func apiKeyHasConfiguredVideoPriceForModel(apiKey *APIKey, model, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPriceForModel(model, resolution) != nil
}

func webSearchPricePerCallFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.WebSearchPricePerCall
}
