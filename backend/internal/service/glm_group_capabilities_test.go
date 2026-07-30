package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeGLMExclusiveGroupFields(t *testing.T) {
	price := 0.25
	group := &Group{
		Platform:                    PlatformGLM,
		AllowImageGeneration:        true,
		AllowBatchImageGeneration:   true,
		ImageRateIndependent:        true,
		ImagePrice1K:                &price,
		ImagePrice2K:                &price,
		ImagePrice4K:                &price,
		VideoRateIndependent:        true,
		VideoPrice480P:              &price,
		VideoPrice720P:              &price,
		VideoPrice1080P:             &price,
		VideoModelPrices:            VideoModelPrices{"cogvideo": {Price480P: &price}},
		WebSearchPricePerCall:       &price,
		ClaudeCodeOnly:              true,
		AllowLive:                   true,
		RequireOAuthOnly:            true,
		RequirePrivacySet:           true,
		AllowMessagesDispatch:       true,
		DefaultMappedModel:          "glm-5.2",
		MaxReasoningEffort:          "high",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{SonnetMappedModel: "glm-5.2"},
	}

	sanitizeGLMExclusiveGroupFields(group)

	require.False(t, group.AllowImageGeneration)
	require.False(t, group.AllowBatchImageGeneration)
	require.False(t, group.ImageRateIndependent)
	require.Nil(t, group.ImagePrice1K)
	require.Nil(t, group.ImagePrice2K)
	require.Nil(t, group.ImagePrice4K)
	require.False(t, group.VideoRateIndependent)
	require.Nil(t, group.VideoPrice480P)
	require.Nil(t, group.VideoPrice720P)
	require.Nil(t, group.VideoPrice1080P)
	require.Empty(t, group.VideoModelPrices)
	require.Nil(t, group.WebSearchPricePerCall)
	require.False(t, group.ClaudeCodeOnly)
	require.False(t, group.AllowLive)
	require.False(t, group.RequireOAuthOnly)
	require.False(t, group.RequirePrivacySet)

	require.True(t, group.AllowMessagesDispatch)
	require.Equal(t, "glm-5.2", group.DefaultMappedModel)
	require.Equal(t, "high", group.MaxReasoningEffort)
	require.Equal(t, "glm-5.2", group.MessagesDispatchModelConfig.SonnetMappedModel)
}

func TestSanitizeGLMExclusiveGroupFieldsDoesNotChangeOtherPlatforms(t *testing.T) {
	price := 0.25
	group := &Group{
		Platform:              PlatformOpenAI,
		AllowImageGeneration:  true,
		WebSearchPricePerCall: &price,
		AllowLive:             true,
		RequireOAuthOnly:      true,
		RequirePrivacySet:     true,
	}

	sanitizeGLMExclusiveGroupFields(group)

	require.True(t, group.AllowImageGeneration)
	require.Same(t, &price, group.WebSearchPricePerCall)
	require.True(t, group.AllowLive)
	require.True(t, group.RequireOAuthOnly)
	require.True(t, group.RequirePrivacySet)
}
