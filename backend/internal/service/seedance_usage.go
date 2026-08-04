package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SeedanceRecordUsageInput keeps Seedance-only billing identity and model
// selection out of the shared OpenAI/Grok usage contract.
type SeedanceRecordUsageInput struct {
	OpenAIRecordUsageInput
	TaskID         string
	RequestedModel string
}

// RecordSeedanceUsage projects the selected Seedance model price card onto a
// private copy of the legacy video price fields. RecordUsage can then retain
// the original OpenAI/Grok billing signatures and behavior.
func (s *OpenAIGatewayService) RecordSeedanceUsage(ctx context.Context, input *SeedanceRecordUsageInput) error {
	if input == nil {
		return errors.New("seedance usage input is nil")
	}
	usageInput := input.OpenAIRecordUsageInput
	if usageInput.Result == nil {
		return errors.New("seedance usage result is nil")
	}
	if usageInput.APIKey == nil || usageInput.APIKey.Group == nil || !IsFFLinkVideoPlatform(usageInput.APIKey.Group.Platform) {
		return errors.New("FFLink video usage requires a compatible API key group")
	}
	if usageInput.Account == nil || !usageInput.Account.IsFFLinkVideo() {
		return errors.New("FFLink video usage requires a compatible account")
	}

	requestedModel := strings.TrimSpace(input.RequestedModel)
	if requestedModel == "" {
		return errors.New("seedance requested model is required")
	}
	requestID := SeedanceUsageRequestID(input.TaskID)
	if requestID == "" {
		return errors.New("seedance task id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	group := *usageInput.APIKey.Group
	group.VideoModelPrices = cloneVideoModelPrices(group.VideoModelPrices)
	if s != nil && s.userGroupRateRepo != nil && usageInput.User != nil && usageInput.User.ID > 0 && group.ID > 0 {
		userPrices, err := s.userGroupRateRepo.GetVideoModelPricesByUserAndGroup(ctx, usageInput.User.ID, group.ID)
		if err != nil {
			return fmt.Errorf("load user video prices: %w", err)
		}
		if override, ok := findVideoModelPrice(group.Platform, userPrices, requestedModel); ok {
			base, _ := findVideoModelPrice(group.Platform, group.VideoModelPrices, requestedModel)
			group.VideoModelPrices[strings.ToLower(requestedModel)] = mergeVideoModelPrice(base, override)
		}
	}
	group.VideoPrice480P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution480P))
	group.VideoPrice720P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution720P))
	group.VideoPrice1080P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution1080P))
	resolution := NormalizeVideoBillingResolutionOrDefault(usageInput.Result.VideoResolution)
	if group.GetVideoPriceForModel(requestedModel, resolution) == nil {
		return fmt.Errorf("seedance video price is not configured for model %s at %s", requestedModel, resolution)
	}

	apiKey := *usageInput.APIKey
	apiKey.Group = &group
	usageInput.APIKey = &apiKey
	usageInput.OriginalModel = requestedModel
	usageInput.UsageRequestID = requestID
	usageInput.DurableUsageLog = true

	result := *usageInput.Result
	result.RequestID = requestID
	result.BillingModel = requestedModel
	usageInput.Result = &result

	return s.RecordUsage(ctx, &usageInput)
}

func mergeVideoModelPrice(base, override VideoModelPrice) VideoModelPrice {
	out := VideoModelPrice{
		Price480P: cloneFloat64Pointer(base.Price480P), Price720P: cloneFloat64Pointer(base.Price720P),
		Price1080P: cloneFloat64Pointer(base.Price1080P), Price1440P: cloneFloat64Pointer(base.Price1440P),
		Price2160P: cloneFloat64Pointer(base.Price2160P),
	}
	if override.Price480P != nil {
		out.Price480P = cloneFloat64Pointer(override.Price480P)
	}
	if override.Price720P != nil {
		out.Price720P = cloneFloat64Pointer(override.Price720P)
	}
	if override.Price1080P != nil {
		out.Price1080P = cloneFloat64Pointer(override.Price1080P)
	}
	if override.Price1440P != nil {
		out.Price1440P = cloneFloat64Pointer(override.Price1440P)
	}
	if override.Price2160P != nil {
		out.Price2160P = cloneFloat64Pointer(override.Price2160P)
	}
	return out
}
