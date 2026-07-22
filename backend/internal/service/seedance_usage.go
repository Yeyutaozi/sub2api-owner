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
	if usageInput.APIKey == nil || usageInput.APIKey.Group == nil || usageInput.APIKey.Group.Platform != PlatformSeedance {
		return errors.New("seedance usage requires a Seedance API key group")
	}
	if usageInput.Account == nil || !usageInput.Account.IsSeedance() {
		return errors.New("seedance usage requires a Seedance account")
	}

	requestedModel := strings.TrimSpace(input.RequestedModel)
	if requestedModel == "" {
		return errors.New("seedance requested model is required")
	}
	requestID := SeedanceUsageRequestID(input.TaskID)
	if requestID == "" {
		return errors.New("seedance task id is required")
	}

	group := *usageInput.APIKey.Group
	group.VideoPrice480P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution480P))
	group.VideoPrice720P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution720P))
	group.VideoPrice1080P = cloneFloat64Pointer(group.GetVideoPriceForModel(requestedModel, VideoBillingResolution1080P))
	resolution := NormalizeVideoBillingResolutionOrDefault(usageInput.Result.VideoResolution)
	if group.GetVideoPrice(resolution) == nil {
		return fmt.Errorf("seedance video price is not configured for model %s at %s", requestedModel, resolution)
	}

	apiKey := *usageInput.APIKey
	apiKey.Group = &group
	usageInput.APIKey = &apiKey
	usageInput.OriginalModel = requestedModel
	usageInput.UsageRequestID = requestID

	result := *usageInput.Result
	result.RequestID = requestID
	usageInput.Result = &result

	if ctx == nil {
		ctx = context.Background()
	}
	return s.RecordUsage(ctx, &usageInput)
}
