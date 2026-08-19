package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSeedanceInspectionFromPayloadAcceptsOpenVideoState(t *testing.T) {
	inspection, err := seedanceInspectionFromPayload([]byte(`{"id":"tk_test","state":"succeeded"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inspection.Status != SeedanceTaskStatusSucceeded {
		t.Fatalf("status = %q, want %q", inspection.Status, SeedanceTaskStatusSucceeded)
	}
}

func TestSeedanceInspectionFromErrorMapsNotFound(t *testing.T) {
	inspection, ok := seedanceInspectionFromError(&SeedanceUpstreamError{
		StatusCode: http.StatusNotFound,
		Body:       []byte(`{"error":{"message":"task not found"}}`),
	})
	require.True(t, ok)
	require.Equal(t, SeedanceTaskStatusFailed, inspection.Status)
	require.NotEmpty(t, inspection.Error)
}

func TestSeedanceInspectionFromErrorMapsTerminalStatusBody(t *testing.T) {
	inspection, ok := seedanceInspectionFromError(&SeedanceUpstreamError{
		StatusCode: http.StatusBadRequest,
		Body:       []byte(`{"status":"failed","error":"provider rejected"}`),
	})
	require.True(t, ok)
	require.Equal(t, SeedanceTaskStatusFailed, inspection.Status)
	require.Contains(t, inspection.Error, "provider rejected")
}

func TestSeedanceInspectionFromErrorIgnoresTransient(t *testing.T) {
	inspection, ok := seedanceInspectionFromError(&SeedanceUpstreamError{
		StatusCode: http.StatusBadGateway,
		Body:       []byte(`{"error":{"message":"temporary upstream issue"}}`),
	})
	require.False(t, ok)
	require.Nil(t, inspection)
}

func TestSeedanceBindingExceedsSettlementMaxAge(t *testing.T) {
	now := time.Now().UTC()
	require.False(t, seedanceBindingExceedsSettlementMaxAge(&SeedanceTaskBinding{CreatedAt: now.Add(-30 * time.Minute)}, now))
	require.True(t, seedanceBindingExceedsSettlementMaxAge(&SeedanceTaskBinding{CreatedAt: now.Add(-3 * time.Hour)}, now))
}

func TestSeedanceSettlementForceFailsAfterMaxAgeOnPollError(t *testing.T) {
	binding := seedanceSettlementBinding()
	binding.CreatedAt = time.Now().UTC().Add(-3 * time.Hour)
	h := newSeedanceSettlementHarness(binding)
	h.inspectionErr = &SeedanceUpstreamError{StatusCode: http.StatusBadGateway, Body: []byte(`{"error":"bad gateway"}`)}

	h.worker.process(context.Background(), &binding)

	require.Equal(t, 1, h.refundCalls)
	require.Len(t, h.updates, 1)
	require.NotNil(t, h.updates[0].SettledAt)
	require.Equal(t, SeedanceTaskStatusFailed, h.updates[0].TaskStatus)
}
