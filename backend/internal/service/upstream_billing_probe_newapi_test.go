package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseNewAPIRateProbeResponseOfficialQuotaOnly(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	body := []byte(`{
		"object":"token_usage",
		"total_granted":1000000,
		"total_used":1234,
		"total_available":998766,
		"unlimited_quota":false
	}`)
	data, detected, found := parseNewAPIRateProbeResponse(body, now)
	require.True(t, detected)
	require.False(t, found)
	require.Nil(t, data)
}

func TestParseNewAPIRateProbeResponseWithGroupRatio(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	body := []byte(`{
		"code":true,
		"data":{
			"object":"token_usage",
			"group_ratio":1.25,
			"total_granted":10
		}
	}`)
	data, detected, found := parseNewAPIRateProbeResponse(body, now)
	require.True(t, detected)
	require.True(t, found)
	require.NotNil(t, data)
	require.Equal(t, "upstream.rate_probe", data["object"])
	require.Equal(t, "token", data["billing_scope"])
	require.Equal(t, "newapi", data["provider"])
	require.InDelta(t, 1.25, data["resolved_rate_multiplier"], 1e-9)
	require.InDelta(t, 1.25, data["effective_rate_multiplier"], 1e-9)
}

func TestParseNewAPIRateProbeResponseRejectsNegative(t *testing.T) {
	now := time.Now().UTC()
	body := []byte(`{"object":"token_usage","group_ratio":-1}`)
	_, detected, found := parseNewAPIRateProbeResponse(body, now)
	require.True(t, detected)
	require.False(t, found)
}

func TestUpstreamBillingProbeFallsBackToNewAPIRate(t *testing.T) {
	account := &Account{
		ID:          91,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example",
		},
		Groups: []*Group{{ID: 7, RateMultiplier: 1.0}},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("not found")),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"object":"token_usage",
				"group_ratio":0.9,
				"total_granted":1
			}`)),
		},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.August, 10, 3, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.SetAccountEnabled(context.Background(), account.ID, true))
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.InDelta(t, 0.9, snapshot.Data["effective_rate_multiplier"], 1e-9)
	require.Equal(t, "newapi", snapshot.Data["provider"])
	require.Len(t, upstream.requests, 2)
	require.Contains(t, upstream.requests[0].URL.Path, "/v1/sub2api/billing")
	require.Contains(t, upstream.requests[1].URL.Path, "/api/usage/token")

	// Safe rate should be ok (0.9 <= sell 1.0).
	statusRaw := account.Extra[AccountExtraSafeRateStatus]
	require.NotNil(t, statusRaw)
}

func TestUpstreamBillingProbeNewAPIRateNotExposed(t *testing.T) {
	account := &Account{
		ID:          92,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example",
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("not found")),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"object":"token_usage",
				"total_granted":100,
				"total_used":1,
				"total_available":99
			}`)),
		},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
	require.Equal(t, "rate_not_exposed", snapshot.LastError)
}

func TestRefreshGroupBoundAccountsSafeRateStatusOnSellRateChange(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	group := &Group{ID: 3, RateMultiplier: 0.5} // sell baseline drops below upstream 0.8
	account := &Account{
		ID: 11,
		Extra: map[string]any{
			AccountExtraUpstreamDeclaredRate: 0.8,
		},
		Groups: []*Group{{ID: 3, RateMultiplier: 1.0}}, // stale hydrated group
	}

	accountRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	groupRepo := &groupIDsForSafeRateStub{ids: map[int64][]int64{3: {11}}}

	RefreshGroupBoundAccountsSafeRateStatus(context.Background(), groupRepo, accountRepo, group, now)

	raw := account.Extra[AccountExtraSafeRateStatus]
	require.NotNil(t, raw)
	status, ok := raw.(SafeRateStatus)
	require.True(t, ok)
	require.Equal(t, SafeRateStatusOverSafe, status.Status)
	require.NotNil(t, status.UpstreamRate)
	require.InDelta(t, 0.8, *status.UpstreamRate, 1e-9)
	require.NotNil(t, status.SafeRateBaseline)
	require.InDelta(t, 0.5, *status.SafeRateBaseline, 1e-9)
}

type groupIDsForSafeRateStub struct {
	ids map[int64][]int64
}

func (g *groupIDsForSafeRateStub) GetAccountIDsByGroupIDs(_ context.Context, groupIDs []int64) ([]int64, error) {
	out := make([]int64, 0)
	seen := map[int64]struct{}{}
	for _, id := range groupIDs {
		for _, accountID := range g.ids[id] {
			if _, ok := seen[accountID]; ok {
				continue
			}
			seen[accountID] = struct{}{}
			out = append(out, accountID)
		}
	}
	return out, nil
}
