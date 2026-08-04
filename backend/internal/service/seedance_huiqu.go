package service

import (
	"context"
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

	DefaultHuiquVideoBaseURL = "https://api.bjhuiqu.net"

	huiquVideoCreatePath   = "/v1/videos/generations"
	huiquVideoTaskPath     = "/v1/videos"
	huiquPublicTaskPrefix  = "hqv1_"
	huiquMaxImageBytes     = int64(30_000_000)
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
}

func isHuiquVideoModel(model string) bool {
	_, ok := huiquVideoModels[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

func IsHuiquVideoModel(model string) bool {
	return isHuiquVideoModel(model)
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
	default:
		return trimmed
	}
}

// huiquUpstreamModelFor resolves a public MX933 model to the fixed-duration
// provider model. Legacy -1s bindings with non-standard durations are passed
// through only so already-created fallback tasks remain recoverable.
func huiquUpstreamModelFor(model string, duration int) (string, error) {
	model = strings.ToLower(strings.TrimSpace(model))
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
		return VideoProviderFFLink, nil
	}
	switch provider {
	case VideoProviderFFLink:
		return provider, nil
	case VideoProviderHuiqu:
		if platform != PlatformSeedance {
			return "", fmt.Errorf("video provider %s is only supported by the seedance platform", provider)
		}
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported video provider: %s", provider)
	}
}

func videoProviderSupportsModel(provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = VideoProviderFFLink
	}
	if provider == VideoProviderHuiqu {
		return isHuiquVideoModel(model)
	}
	return !isHuiquVideoModel(model)
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
	return a != nil && a.IsSeedance() && a.Type == AccountTypeAPIKey && a.GetVideoProvider() == VideoProviderHuiqu
}

func publicSeedanceTaskID(provider, upstreamTaskID string) (string, error) {
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	if !seedanceTaskIDPattern.MatchString(upstreamTaskID) {
		return "", errors.New("invalid Seedance upstream task id")
	}
	if provider != VideoProviderHuiqu {
		return upstreamTaskID, nil
	}
	publicID := huiquPublicTaskPrefix + upstreamTaskID
	if !seedanceTaskIDPattern.MatchString(publicID) {
		return "", errors.New("Seedance upstream task id is too long")
	}
	return publicID, nil
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
	if i.HasReferenceMedia() {
		return nil, errors.New("Huiqu reference media requires multipart/form-data")
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

func (s *SeedanceMediaService) PrepareHuiquMedia(ctx context.Context, info *SeedanceRequestInfo) (*SeedanceHuiquPreparedMedia, error) {
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
	download := func(source, kind, label string, index int) (*SeedanceHuiquMediaFile, error) {
		limit := huiquMaxImageBytes
		switch kind {
		case "video":
			limit = huiquMaxVideoBytes
		case "audio":
			limit = huiquMaxAudioBytes
		}
		file, err := s.downloadHuiquMedia(ctx, source, kind, label, index, limit)
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
			return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "request_too_large", "Huiqu multipart media must not exceed 384 MiB")
		}
		return file, nil
	}

	var err error
	if strings.TrimSpace(info.StartFrameURL) != "" {
		prepared.FirstFrame, err = download(info.StartFrameURL, "image", "first-frame", 0)
		if err != nil {
			return fail(err)
		}
	}
	if strings.TrimSpace(info.EndFrameURL) != "" {
		prepared.LastFrame, err = download(info.EndFrameURL, "image", "last-frame", 0)
		if err != nil {
			return fail(err)
		}
	}
	for index, reference := range info.References {
		file, fileErr := download(reference.URL, "image", "image", index+1)
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Images = append(prepared.Images, *file)
	}
	for index, reference := range info.VideoReferences {
		file, fileErr := download(reference.URL, "video", "video", index+1)
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Videos = append(prepared.Videos, *file)
	}
	for index, reference := range info.AudioReferences {
		file, fileErr := download(reference.URL, "audio", "audio", index+1)
		if fileErr != nil {
			return fail(fileErr)
		}
		prepared.Audios = append(prepared.Audios, *file)
	}
	return prepared, nil
}

func (s *SeedanceMediaService) downloadHuiquMedia(
	ctx context.Context,
	source, kind, label string,
	index int,
	limit int64,
) (*SeedanceHuiquMediaFile, error) {
	validated, err := validateSeedanceMediaRemoteURL(source)
	if err != nil {
		return nil, infraerrors.BadRequest("invalid_media_url", err.Error())
	}
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
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "failed to download reference media").WithCause(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", fmt.Sprintf("reference media returned HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > limit {
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
	written, err := io.Copy(tmp, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "media_fetch_failed", "failed to read reference media").WithCause(err)
	}
	if written == 0 {
		return nil, infraerrors.BadRequest("invalid_media", "reference media must not be empty")
	}
	if written > limit {
		return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "media_too_large", fmt.Sprintf("reference %s exceeds the upstream size limit", kind))
	}
	parsed, _ := url.Parse(validated)
	originalName := filepath.Base(parsed.Path)
	contentType, extension, err := inspectHuiquMedia(tmp, resp.Header.Get("Content-Type"), originalName, kind)
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
	fields := []struct{ name, value string }{
		{"model", strings.TrimSpace(upstreamModel)},
		{"prompt", info.Prompt},
		{"seconds", strconv.Itoa(info.DurationSeconds)},
		{"aspect_ratio", info.AspectRatio},
		{"resolution", info.Resolution},
		{"generate_audio", strconv.FormatBool(info.GenerateAudio)},
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
	if info.HuiquMedia.FirstFrame != nil {
		if err := writeFile("first_frame", *info.HuiquMedia.FirstFrame); err != nil {
			return nil, err
		}
	}
	if info.HuiquMedia.LastFrame != nil {
		if err := writeFile("last_frame", *info.HuiquMedia.LastFrame); err != nil {
			return nil, err
		}
	}
	for _, media := range info.HuiquMedia.Images {
		if err := writeFile("images", media); err != nil {
			return nil, err
		}
	}
	for _, media := range info.HuiquMedia.Videos {
		if err := writeFile("videos", media); err != nil {
			return nil, err
		}
	}
	for _, media := range info.HuiquMedia.Audios {
		if err := writeFile("audios", media); err != nil {
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
