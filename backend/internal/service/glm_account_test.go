package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestValidateGLMAccountConfiguration(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateGLMAccountConfiguration(
		PlatformGLM,
		AccountTypeAPIKey,
		map[string]any{"api_key": " glm-key "},
	))
	require.ErrorContains(t, ValidateGLMAccountConfiguration(
		PlatformGLM,
		AccountTypeOAuth,
		map[string]any{"api_key": "glm-key"},
	), "apikey")
	require.ErrorContains(t, ValidateGLMAccountConfiguration(
		PlatformGLM,
		AccountTypeAPIKey,
		map[string]any{},
	), "API key")
	require.NoError(t, ValidateGLMAccountConfiguration(PlatformOpenAI, AccountTypeOAuth, nil))
}

func TestBuildAccountForCreateForcesGLMChatCompletionsMode(t *testing.T) {
	t.Parallel()

	extra := map[string]any{
		"custom":                                      "preserved",
		openai_compat.ExtraKeyResponsesMode:           "auto",
		openai_compat.ExtraKeyResponsesSupported:      true,
	}
	account, err := buildAccountForCreate(&CreateAccountInput{
		Name:        "glm-account",
		Platform:    PlatformGLM,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "glm-key"},
		Concurrency: 1,
	}, extra)

	require.NoError(t, err)
	require.Equal(t, "preserved", account.Extra["custom"])
	require.Equal(t,
		string(openai_compat.ResponsesSupportModeForceChatCompletions),
		account.Extra[openai_compat.ExtraKeyResponsesMode],
	)
	require.NotContains(t, account.Extra, openai_compat.ExtraKeyResponsesSupported)
}

func TestNormalizeGLMAccountExtraDoesNotChangeOtherPlatforms(t *testing.T) {
	t.Parallel()

	extra := map[string]any{"custom": "value"}
	got := normalizeGLMAccountExtra(PlatformAnthropic, extra)

	require.Equal(t, map[string]any{"custom": "value"}, got)
	require.NotContains(t, got, openai_compat.ExtraKeyResponsesMode)
}

func TestGLMUsesOpenAICompatibleRuntimeBlockingWithoutOAuthClassification(t *testing.T) {
	t.Parallel()

	account := &Account{Platform: PlatformGLM, Type: AccountTypeAPIKey}
	require.True(t, isOpenAIAccount(account))
	require.False(t, isOpenAIOAuthAccount(account))
}
