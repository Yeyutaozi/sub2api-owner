package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	VideoBillingUnitPerSecond  = domain.VideoBillingUnitPerSecond
	VideoBillingUnitPerRequest = domain.VideoBillingUnitPerRequest
)

func normalizeVideoBillingUnit(platform, value string) (string, error) {
	unit := strings.ToLower(strings.TrimSpace(value))
	if unit == "" {
		unit = VideoBillingUnitPerSecond
	}
	if unit != VideoBillingUnitPerSecond && unit != VideoBillingUnitPerRequest {
		return "", fmt.Errorf("video_billing_unit must be %s or %s", VideoBillingUnitPerSecond, VideoBillingUnitPerRequest)
	}
	if unit == VideoBillingUnitPerRequest && platform != PlatformSeedance {
		return "", fmt.Errorf("video_billing_unit %s is only supported by the seedance platform", VideoBillingUnitPerRequest)
	}
	return unit, nil
}

// EffectiveVideoBillingUnit is fail-closed for legacy, incomplete, or invalid
// group snapshots. Only a Seedance group explicitly set to per_request can
// bypass duration multiplication.
func EffectiveVideoBillingUnit(platform, value string) string {
	unit, err := normalizeVideoBillingUnit(platform, value)
	if err != nil {
		return VideoBillingUnitPerSecond
	}
	return unit
}
