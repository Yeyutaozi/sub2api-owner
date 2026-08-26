package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	VideoProviderFFLink = "fflink"
	VideoProviderHuiqu  = "huiqu"

	SeedanceMX933Model           = "sd2-mx933"
	SeedanceMX933FastModel       = "sd2-mx933-fast"
	SeedanceMX933LegacyModel     = "sd2-mx933-720-1s"
	SeedanceMX933LegacyFastModel = "sd2-mx933-720-fast-1s"

	// Public MiniMax H3 model id for Huiqu. Upstream keeps the fixed provider name.
	SeedanceMiniMaxH3Model         = "minimax-h3"
	SeedanceMiniMaxH3UpstreamModel = "MiniMax-H3-933-1440P-GF"

	DefaultHuiquVideoBaseURL = "https://api.bjhuiqu.net"

	huiquVideoCreatePath   = "/v1/videos/generations"
	huiquVideoTaskPath     = "/v1/videos"
	huiquPublicTaskPrefix  = "hqv1_"
	huiquMaxImageBytes     = SeedanceMaxImageBytes // keep platform upload + Huiqu fetch limits aligned
	huiquMaxVideoBytes     = int64(50_000_000)
	huiquMaxAudioBytes     = int64(15_000_000)
	huiquMaxRequestBytes   = int64(384 << 20)
	huiquMediaFetchTimeout = 2 * time.Minute
)

var huiquVideoModels = map[string]struct{}{
	SeedanceMX933Model:           {},
	SeedanceMX933FastModel:       {},
	SeedanceMX933LegacyModel:     {},
	SeedanceMX933LegacyFastModel: {},
	SeedanceMiniMaxH3Model:       {},
	// Accept the provider-facing model name when accounts map 1:1.
	"minimax-h3-933-1440p-gf": {},
}

func isHuiquVideoModel(model string) bool {
	_, ok := huiquVideoModels[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

func IsHuiquVideoModel(model string) bool {
	return isHuiquVideoModel(model)
}

func isHuiquMiniMaxH3Model(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceMiniMaxH3Model, "minimax-h3-933-1440p-gf", strings.ToLower(SeedanceMiniMaxH3UpstreamModel):
		return true
	default:
		return false
	}
}

func isHuiquMiniMaxH3DurationSupported(duration int) bool {
	return duration >= 5 && duration <= 15
}

func huiquMiniMaxH3SizeFor(aspectRatio string) string {
	switch strings.ToLower(strings.TrimSpace(aspectRatio)) {
	case "9:16":
		return "1440x2560"
	default:
		// Docs advertise 16:9 / 9:16; default the 16:9 QHD tier from the H3 example.
		return "2560x1440"
	}
}

func huiquMiniMaxH3UpstreamResolution() string {
	return "1440P"
}

func isSeedanceDurationSupported(duration int) bool {
	return duration == 5 || duration == 10 || duration == 15
}

func isLegacyHuiquVariableDurationModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case SeedanceMX933LegacyModel, SeedanceMX933LegacyFastModel:
		return true
	default:
		return false
	}
}

// seedanceModelLookupCandidates keeps existing Huiqu account mappings and
// price cards usable while the public model IDs move away from the legacy
// per-second names. The exact requested model always wins when both exist.
func seedanceModelLookupCandidates(model string) []string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch model {
	case SeedanceMX933Model:
		return []string{SeedanceMX933Model, SeedanceMX933LegacyModel}
	case SeedanceMX933LegacyModel:
		return []string{SeedanceMX933LegacyModel, SeedanceMX933Model}
	case SeedanceMX933FastModel:
		return []string{SeedanceMX933FastModel, SeedanceMX933LegacyFastModel}
	case SeedanceMX933LegacyFastModel:
		return []string{SeedanceMX933LegacyFastModel, SeedanceMX933FastModel}
	default:
		return []string{model}
	}
}

// PublicSeedanceModelID keeps provider-only MX933 tier names out of client
// responses while preserving the stored model for legacy task recovery.
func PublicSeedanceModelID(model string) string {
	trimmed := strings.TrimSpace(model)
	switch strings.ToLower(trimmed) {
	case SeedanceMX933Model,
		SeedanceMX933LegacyModel,
		"sd2-mx933-720-5s",
		"sd2-mx933-720-10s",
		"sd2-mx933-720-15s":
		return SeedanceMX933Model
	case SeedanceMX933FastModel,
		SeedanceMX933LegacyFastModel,
		"sd2-mx933-720-fast-5s",
		"sd2-mx933-720-fast-10s",
		"sd2-mx933-720-fast-15s":
		return SeedanceMX933FastModel
	case SeedanceMiniMaxH3Model,
		"minimax-h3-933-1440p-gf",
		strings.ToLower(SeedanceMiniMaxH3UpstreamModel):
		return SeedanceMiniMaxH3Model
	default:
		return trimmed
	}
}

// huiquUpstreamModelFor resolves a public MX933 model to the fixed-duration
// provider model. Legacy -1s bindings with non-standard durations are passed
// through only so already-created fallback tasks remain recoverable.
// MiniMax H3 keeps a single upstream model name and accepts 5-15 second
// durations as arbitrary integers.
func huiquUpstreamModelFor(model string, duration int) (string, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	if isHuiquMiniMaxH3Model(model) {
		if !isHuiquMiniMaxH3DurationSupported(duration) {
			return "", fmt.Errorf("duration %d is not supported by model %s", duration, model)
		}
		return SeedanceMiniMaxH3UpstreamModel, nil
	}
	if isLegacyHuiquVariableDurationModel(model) && !isSeedanceDurationSupported(duration) {
		if duration >= 1 && duration <= 15 {
			return model, nil
		}
		return "", fmt.Errorf("duration %d is not supported by model %s", duration, model)
	}
	if !isSeedanceDurationSupported(duration) {
		return "", fmt.Errorf("duration %d is not supported by model %s", duration, model)
	}
	switch model {
	case SeedanceMX933Model, SeedanceMX933LegacyModel:
		return fmt.Sprintf("sd2-mx933-720-%ds", duration), nil
	case SeedanceMX933FastModel, SeedanceMX933LegacyFastModel:
		return fmt.Sprintf("sd2-mx933-720-fast-%ds", duration), nil
	default:
		return "", fmt.Errorf("unsupported Huiqu video model: %s", model)
	}
}

func IsHuiquSeedanceTaskID(taskID string) bool {
	return strings.HasPrefix(strings.TrimSpace(taskID), huiquPublicTaskPrefix)
}

func normalizeVideoProvider(platform, provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		// MiniMax product line defaults to huiqu today; seedance/ltx/happyhorse keep fflink.
		if platform == PlatformMiniMax {
			return VideoProviderHuiqu, nil
		}
		return VideoProviderFFLink, nil
	}
	switch provider {
	case VideoProviderFFLink:
		if platform == PlatformMiniMax {
			return "", fmt.Errorf("video provider %s is not supported by the minimax platform", provider)
		}
		return provider, nil
	case VideoProviderHuiqu:
		if platform != PlatformSeedance && platform != PlatformMiniMax {
			return "", fmt.Errorf("video provider %s is only supported by the seedance or minimax platforms", provider)
		}
		return provider, nil
	case VideoProviderXimei:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderWeijin:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderGlobalAIOPC:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderLensForge:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderOpenVideo:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderZhi168:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	case VideoProviderTianyue:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported video provider: %s", provider)
	}
}

func videoProviderSupportsModel(provider, model string) bool {
	return videoProviderSupportsModelForPlatform("", provider, model)
}

// videoProviderSupportsModelForPlatform keeps product platforms (seedance vs minimax)
// isolated while allowing the same upstream channel (e.g. huiqu) to host both.
func videoProviderSupportsModelForPlatform(platform, provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		if platform == PlatformMiniMax {
			provider = VideoProviderHuiqu
		} else {
			provider = VideoProviderFFLink
		}
	}
	switch provider {
	case VideoProviderHuiqu:
		if !isHuiquVideoModel(model) {
			return false
		}
		isH3 := isHuiquMiniMaxH3Model(model)
		switch platform {
		case PlatformMiniMax:
			return isH3
		case PlatformSeedance:
			return !isH3
		case "":
			// Legacy callers without platform context accept all huiqu models.
			return true
		default:
			return false
		}
	case VideoProviderXimei:
		return (platform == PlatformSeedance || platform == "") && isXimeiVideoModel(model)
	case VideoProviderWeijin:
		return (platform == PlatformSeedance || platform == "") && isWeijinVideoModel(model)
	case VideoProviderGlobalAIOPC:
		return (platform == PlatformSeedance || platform == "") && isGlobalAIOPCVideoModel(model)
	case VideoProviderLensForge:
		return (platform == PlatformSeedance || platform == "") && strings.EqualFold(strings.TrimSpace(model), SeedanceLensForge933Model)
	case VideoProviderOpenVideo:
		return (platform == PlatformSeedance || platform == "") && strings.EqualFold(strings.TrimSpace(model), SeedanceOpenVideoModel)
	case VideoProviderZhi168:
		return (platform == PlatformSeedance || platform == "") && strings.EqualFold(strings.TrimSpace(model), SeedanceZhi168Model)
	case VideoProviderTianyue:
		return (platform == PlatformSeedance || platform == "") && isTianyueVideoModel(model)
	default: // fflink and future non-opaque providers
		if platform == PlatformMiniMax {
			return false
		}
		return !isHuiquVideoModel(model) && !isXimeiVideoModel(model) && !isWeijinVideoModel(model) && !isGlobalAIOPCVideoModel(model)
	}
}

func (a *Account) GetVideoProvider() string {
	if a == nil || !a.IsFFLinkVideo() {
		return ""
	}
	provider, err := normalizeVideoProvider(a.Platform, a.GetCredential("video_provider"))
	if err != nil {
		return ""
	}
	return provider
}

func (a *Account) IsHuiquVideo() bool {
	return a != nil && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderHuiqu && (a.IsSeedance() || a.IsMiniMax())
}

func publicSeedanceTaskID(provider, upstreamTaskID string, accountID int64, idempotencyKey string) (string, error) {
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
		return "", errors.New("invalid Seedance upstream task id")
	}
	if provider == VideoProviderHuiqu {
		publicID := huiquPublicTaskPrefix + upstreamTaskID
		if !seedanceTaskIDPattern.MatchString(publicID) {
			return "", errors.New("Seedance upstream task id is too long")
		}
		return publicID, nil
	}
	if accountID <= 0 {
		return upstreamTaskID, nil
	}
	// Upstream task IDs are often only unique within one provider account and
	// may reset after a provider-side restart. Keep the public ID stable for a
	// retried idempotent request while separating account/task namespaces.
	digest := sha256.Sum256([]byte(fmt.Sprintf("seedance:%d:%s:%s", accountID, upstreamTaskID, strings.TrimSpace(idempotencyKey))))
	return "vidjob_" + base64.RawURLEncoding.EncodeToString(digest[:18]), nil
}

func upstreamSeedanceTaskID(provider, publicTaskID string) (string, error) {
	publicTaskID = strings.TrimSpace(publicTaskID)
	if !seedanceTaskIDPattern.MatchString(publicTaskID) {
		return "", errors.New("invalid Seedance task id")
	}
	isHuiquTask := strings.HasPrefix(publicTaskID, huiquPublicTaskPrefix)
	if provider == VideoProviderHuiqu && !isHuiquTask {
		return "", errors.New("Seedance task does not belong to the Huiqu provider")
	}
	if provider != VideoProviderHuiqu && isHuiquTask {
		return "", errors.New("Huiqu Seedance task cannot be forwarded through another provider")
	}
	if isHuiquTask {
		publicTaskID = strings.TrimPrefix(publicTaskID, huiquPublicTaskPrefix)
	}
	if !seedanceTaskIDPattern.MatchString(publicTaskID) {
		return "", errors.New("invalid Seedance upstream task id")
	}
	return publicTaskID, nil
}

func huiquSeedanceTaskAccountEligible(account *Account, groupID int64) bool {
	if account == nil || groupID <= 0 || !account.IsHuiquVideo() || !account.IsSchedulable() {
		return false
	}
	return openAIStickyAccountMatchesGroup(account, &groupID)
}

func (i *SeedanceRequestInfo) HasReferenceMedia() bool {
	return i != nil && (strings.TrimSpace(i.StartFrameURL) != "" ||
		strings.TrimSpace(i.EndFrameURL) != "" ||
		len(i.References) > 0 || len(i.VideoReferences) > 0 || len(i.AudioReferences) > 0)
}

func (i *SeedanceRequestInfo) HuiquUpstreamBody(upstreamModel string) ([]byte, error) {
	if i == nil {
		return nil, errors.New("seedance request info is required")
	}
	isH3 := isHuiquMiniMaxH3Model(i.Model) || strings.EqualFold(strings.TrimSpace(upstreamModel), SeedanceMiniMaxH3UpstreamModel)
	// MiniMax H3 on Huiqu/NewAPI only accepts application/json. Multipart bodies are
	// mis-parsed as JSON and fail with: invalid character '-' in numeric literal.
	// Reference media must be embedded as data URLs (or public URLs) inside JSON.
	if i.HasReferenceMedia() {
		if isH3 {
			return i.buildHuiquMiniMaxH3JSONBody(upstreamModel)
		}
		return nil, errors.New("Huiqu reference media requires multipart/form-data")
	}
	if isH3 {
		body := map[string]any{
			"model":        strings.TrimSpace(upstreamModel),
			"prompt":       i.Prompt,
			"seconds":      i.DurationSeconds,
			"aspect_ratio": i.AspectRatio,
			"resolution":   huiquMiniMaxH3UpstreamResolution(),
			"size":         huiquMiniMaxH3SizeFor(i.AspectRatio),
			// H3 always emits native audio; omit false to avoid upstream unsupported_parameter.
			"audio": true,
		}
		return json.Marshal(body)
	}
	body := map[string]any{
		"model":          strings.TrimSpace(upstreamModel),
		"prompt":         i.Prompt,
		"seconds":        i.DurationSeconds,
		"aspect_ratio":   i.AspectRatio,
		"resolution":     i.Resolution,
		"generate_audio": i.GenerateAudio,
	}
	return json.Marshal(body)
}

func huiquMediaFileDataURL(media SeedanceHuiquMediaFile) (string, error) {
	if strings.TrimSpace(media.Path) == "" {
		return "", errors.New("huiqu media path is required")
	}
	raw, err := os.ReadFile(media.Path)
	if err != nil {
		return "", err
	}
	contentType := strings.TrimSpace(media.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// buildHuiquMiniMaxH3JSONBody builds the documented H3 JSON payload including
// start_frame / end_frame / reference_images / audio_reference as data URLs.
func (i *SeedanceRequestInfo) buildHuiquMiniMaxH3JSONBody(upstreamModel string) ([]byte, error) {
	if i == nil {
		return nil, errors.New("seedance request info is required")
	}
	if i.HuiquMedia == nil {
		return nil, errors.New("Huiqu MiniMax H3 reference media is not prepared")
	}
	body := map[string]any{
		"model":        strings.TrimSpace(upstreamModel),
		"prompt":       i.Prompt,
		"seconds":      i.DurationSeconds,
		"aspect_ratio": i.AspectRatio,
		"resolution":   huiquMiniMaxH3UpstreamResolution(),
		"size":         huiquMiniMaxH3SizeFor(i.AspectRatio),
		"audio":        true,
	}
	if i.HuiquMedia.FirstFrame != nil {
		dataURL, err := huiquMediaFileDataURL(*i.HuiquMedia.FirstFrame)
		if err != nil {
			return nil, fmt.Errorf("encode start_frame: %w", err)
		}
		body["start_frame"] = dataURL
	}
	if i.HuiquMedia.LastFrame != nil {
		dataURL, err := huiquMediaFileDataURL(*i.HuiquMedia.LastFrame)
		if err != nil {
			return nil, fmt.Errorf("encode end_frame: %w", err)
		}
		body["end_frame"] = dataURL
	}
	if len(i.HuiquMedia.Images) > 0 {
		images := make([]string, 0, len(i.HuiquMedia.Images))
		for idx, media := range i.HuiquMedia.Images {
			dataURL, err := huiquMediaFileDataURL(media)
			if err != nil {
				return nil, fmt.Errorf("encode reference_images[%d]: %w", idx, err)
			}
			images = append(images, dataURL)
		}
		body["reference_images"] = images
	}
	if len(i.HuiquMedia.Audios) > 0 {
		audios := make([]string, 0, len(i.HuiquMedia.Audios))
		for idx, media := range i.HuiquMedia.Audios {
			dataURL, err := huiquMediaFileDataURL(media)
			if err != nil {
				return nil, fmt.Errorf("encode audio_reference[%d]: %w", idx, err)
			}
			audios = append(audios, dataURL)
		}
		body["audio_reference"] = audios
	}
	if len(i.HuiquMedia.Videos) > 0 {
		return nil, errors.New("MiniMax H3 does not support video references")
	}
	return json.Marshal(body)
}

type SeedanceHuiquMediaFile struct {
	Path        string
	Filename    string
	ContentType string
	SizeBytes   int64
}

type SeedanceHuiquPreparedMedia struct {
	FirstFrame *SeedanceHuiquMediaFile
	LastFrame  *SeedanceHuiquMediaFile
	Images     []SeedanceHuiquMediaFile
	Videos     []SeedanceHuiquMediaFile
	Audios     []SeedanceHuiquMediaFile
	paths      []string
}

func (m *SeedanceHuiquPreparedMedia) Cleanup() {
	if m == nil {
		return
	}
	for _, path := range m.paths {
		if strings.TrimSpace(path) != "" {
			_ = os.Remove(path)
		}
	}
	m.paths = nil
}

// PrepareHuiquMedia materializes reference media into local temp files so Huiqu
// payloads can embed them. MiniMax H3 embeds data URLs in JSON (upstream cannot
// fetch private COS URLs); MX933 still uses multipart file parts.
// Prefer reading from project COS / managed uploads; only fall back to HTTP for
// already-presigned object URLs that our backend can reach.
func (s *SeedanceMediaService) PrepareHuiquMedia(ctx context.Context, owner SeedanceMediaOwner, info *SeedanceRequestInfo) (*SeedanceHuiquPreparedMedia, error) {
	if s == nil || info == nil {
		return nil, infraerrors.BadRequest("invalid_request", "Seedance request info is required")
	}
	if !info.HasReferenceMedia() {
		return nil, nil
	}
	prepared := &SeedanceHuiquPreparedMedia{}
	fail := func(err error) (*SeedanceHuiquPreparedMedia, error) {
		prepared.Cleanup()
		return nil, err
	}

	var totalBytes int64
	var videoBytes int64
	mediaCount := 0
	download := func(source, kind, label string, index int, slot string) (*SeedanceHuiquMediaFile, error) {
		limit := huiquMaxImageBytes
		switch kind {
		case "video":
			limit = huiquMaxVideoBytes
		case "audio":
			limit = huiquMaxAudioBytes
		}
		file, err := s.downloadHuiquMedia(ctx, owner, info, source, kind, label, index, slot, limit)
		if err != nil {
			return nil, err
		}
		prepared.paths = append(prepared.paths, file.Path)
		mediaCount++
		totalBytes += file.SizeBytes
		if kind == "video" {
			videoBytes += file.SizeBytes
		}
		if mediaCount > 12 {
			return nil, infraerrors.BadRequest("too_many_media_files", "Huiqu requests support at most 12 reference media files")
		}
		if videoBytes > huiquMaxVideoBytes {
			return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", "reference videos must not exceed 50,000,000 bytes in total")
		}
		if totalBytes > huiquMaxRequestBytes {
			return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "request_too_large", "Huiqu reference media must not exceed 384 MiB")
		}
		return file, nil
	}

	var err error
	if strings.TrimSpace(info.StartFrameURL) != "" {
		prepared.FirstFrame, err = download(info.StartFrameURL, "image", "first-frame", 0, "start_frame")
		if err != nil {
			return fail(err)
		}
	}
	if strings.TrimSpace(info.EndFrameURL) != "" {
		prepared.LastFrame, err = download(info.EndFrameURL, "image", "last-frame", 0, "end_frame")
		if err != nil {
			return fail(err)
		}
	}
	for index, reference := range info.References {
		file, fileErr := download(reference.URL, "image", "image", index, "image_reference")
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Images = append(prepared.Images, *file)
	}
	for index, reference := range info.VideoReferences {
		file, fileErr := download(reference.URL, "video", "video", index, "video_reference")
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Videos = append(prepared.Videos, *file)
	}
	for index, reference := range info.AudioReferences {
		file, fileErr := download(reference.URL, "audio", "audio", index, "audio_reference")
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Audios = append(prepared.Audios, *file)
	}
	return prepared, nil
}

func (s *SeedanceMediaService) storedMediaLocation(info *SeedanceRequestInfo, slot string, index int) (AgentArtifactObjectLocation, bool) {
	if info == nil {
		return AgentArtifactObjectLocation{}, false
	}
	for _, item := range info.StoredMedia {
		if !strings.EqualFold(strings.TrimSpace(item.Slot), strings.TrimSpace(slot)) {
			continue
		}
		if item.Index != index {
			continue
		}
		if strings.TrimSpace(item.ObjectKey) == "" {
			continue
		}
		return AgentArtifactObjectLocation{
			StorageProvider: item.StorageProvider,
			Bucket:          item.Bucket,
			ObjectKey:       item.ObjectKey,
		}, true
	}
	return AgentArtifactObjectLocation{}, false
}

func (s *SeedanceMediaService) downloadHuiquMedia(
	ctx context.Context,
	owner SeedanceMediaOwner,
	info *SeedanceRequestInfo,
	source, kind, label string,
	index int,
	slot string,
	limit int64,
) (*SeedanceHuiquMediaFile, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, infraerrors.BadRequest("invalid_media_url", "reference media URL is required")
	}

	// 1) data URI already embedded by the client.
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		return s.materializeHuiquDataURI(source, kind, label, index, limit)
	}

	// 2) Durable COS object tracked during MaterializeImages.
	if location, ok := s.storedMediaLocation(info, slot, index); ok {
		if file, err := s.readHuiquMediaFromLocation(ctx, location, kind, label, index, limit); err == nil {
			return file, nil
		}
		// Fall through if the stored pointer is stale.
	}

	// 3) Managed upload URL (project COS behind /v1/videos/uploads/:id).
	if uploadID := managedSeedanceUploadID(source); uploadID != "" && validSeedanceMediaOwner(owner) {
		record, err := s.loadManagedUpload(ctx, owner, uploadID)
		if err != nil {
			return nil, err
		}
		return s.readHuiquMediaFromLocation(ctx, record.location(), kind, label, index, limit)
	}

	// 4) Own project COS URL recovered from the absolute media URL.
	if location, ok := s.seedanceObjectLocationFromOwnURL(owner, source); ok {
		if file, err := s.readHuiquMediaFromLocation(ctx, location, kind, label, index, limit); err == nil {
			return file, nil
		}
	}

	// 5) Presigned COS / remote HTTPS URL that this backend can fetch.
	// Upstream providers never receive this URL for H3 (embedded as data URL instead).
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_media_url", err.Error())
	}
	return s.downloadHuiquMediaHTTP(ctx, validated, kind, label, index, limit)
}

func (s *SeedanceMediaService) materializeHuiquDataURI(source, kind, label string, index int, limit int64) (*SeedanceHuiquMediaFile, error) {
	if kind != "image" {
		return nil, infraerrors.BadRequest("invalid_media_url", "only image data URIs are supported for Huiqu media preparation")
	}
	mediaType, encoded, err := splitSeedanceImageDataURI(source)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_media_url", err.Error())
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_media_url", "image data URI base64 payload is invalid")
	}
	if int64(len(raw)) == 0 {
		return nil, infraerrors.BadRequest("invalid_media", "reference media must not be empty")
	}
	if int64(len(raw)) > limit {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", fmt.Sprintf("reference %s exceeds the upstream size limit", kind))
	}
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "huiqu-media-*")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	failed := true
	defer func() {
		_ = tmp.Close()
		if failed {
			_ = os.Remove(path)
		}
	}()
	if _, err := tmp.Write(raw); err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	contentType, extension, err := inspectHuiquMedia(tmp, mediaType, label+".bin", kind)
	if err != nil {
		return nil, err
	}
	filename := label
	if index > 0 {
		filename += "-" + strconv.Itoa(index)
	}
	filename += "." + extension
	failed = false
	return &SeedanceHuiquMediaFile{Path: path, Filename: filename, ContentType: contentType, SizeBytes: int64(len(raw))}, nil
}

func (s *SeedanceMediaService) readHuiquMediaFromLocation(
	ctx context.Context,
	location AgentArtifactObjectLocation,
	kind, label string,
	index int,
	limit int64,
) (*SeedanceHuiquMediaFile, error) {
	if strings.TrimSpace(location.ObjectKey) == "" {
		return nil, infraerrors.ServiceUnavailable("media_storage_error", "Seedance media object location is invalid")
	}
	record := seedanceMediaRecord{
		StorageProvider: location.StorageProvider,
		Bucket:          location.Bucket,
		ObjectKey:       location.ObjectKey,
	}
	fetchCtx, cancel := context.WithTimeout(ctx, huiquMediaFetchTimeout)
	defer cancel()
	stream, err := s.openRecord(fetchCtx, record, "")
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "failed to read reference media from project storage").WithCause(err)
	}
	if stream == nil || stream.Body == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "project storage returned empty reference media")
	}
	defer func() { _ = stream.Body.Close() }()
	contentType := ""
	if stream.Header != nil {
		contentType = stream.Header.Get("Content-Type")
	}
	return s.writeHuiquMediaTemp(stream.Body, contentType, filepath.Base(location.ObjectKey), kind, label, index, limit)
}

func (s *SeedanceMediaService) downloadHuiquMediaHTTP(
	ctx context.Context,
	validated, kind, label string,
	index int,
	limit int64,
) (*SeedanceHuiquMediaFile, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, huiquMediaFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_media_url", "reference media URL is invalid")
	}
	req.Header.Set("Accept-Encoding", "identity")
	client := s.httpClient
	if client == nil {
		client = newSeedanceMediaHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "failed to download reference media from project storage").WithCause(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", fmt.Sprintf("reference media returned HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > limit {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", fmt.Sprintf("reference %s exceeds the upstream size limit", kind))
	}
	parsed, _ := url.Parse(validated)
	originalName := filepath.Base(parsed.Path)
	return s.writeHuiquMediaTemp(resp.Body, resp.Header.Get("Content-Type"), originalName, kind, label, index, limit)
}

func (s *SeedanceMediaService) writeHuiquMediaTemp(
	body io.Reader,
	declaredType, originalName, kind, label string,
	index int,
	limit int64,
) (*SeedanceHuiquMediaFile, error) {
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "huiqu-media-*")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	failed := true
	defer func() {
		_ = tmp.Close()
		if failed {
			_ = os.Remove(path)
		}
	}()
	written, err := io.Copy(tmp, io.LimitReader(body, limit+1))
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "failed to read reference media").WithCause(err)
	}
	if written == 0 {
		return nil, infraerrors.BadRequest("invalid_media", "reference media must not be empty")
	}
	if written > limit {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", fmt.Sprintf("reference %s exceeds the upstream size limit", kind))
	}
	contentType, extension, err := inspectHuiquMedia(tmp, declaredType, originalName, kind)
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	filename := label
	if index > 0 {
		filename += "-" + strconv.Itoa(index)
	}
	filename += "." + extension
	failed = false
	return &SeedanceHuiquMediaFile{Path: path, Filename: filename, ContentType: contentType, SizeBytes: written}, nil
}

func inspectHuiquMedia(file *os.File, declaredType, filename, kind string) (string, string, error) {
	contentType, extension, err := inspectSeedanceMedia(file, declaredType, filename, kind)
	if err != nil {
		return "", "", err
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	extension = strings.ToLower(strings.TrimSpace(extension))
	switch kind {
	case "image":
		if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
			return "", "", infraerrors.BadRequest("invalid_media_type", "reference images must be JPEG, PNG, or WebP")
		}
	case "video":
		if contentType != "video/mp4" && contentType != "video/quicktime" {
			return "", "", infraerrors.BadRequest("invalid_media_type", "reference videos must be MP4 or MOV")
		}
	case "audio":
		if contentType != "audio/mpeg" && contentType != "audio/mp3" && contentType != "audio/wav" &&
			contentType != "audio/x-wav" && contentType != "audio/wave" && contentType != "audio/vnd.wave" {
			return "", "", infraerrors.BadRequest("invalid_media_type", "reference audio must be MP3 or WAV")
		}
		if contentType == "audio/x-wav" || contentType == "audio/wave" || contentType == "audio/vnd.wave" {
			contentType = "audio/wav"
		}
	default:
		return "", "", infraerrors.BadRequest("invalid_media_type", "unsupported reference media type")
	}
	return contentType, extension, nil
}

type seedanceHuiquMultipartBody struct {
	File        *os.File
	Path        string
	ContentType string
	SizeBytes   int64
}

func (b *seedanceHuiquMultipartBody) Close() {
	if b == nil {
		return
	}
	if b.File != nil {
		_ = b.File.Close()
	}
	if b.Path != "" {
		_ = os.Remove(b.Path)
	}
}

func (b *seedanceHuiquMultipartBody) GetBody() (io.ReadCloser, error) {
	if b == nil || strings.TrimSpace(b.Path) == "" {
		return nil, errors.New("Huiqu multipart body is unavailable")
	}
	return os.Open(b.Path)
}

func buildHuiquMultipartBody(info *SeedanceRequestInfo, upstreamModel string) (*seedanceHuiquMultipartBody, error) {
	if info == nil || info.HuiquMedia == nil {
		return nil, errors.New("Huiqu multipart media is not prepared")
	}
	tmp, err := os.CreateTemp(seedanceTempDirectory(), "huiqu-request-*.multipart")
	if err != nil {
		return nil, err
	}
	body := &seedanceHuiquMultipartBody{File: tmp, Path: tmp.Name()}
	failed := true
	defer func() {
		if failed {
			body.Close()
		}
	}()

	writer := multipart.NewWriter(tmp)
	isH3 := isHuiquMiniMaxH3Model(info.Model) || strings.EqualFold(strings.TrimSpace(upstreamModel), SeedanceMiniMaxH3UpstreamModel)
	resolution := info.Resolution
	audioField := "generate_audio"
	audioEnabled := info.GenerateAudio
	if isH3 {
		resolution = huiquMiniMaxH3UpstreamResolution()
		audioField = "audio"
		// H3 always emits native audio; upstream rejects audio=false.
		audioEnabled = true
	}
	fields := []struct{ name, value string }{
		{"model", strings.TrimSpace(upstreamModel)},
		{"prompt", info.Prompt},
		{"seconds", strconv.Itoa(info.DurationSeconds)},
		{"aspect_ratio", info.AspectRatio},
		{"resolution", resolution},
		{audioField, strconv.FormatBool(audioEnabled)},
	}
	if isH3 {
		fields = append(fields, struct{ name, value string }{"size", huiquMiniMaxH3SizeFor(info.AspectRatio)})
	}
	for _, field := range fields {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return nil, err
		}
	}
	writeFile := func(field string, media SeedanceHuiquMediaFile) error {
		source, err := os.Open(media.Path)
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		filename := strings.ReplaceAll(filepath.Base(media.Filename), `"`, "")
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
		header.Set("Content-Type", media.ContentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(part, source)
		return err
	}
	firstFrameField := "first_frame"
	lastFrameField := "last_frame"
	imageField := "images"
	audioRefField := "audios"
	if isH3 {
		firstFrameField = "start_frame"
		lastFrameField = "end_frame"
		imageField = "reference_images"
		audioRefField = "audio_reference"
	}
	if info.HuiquMedia.FirstFrame != nil {
		if err := writeFile(firstFrameField, *info.HuiquMedia.FirstFrame); err != nil {
			return nil, err
		}
	}
	if info.HuiquMedia.LastFrame != nil {
		if err := writeFile(lastFrameField, *info.HuiquMedia.LastFrame); err != nil {
			return nil, err
		}
	}
	for _, media := range info.HuiquMedia.Images {
		if err := writeFile(imageField, media); err != nil {
			return nil, err
		}
	}
	if !isH3 {
		for _, media := range info.HuiquMedia.Videos {
			if err := writeFile("videos", media); err != nil {
				return nil, err
			}
		}
	}
	for _, media := range info.HuiquMedia.Audios {
		if err := writeFile(audioRefField, media); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	stat, err := tmp.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() > huiquMaxRequestBytes {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "request_too_large", "Huiqu multipart request must not exceed 384 MiB")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	body.ContentType = writer.FormDataContentType()
	body.SizeBytes = stat.Size()
	failed = false
	return body, nil
}
