package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCachePublicVideoContentBindingRoundTripAndExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()
	payload := []byte(`{"request_id":"video_secret_123","user_id":7}`)

	require.NoError(t, cache.SetPublicVideoContentBinding(ctx, "video_secret_123", payload, 24*time.Hour))
	got, err := cache.GetPublicVideoContentBinding(ctx, "video_secret_123")
	require.NoError(t, err)
	require.Equal(t, payload, got)
	require.False(t, server.Exists(publicVideoContentPrefix+"video_secret_123"), "raw task IDs must not be used as Redis keys")

	server.FastForward(24*time.Hour + time.Second)
	got, err = cache.GetPublicVideoContentBinding(ctx, "video_secret_123")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGatewayCachePublicVideoContentBindingRejectsInvalidIDs(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}

	require.Error(t, cache.SetPublicVideoContentBinding(context.Background(), "", []byte(`{}`), time.Hour))
	_, err := cache.GetPublicVideoContentBinding(context.Background(), "")
	require.Error(t, err)
}
