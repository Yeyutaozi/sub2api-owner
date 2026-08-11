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



func TestUpstreamBillingProbeSub2AuthFailStillTriesNewAPIAccessToken(t *testing.T) {
	// NewAPI gateways often return 401/5xx on unknown /v1/sub2api/billing.
	// Probing must still fall through to Access Token group ratios.
	account := &Account{
		ID:          95,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example/v1",
		},
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			AccountExtraNewAPIUserID:      "7",
			AccountExtraNewAPIGroup:       "default",
		},
		Groups: []*Group{{ID: 1, RateMultiplier: 1.0, SafeRateMultiplier: 1.0}},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		// 1) sub2 hard-fail auth
		{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		},
		// 2) AT groups success (preferred path when AT configured)
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"code": true,
				"data": {
					"default": {"ratio": 0.75, "desc": "default"}
				}
			}`)),
		},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.InDelta(t, 0.75, snapshot.Data["effective_rate_multiplier"], 1e-9)
	require.Equal(t, "newapi", snapshot.Data["provider"])
	require.GreaterOrEqual(t, len(upstream.requests), 2)
	require.Contains(t, upstream.requests[0].URL.Path, "/v1/sub2api/billing")
	last := upstream.requests[len(upstream.requests)-1]
	require.Contains(t, last.URL.Path, "/api/user/self/groups")
	require.Equal(t, "Bearer access-token-xyz", last.Header.Get("Authorization"))
	require.Equal(t, "7", last.Header.Get("New-Api-User"))
}

func TestUpstreamBillingProbeSub2ServerErrorStillTriesSkUsage(t *testing.T) {
	account := &Account{
		ID:          96,
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
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"object":"token_usage",
				"group_ratio":1.1,
				"total_granted":1
			}`)),
		},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.InDelta(t, 1.1, snapshot.Data["effective_rate_multiplier"], 1e-9)
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


func TestParseNewAPIUserGroupsRatePreferredAndDefault(t *testing.T) {
	body := []byte(`{
		"code": true,
		"data": {
			"default": {"ratio": 1.0, "desc": "默认"},
			"vip": {"ratio": 0.5, "desc": "VIP"}
		}
	}`)
	rate, name, ok := parseNewAPIUserGroupsRate(body, "vip")
	require.True(t, ok)
	require.Equal(t, "vip", name)
	require.InDelta(t, 0.5, rate, 1e-9)

	rate, name, ok = parseNewAPIUserGroupsRate(body, "")
	require.True(t, ok)
	require.Equal(t, "default", name)
	require.InDelta(t, 1.0, rate, 1e-9)
}

func TestParseNewAPIUserSelfGroup(t *testing.T) {
	body := []byte(`{"code":true,"data":{"id":7,"group":"vip","username":"u"}}`)
	g, ok := parseNewAPIUserSelfGroup(body)
	require.True(t, ok)
	require.Equal(t, "vip", g)
}

func TestUpstreamBillingProbeNewAPIAccessToken(t *testing.T) {
	account := &Account{
		ID:          93,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example",
		},
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			AccountExtraNewAPIUserID:      "42",
			AccountExtraNewAPIGroup:       "vip",
		},
		Groups: []*Group{{ID: 1, RateMultiplier: 1.0, SafeRateMultiplier: 1.0}},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
		upstream := &httpUpstreamRecorder{responses: []*http.Response{
		// 1) sub2api billing missing
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("not found")),
		},
		// 2) Access Token first (preferred group set → /api/user/self/groups only)
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"code": true,
				"data": {
					"default": {"ratio": 1.0},
					"vip": {"ratio": 0.75}
				}
			}`)),
		},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.August, 11, 3, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.InDelta(t, 0.75, snapshot.Data["effective_rate_multiplier"], 1e-9)
	require.Equal(t, "newapi_access_token", snapshot.Data["source"])
	require.Equal(t, "vip", snapshot.Data["group_name"])
	require.GreaterOrEqual(t, len(upstream.requests), 2)
	// Last request should be groups with Access Token + New-Api-User
	last := upstream.requests[len(upstream.requests)-1]
	require.Contains(t, last.URL.Path, "/api/user/self/groups")
	require.Equal(t, "Bearer access-token-xyz", last.Header.Get("Authorization"))
	require.Equal(t, "42", last.Header.Get("New-Api-User"))
}

func TestUpstreamBillingProbeNewAPIAccessTokenMissingUserID(t *testing.T) {
	account := &Account{
		ID:          94,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example",
		},
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			// missing user id
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
	require.Equal(t, "newapi_user_id_missing", snapshot.LastError)
}

func TestRefreshGroupBoundAccountsSafeRateStatusOnSellRateChange(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	group := &Group{ID: 3, RateMultiplier: 1.0, SafeRateMultiplier: 0.5} // safe ceiling below upstream 0.8
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


func TestParseNewAPIUserGroupsRateArray(t *testing.T) {
	body := []byte(`{"success":true,"data":[{"name":"vip","ratio":1.5},{"name":"default","ratio":1}]}`)
	rate, name, ok := parseNewAPIUserGroupsRate(body, "vip")
	require.True(t, ok)
	require.Equal(t, "vip", name)
	require.InDelta(t, 1.5, rate, 1e-9)

	rate, name, ok = parseNewAPIUserGroupsRate(body, "")
	require.True(t, ok)
	require.Equal(t, "default", name)
	require.InDelta(t, 1.0, rate, 1e-9)
}


func TestUpstreamBillingProbeNewAPIWithV1BaseURL(t *testing.T) {
	// Operators usually paste NewAPI OpenAI base as https://host/v1.
	// Management APIs must still hit host /api/* (not /v1/api/*).
	account := &Account{
		ID:          95,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://newapi.example/v1",
		},
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			AccountExtraNewAPIUserID:      "7",
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
		upstream := &httpUpstreamRecorder{responses: []*http.Response{
		// sub2 missing
		{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("nf"))},
		// AT-first: no preferred group → /api/user/self then groups
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"group":"default"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"default":{"ratio":1.2}}}`))},
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.InDelta(t, 1.2, snapshot.Data["effective_rate_multiplier"], 1e-9)
	// Ensure no request went to /v1/api/*
	for _, req := range upstream.requests {
		require.NotContains(t, req.URL.Path, "/v1/api/")
	}
	// usage + self + groups should be under /api
	var sawSelf, sawGroups bool
	for _, req := range upstream.requests {
		switch {
		case strings.HasSuffix(req.URL.Path, "/api/user/self"):
			sawSelf = true
		case strings.HasSuffix(req.URL.Path, "/api/user/self/groups"):
			sawGroups = true
		}
	}
	require.True(t, sawSelf)
	require.True(t, sawGroups)
	// AT-first success may skip /api/usage/token
}


func TestAccountNewAPIAccessCredsNumericUserID(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			AccountExtraNewAPIUserID:      float64(42), // JSON number
			AccountExtraNewAPIGroup:       "vip",
		},
	}
	token, userID, group := accountNewAPIAccessCreds(account)
	require.Equal(t, "access-token-xyz", token)
	require.Equal(t, "42", userID)
	require.Equal(t, "vip", group)
}

func TestAccountNewAPIAccessCredsStringUserID(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			AccountExtraNewAPIAccessToken: "access-token-xyz",
			AccountExtraNewAPIUserID:      "7",
		},
	}
	token, userID, group := accountNewAPIAccessCreds(account)
	require.Equal(t, "access-token-xyz", token)
	require.Equal(t, "7", userID)
	require.Equal(t, "", group)
}

func TestExtraValueAsString(t *testing.T) {
	require.Equal(t, "12", extraValueAsString(map[string]any{"id": float64(12)}, "id"))
	require.Equal(t, "ab", extraValueAsString(map[string]any{"id": "ab"}, "id"))
	require.Equal(t, "", extraValueAsString(map[string]any{}, "id"))
}

