package service

import "strings"

const (
	VideoBillingResolution480P  = "480p"
	VideoBillingResolution720P  = "720p"
	VideoBillingResolution1080P = "1080p"
	VideoBillingResolution1440P = "1440p"
	VideoBillingResolution2160P = "2160p"
)

// xAI 视频生成按秒计费，duration 请求参数允许 1-15 秒；未指定时上游默认生成 8 秒。
// 计费时长必须与上游实际消耗对齐，否则用户可通过拉长 duration 套利（提交时长由用户控制）。
const (
	VideoBillingMinDurationSeconds     = 1
	VideoBillingMaxDurationSeconds     = 15
	VideoBillingDefaultDurationSeconds = 8
)

// NormalizeVideoBillingDurationSecondsOrDefault 归一化计费用视频时长：
// 未指定（<=0）按上游默认 8 秒计，超出上游允许区间按边界收敛。
func NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds int) int {
	if durationSeconds <= 0 {
		return VideoBillingDefaultDurationSeconds
	}
	if durationSeconds < VideoBillingMinDurationSeconds {
		return VideoBillingMinDurationSeconds
	}
	if durationSeconds > VideoBillingMaxDurationSeconds {
		return VideoBillingMaxDurationSeconds
	}
	return durationSeconds
}

func NormalizeVideoBillingResolutionOrDefault(resolution string) string {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480", "480p", "sd":
		return VideoBillingResolution480P
	case "720", "720p", "hd":
		return VideoBillingResolution720P
	case "1080", "1080p", "full_hd", "full-hd", "fhd":
		return VideoBillingResolution1080P
	case "1440", "1440p", "2k", "qhd":
		return VideoBillingResolution1440P
	case "2160", "2160p", "4k", "uhd":
		return VideoBillingResolution2160P
	default:
		return VideoBillingResolution480P
	}
}

func NormalizeVideoBillingDurationSecondsForModelOrDefault(model string, durationSeconds int) int {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == SeedanceXimeiSD25Model {
		if durationSeconds <= 0 {
			return seedanceXimeiSD25DefaultDurationSeconds
		}
		if durationSeconds > seedanceXimeiSD25MaxDurationSeconds {
			return seedanceXimeiSD25MaxDurationSeconds
		}
		return durationSeconds
	}
	if model == SeedanceGlobalAIOPCC1Model {
		if durationSeconds <= 0 {
			return globalAIOPCDefaultDurationSeconds
		}
		if durationSeconds > globalAIOPCMaxDurationSeconds {
			return globalAIOPCMaxDurationSeconds
		}
		return durationSeconds
	}
	if strings.HasPrefix(model, "ltx-2.3-fast") {
		if durationSeconds <= 0 {
			return 6
		}
		if durationSeconds > 20 {
			return 20
		}
		return durationSeconds
	}
	if strings.HasPrefix(model, "ltx-2.3-pro") {
		if durationSeconds <= 0 {
			return 6
		}
		if durationSeconds > 10 {
			return 10
		}
		return durationSeconds
	}
	return NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
}
