package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectOpenAIImageDownloadURLs(t *testing.T) {
	body := []byte(`{
		"data": [
			{"url": "https://cdn.example/one.png"},
			{"image_url": "https://signed.example/media?id=two"},
			{"url": "https://cdn.example/one.png"},
			{"b64_json": "aGVsbG8="}
		]
	}`)

	require.Equal(t, []string{
		"https://cdn.example/one.png",
		"https://signed.example/media?id=two",
	}, collectOpenAIImageDownloadURLs(body))
}
