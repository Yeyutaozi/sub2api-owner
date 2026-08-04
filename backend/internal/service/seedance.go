package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

const (
	SeedanceOfficialTasksEndpoint   = "/api/v3/contents/generations/tasks"
	SeedanceOfficialUploadsEndpoint = "/api/v3/contents/generations/uploads"
	SeedancePublicJobsEndpoint      = "/v1/videos/jobs"
	SeedancePublicUploadsEndpoint   = "/v1/videos/uploads"
	DefaultSeedanceBaseURL          = "https://api.fflink.top"
	seedanceUpstreamCreatePath      = "/v1/videos/generations"
	seedanceUpstreamJobsPath        = "/v1/videos/jobs"
	seedanceTaskBindingTTL          = 7 * 24 * time.Hour
	SeedanceFallbackLeaseDuration   = 10 * time.Minute
	seedanceTaskStatusConcurrency   = 5
	DefaultSeedanceJobsLimit        = 50
	MaxSeedanceJobsLimit            = 100
)

var seedanceTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

var seedanceSensitiveQueryParamPattern = regexp.MustCompile(`(?i)((?:[?&]|\\u0026)(?:key|client_secret|access_token|refresh_token|token|sig|signature|credential|policy|ossaccesskeyid|x-amz-[a-z0-9-]+|x-goog-[a-z0-9-]+|q-[a-z0-9-]+|x-oss-[a-z0-9-]+)=)[^&"'\s\\},]+`)
var huiquCamelCaseResponseKeyPattern = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var huiquAbsoluteURLPattern = regexp.MustCompile(`(?i)(?:https?:)?//[^\s"'<>\\]+`)
var huiquSensitiveAssignmentPattern = regexp.MustCompile(`(?i)(\b(?:api[_-]?key|client[_-]?secret|access[_-]?token|refresh[_-]?token|token|sig|signature|credential|secret)\s*[=:]\s*)[^\s,;"'}]+`)
var huiquInternalModelPattern = regexp.MustCompile(`(?i)\bsd2-mx933-720(?:-fast)?-(?:1|5|10|15)s\b`)
var huiquProviderNamePattern = regexp.MustCompile(`(?i)\b(?:huiqu|bjhuiqu)\b`)

func ValidateSeedanceAccountConfiguration(platform, accountType string, credentials map[string]any) error {
	if !IsFFLinkVideoPlatform(platform) {
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return infraerrors.BadRequest("VIDEO_ACCOUNT_TYPE_INVALID", "video accounts must use the apikey account type")
	}
	apiKey, _ := credentials["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		return infraerrors.BadRequest("VIDEO_API_KEY_REQUIRED", "video accounts require an upstream API key")
	}
	providerValue := ""
	if raw, exists := credentials["video_provider"]; exists && raw != nil {
		var ok bool
		providerValue, ok = raw.(string)
		if !ok {
			return infraerrors.BadRequest("VIDEO_PROVIDER_INVALID", "video_provider must be a string")
		}
	}
	provider, err := normalizeVideoProvider(platform, providerValue)
	if err != nil {
		return infraerrors.BadRequest("VIDEO_PROVIDER_INVALID", err.Error())
	}
	if mapping := stringMappingFromRaw(credentials["model_mapping"]); len(mapping) > 0 {
		for requestedModel, upstreamModel := range mapping {
			if !videoProviderSupportsModel(provider, requestedModel) || !videoProviderSupportsModel(provider, upstreamModel) {
				return infraerrors.BadRequest(
					"VIDEO_PROVIDER_MODEL_MISMATCH",
					fmt.Sprintf("model %s is not supported by video provider %s", requestedModel, provider),
				)
			}
		}
	}
	return nil
}

// SeedanceCreateRequest is the Volcengine Ark-compatible video task request.
type SeedanceCreateRequest struct {
	Model         string                `json:"model"`
	Content       []SeedanceContentItem `json:"content"`
	Ratio         string                `json:"ratio,omitempty"`
	Duration      int                   `json:"duration,omitempty"`
	Resolution    string                `json:"resolution,omitempty"`
	GenerateAudio *bool                 `json:"generate_audio,omitempty"`
	Watermark     *bool                 `json:"watermark,omitempty"`
	Seed          *int64                `json:"seed,omitempty"`
	CameraFixed   *bool                 `json:"camera_fixed,omitempty"`
}

type SeedanceContentItem struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL json.RawMessage `json:"image_url,omitempty"`
	Role     string          `json:"role,omitempty"`
	Strength string          `json:"strength,omitempty"`
}

type SeedancePublicCreateRequest struct {
	Model         string                        `json:"model"`
	Prompt        string                        `json:"prompt,omitempty"`
	Resolution    string                        `json:"resolution,omitempty"`
	Duration      int                           `json:"duration,omitempty"`
	AspectRatio   string                        `json:"aspect_ratio,omitempty"`
	Audio         bool                          `json:"audio,omitempty"`
	PromptEnhance json.RawMessage               `json:"prompt_enhance,omitempty"`
	ImageURL      string                        `json:"image_url,omitempty"`
	StartFrameURL string                        `json:"start_frame_url,omitempty"`
	EndFrameURL   string                        `json:"end_frame_url,omitempty"`
	Guidances     SeedancePublicCreateGuidances `json:"guidances,omitempty"`
}

type SeedancePublicCreateGuidances struct {
	ImageReference     []SeedancePublicImageReference `json:"image_reference,omitempty"`
	VideoReferenceBase []SeedancePublicVideoReference `json:"video_reference_base,omitempty"`
	AudioReference     []SeedancePublicAudioReference `json:"audio_reference,omitempty"`
}

type SeedancePublicImageReference struct {
	Image    SeedancePublicMediaReference `json:"image"`
	Strength string                       `json:"strength,omitempty"`
	Order    int                          `json:"order,omitempty"`
}

type SeedancePublicVideoReference struct {
	Video SeedancePublicMediaReference `json:"video"`
	Order int                          `json:"order,omitempty"`
}

type SeedancePublicAudioReference struct {
	Audio SeedancePublicMediaReference `json:"audio"`
	Order int                          `json:"order,omitempty"`
}

type SeedancePublicMediaReference struct {
	URL  string `json:"url"`
	Type string `json:"type,omitempty"`
}

type seedanceImageInput struct {
	URL       string `json:"url"`
	Base64    string `json:"base64,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Role      string `json:"role,omitempty"`
	Strength  string `json:"strength,omitempty"`
}

type SeedanceRequestInfo struct {
	Model           string
	Prompt          string
	Resolution      string
	DurationSeconds int
	AspectRatio     string
	GenerateAudio   bool
	PromptEnhance   any
	StartFrameURL   string
	EndFrameURL     string
	References      []SeedanceReferenceImage
	VideoReferences []SeedanceReferenceVideo
	AudioReferences []SeedanceReferenceAudio
	StoredMedia     []SeedanceStoredMediaReference
	HuiquMedia      *SeedanceHuiquPreparedMedia
}

type SeedanceReferenceImage struct {
	URL      string
	Strength string
}

type SeedanceReferenceVideo struct {
	URL string
}

type SeedanceReferenceAudio struct {
	URL string
}

// SeedanceStoredMediaReference ties a request media slot to the durable object
// that backs its temporary URL. Fallback workers use it to issue a fresh URL
// instead of persisting an expiring presigned URL as the source of truth.
type SeedanceStoredMediaReference struct {
	Slot                  string `json:"slot"`
	Index                 int    `json:"index,omitempty"`
	StorageProvider       string `json:"storage_provider"`
	Bucket                string `json:"bucket"`
	ObjectKey             string `json:"object_key"`
	DeleteAfterSettlement bool   `json:"delete_after_settlement,omitempty"`
}

func (i *SeedanceRequestInfo) HasInlineImages() bool {
	if i == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.StartFrameURL)), "data:") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.EndFrameURL)), "data:") {
		return true
	}
	for _, reference := range i.References {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(reference.URL)), "data:") {
			return true
		}
	}
	return false
}

type SeedanceUpstreamResponse struct {
	StatusCode  int
	Header      http.Header
	Body        []byte
	BodyStream  io.ReadCloser
	ContentType string
	Streamed    bool
	Result      *OpenAIForwardResult
}

type SeedanceTaskBinding struct {
	ID                  int64
	UserID              int64
	APIKeyID            int64
	GroupID             int64
	AccountID           int64
	JobID               string
	UpstreamJobID       string
	Model               string
	FallbackModel       string
	FallbackStatus      string
	FallbackClaimToken  string
	FallbackLeaseUntil  time.Time
	RequestSnapshot     []byte
	TaskStatus          string
	NextPollAt          time.Time
	LastPolledAt        time.Time
	SettledAt           time.Time
	RefundedAt          time.Time
	RefundStatus        string
	RefundAttempts      int
	SettlementAttempts  int
	SettlementClaimedAt time.Time
	SettlementClaimedBy string
	LastError           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SeedanceTaskBindingRepository interface {
	SaveSeedanceTaskBinding(ctx context.Context, binding *SeedanceTaskBinding) error
	GetSeedanceTaskBinding(ctx context.Context, userID, apiKeyID, groupID int64, jobID string) (*SeedanceTaskBinding, error)
	ListSeedanceTaskBindings(ctx context.Context, userID, apiKeyID, groupID int64, limit int) ([]SeedanceTaskBinding, error)
}

type SeedanceTaskFallbackRepository interface {
	ClaimSeedanceTaskFallback(ctx context.Context, userID, apiKeyID, groupID int64, jobID string) (bool, string, error)
	ActivateSeedanceTaskFallback(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string, accountID int64, upstreamJobID string) (bool, error)
	FailSeedanceTaskFallback(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error)
	ReleaseSeedanceTaskFallback(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error)
	RenewSeedanceTaskFallback(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error)
}

type SeedanceTaskCancellationRepository interface {
	ClaimSeedanceTaskCancellation(ctx context.Context, userID, apiKeyID, groupID int64, jobID string) (bool, string, error)
	CompleteSeedanceTaskCancellation(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error)
	ReleaseSeedanceTaskCancellation(ctx context.Context, userID, apiKeyID, groupID int64, jobID, claimToken string) (bool, error)
}

type SeedanceTaskSettlementUpdate struct {
	TaskStatus         string
	NextPollAt         *time.Time
	SettledAt          *time.Time
	RefundedAt         *time.Time
	RefundStatus       string
	RefundAttempts     int
	SettlementAttempts int
	LastError          string
}

type SeedanceTaskSettlementRepository interface {
	ClaimSeedanceTaskSettlements(ctx context.Context, workerID string, limit int, leaseDuration time.Duration) ([]SeedanceTaskBinding, error)
	RenewSeedanceTaskSettlement(ctx context.Context, id int64, workerID string) (bool, error)
	CompleteSeedanceTaskSettlement(ctx context.Context, id int64, workerID string, update SeedanceTaskSettlementUpdate) (bool, error)
}

type seedanceIdempotencyKeyContextKey struct{}

func WithSeedanceIdempotencyKey(ctx context.Context, key string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, seedanceIdempotencyKeyContextKey{}, key)
}

func seedanceIdempotencyKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	key, _ := ctx.Value(seedanceIdempotencyKeyContextKey{}).(string)
	return strings.TrimSpace(key)
}

type SeedanceUpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *SeedanceUpstreamError) Error() string {
	if e == nil {
		return "seedance upstream request failed"
	}
	return fmt.Sprintf("seedance upstream returned status %d", e.StatusCode)
}

// SeedanceUpstreamAcceptanceUnknownError means a create request may have
// reached the provider, but the gateway could not prove whether it was accepted
// or recover its task identifier. Callers must keep the reservation recoverable
// and must not refund until the provider returns an explicit failure.
type SeedanceUpstreamAcceptanceUnknownError struct {
	Err error
}

func (e *SeedanceUpstreamAcceptanceUnknownError) Error() string {
	if e == nil || e.Err == nil {
		return "seedance upstream request acceptance is unknown"
	}
	return fmt.Sprintf("seedance upstream request acceptance is unknown: %v", e.Err)
}

func (e *SeedanceUpstreamAcceptanceUnknownError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ParseSeedanceCreateRequest(body []byte) (*SeedanceRequestInfo, error) {
	if len(body) == 0 {
		return nil, errors.New("request body is empty")
	}
	var request SeedanceCreateRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must contain exactly one JSON object")
	}
	if err := validateExplicitVideoDuration(body, request.Duration); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("model is required")
	}
	if len(request.Content) == 0 {
		return nil, errors.New("content is required")
	}
	if request.Seed != nil {
		return nil, errors.New("seed is not supported by the configured Seedance upstream")
	}
	if request.CameraFixed != nil && *request.CameraFixed {
		return nil, errors.New("camera_fixed is not supported by the configured Seedance upstream")
	}
	if request.Watermark != nil && *request.Watermark {
		return nil, errors.New("watermark=true is not supported by the configured Seedance upstream")
	}

	info := &SeedanceRequestInfo{
		Model:           strings.TrimSpace(request.Model),
		Resolution:      strings.ToLower(strings.TrimSpace(request.Resolution)),
		DurationSeconds: request.Duration,
		AspectRatio:     strings.TrimSpace(request.Ratio),
	}
	if request.GenerateAudio != nil {
		info.GenerateAudio = *request.GenerateAudio
	}
	var unroledImageSeen bool
	for _, item := range request.Content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "text":
			text := strings.TrimSpace(item.Text)
			if text != "" {
				if info.Prompt != "" {
					info.Prompt += "\n"
				}
				info.Prompt += text
			}
		case "image_url":
			imageInput, err := parseSeedanceImageInput(item)
			if err != nil {
				return nil, err
			}
			switch normalizeSeedanceImageRole(imageInput.Role) {
			case "first_frame":
				if info.StartFrameURL != "" {
					return nil, errors.New("only one first-frame image is allowed")
				}
				info.StartFrameURL = imageInput.URL
			case "last_frame":
				if info.EndFrameURL != "" {
					return nil, errors.New("only one last-frame image is allowed")
				}
				info.EndFrameURL = imageInput.URL
			case "reference_image":
				info.References = append(info.References, SeedanceReferenceImage{URL: imageInput.URL, Strength: normalizeSeedanceStrength(imageInput.Strength)})
			default:
				if unroledImageSeen || info.StartFrameURL != "" {
					return nil, errors.New("multiple image_url items require explicit roles")
				}
				unroledImageSeen = true
				info.StartFrameURL = imageInput.URL
			}
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	if info.Prompt == "" {
		return nil, errors.New("content must include a non-empty text item")
	}
	if info.EndFrameURL != "" && info.StartFrameURL == "" && !isHuiquVideoModel(info.Model) {
		return nil, errors.New("a last-frame image requires a first-frame image")
	}
	if len(info.References) > 0 && (info.StartFrameURL != "" || info.EndFrameURL != "") && !isHuiquVideoModel(info.Model) {
		return nil, errors.New("reference images cannot be combined with first/last frames")
	}
	if err := validateFFLinkVideoRequestInfo(info); err != nil {
		return nil, err
	}
	return info, nil
}

func ParseSeedanceVideoGenerationRequest(body []byte) (*SeedanceRequestInfo, error) {
	if len(body) == 0 {
		return nil, errors.New("request body is empty")
	}
	var request SeedancePublicCreateRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must contain exactly one JSON object")
	}
	if err := validateExplicitVideoDuration(body, request.Duration); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("model is required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	info := &SeedanceRequestInfo{
		Model:           strings.TrimSpace(request.Model),
		Prompt:          strings.TrimSpace(request.Prompt),
		Resolution:      strings.ToLower(strings.TrimSpace(request.Resolution)),
		DurationSeconds: request.Duration,
		AspectRatio:     strings.TrimSpace(request.AspectRatio),
		GenerateAudio:   request.Audio,
		StartFrameURL:   strings.TrimSpace(request.StartFrameURL),
		EndFrameURL:     strings.TrimSpace(request.EndFrameURL),
	}
	if len(request.PromptEnhance) > 0 && string(request.PromptEnhance) != "null" {
		var promptEnhance any
		if err := json.Unmarshal(request.PromptEnhance, &promptEnhance); err != nil {
			return nil, errors.New("prompt_enhance must be a boolean or string")
		}
		normalized, err := normalizeFFLinkPromptEnhance(promptEnhance, info.Model)
		if err != nil {
			return nil, err
		}
		info.PromptEnhance = normalized
	}

	if imageURL := strings.TrimSpace(request.ImageURL); imageURL != "" {
		if info.StartFrameURL != "" || info.EndFrameURL != "" || (len(request.Guidances.ImageReference) > 0 && !isHuiquVideoModel(info.Model)) {
			return nil, errors.New("image_url cannot be combined with start_frame_url, end_frame_url, or image references")
		}
		info.StartFrameURL = imageURL
	}

	if len(request.Guidances.ImageReference) > 0 {
		imageRefs := make([]SeedancePublicImageReference, len(request.Guidances.ImageReference))
		copy(imageRefs, request.Guidances.ImageReference)
		sort.SliceStable(imageRefs, func(i, j int) bool {
			if imageRefs[i].Order == imageRefs[j].Order {
				return i < j
			}
			return imageRefs[i].Order < imageRefs[j].Order
		})
		for _, item := range imageRefs {
			url := strings.TrimSpace(item.Image.URL)
			if url == "" {
				return nil, errors.New("guidances.image_reference.image.url is required")
			}
			switch {
			case (info.StartFrameURL != "" || info.EndFrameURL != "") && !isHuiquVideoModel(info.Model):
				return nil, errors.New("image references cannot be combined with start_frame_url or end_frame_url")
			}
			info.References = append(info.References, SeedanceReferenceImage{
				URL:      url,
				Strength: normalizeSeedanceStrength(item.Strength),
			})
		}
	}
	for _, item := range request.Guidances.VideoReferenceBase {
		url := strings.TrimSpace(item.Video.URL)
		if url == "" {
			return nil, errors.New("guidances.video_reference_base.video.url is required")
		}
		if !isSeedanceHTTPImageURL(url) {
			return nil, errors.New("video reference URL must be an absolute HTTP(S) URL")
		}
		info.VideoReferences = append(info.VideoReferences, SeedanceReferenceVideo{URL: url})
	}
	for _, item := range request.Guidances.AudioReference {
		url := strings.TrimSpace(item.Audio.URL)
		if url == "" {
			return nil, errors.New("guidances.audio_reference.audio.url is required")
		}
		if !isSeedanceHTTPImageURL(url) {
			return nil, errors.New("audio reference URL must be an absolute HTTP(S) URL")
		}
		info.AudioReferences = append(info.AudioReferences, SeedanceReferenceAudio{URL: url})
	}
	if info.EndFrameURL != "" && info.StartFrameURL == "" && !isHuiquVideoModel(info.Model) {
		return nil, errors.New("a last-frame image requires a first-frame image")
	}
	if len(info.References) > 0 && (info.StartFrameURL != "" || info.EndFrameURL != "") && !isHuiquVideoModel(info.Model) {
		return nil, errors.New("reference images cannot be combined with first/last frames")
	}
	if err := validateFFLinkVideoRequestInfo(info); err != nil {
		return nil, err
	}
	return info, nil
}

func validateExplicitVideoDuration(body []byte, duration int) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil
	}
	raw, provided := fields["duration"]
	if !provided {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || duration <= 0 {
		return errors.New("duration must be a positive integer when provided")
	}
	return nil
}

func parseSeedanceImageInput(item SeedanceContentItem) (*seedanceImageInput, error) {
	if len(item.ImageURL) == 0 || string(item.ImageURL) == "null" {
		return nil, errors.New("image_url is required for image content")
	}
	input := &seedanceImageInput{Role: item.Role, Strength: item.Strength}
	var directURL string
	if err := json.Unmarshal(item.ImageURL, &directURL); err == nil {
		input.URL = directURL
	} else {
		decoder := json.NewDecoder(bytes.NewReader(item.ImageURL))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(input); err != nil {
			return nil, errors.New("image_url must be a URL/data URI string or an object containing url or base64")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, errors.New("image_url must contain exactly one object")
		}
	}
	input.URL = strings.TrimSpace(input.URL)
	input.Base64 = strings.TrimSpace(input.Base64)
	if input.URL != "" && input.Base64 != "" {
		return nil, errors.New("image_url.url and image_url.base64 are mutually exclusive")
	}
	if input.Base64 != "" {
		mediaType := normalizeSeedanceInlineImageMediaType(input.MediaType)
		if mediaType == "" {
			return nil, errors.New("image_url.media_type must be image/png, image/jpeg, or image/webp when base64 is used")
		}
		input.URL = "data:" + mediaType + ";base64," + input.Base64
	}
	if input.URL == "" {
		return nil, errors.New("image_url.url or image_url.base64 is required")
	}
	if strings.HasPrefix(strings.ToLower(input.URL), "data:") {
		if _, _, err := splitSeedanceImageDataURI(input.URL); err != nil {
			return nil, err
		}
		return input, nil
	}
	parsed, err := url.Parse(input.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("image_url.url must be an absolute HTTP(S) URL")
	}
	return input, nil
}

func normalizeSeedanceInlineImageMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func splitSeedanceImageDataURI(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return "", "", errors.New("image data URI must start with data:")
	}
	comma := strings.IndexByte(value, ',')
	if comma <= len("data:") || comma == len(value)-1 {
		return "", "", errors.New("image data URI is invalid")
	}
	header := value[len("data:"):comma]
	parts := strings.Split(header, ";")
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[1]), "base64") {
		return "", "", errors.New("image data URI must use base64 encoding")
	}
	mediaType := normalizeSeedanceInlineImageMediaType(parts[0])
	if mediaType == "" {
		return "", "", errors.New("image data URI media type must be image/png, image/jpeg, or image/webp")
	}
	return mediaType, value[comma+1:], nil
}

func normalizeSeedanceImageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "first_frame", "start_frame", "first":
		return "first_frame"
	case "last_frame", "end_frame", "last":
		return "last_frame"
	case "reference_image", "reference", "ref":
		return "reference_image"
	default:
		return ""
	}
}

func normalizeSeedanceStrength(strength string) string {
	switch strings.ToUpper(strings.TrimSpace(strength)) {
	case "LOW", "MID", "HIGH":
		return strings.ToUpper(strings.TrimSpace(strength))
	default:
		return "MID"
	}
}

func validateSeedanceAspectRatio(ratio string) error {
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "", "adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21":
		return nil
	default:
		return errors.New("ratio must be adaptive, 16:9, 9:16, 1:1, 4:3, 3:4, 21:9, or 9:21")
	}
}

func (i *SeedanceRequestInfo) UpstreamBody(upstreamModel string) ([]byte, error) {
	if i == nil {
		return nil, errors.New("seedance request info is required")
	}
	audioEnabled := i.GenerateAudio || len(i.AudioReferences) > 0
	body := map[string]any{
		"model":      strings.TrimSpace(upstreamModel),
		"prompt":     i.Prompt,
		"resolution": i.Resolution,
		"duration":   i.DurationSeconds,
		"audio":      audioEnabled,
	}
	if ratio := strings.TrimSpace(i.AspectRatio); ratio != "" && !strings.EqualFold(ratio, "adaptive") {
		body["aspect_ratio"] = ratio
	}
	if i.PromptEnhance != nil {
		body["prompt_enhance"] = i.PromptEnhance
	}
	if len(i.References) > 0 {
		references := make([]map[string]any, 0, len(i.References))
		for order, reference := range i.References {
			if !isSeedanceHTTPImageURL(reference.URL) {
				return nil, errors.New("inline/reference image must be uploaded before forwarding")
			}
			references = append(references, map[string]any{
				"image":    map[string]any{"url": reference.URL, "type": "UPLOADED"},
				"strength": reference.Strength,
				"order":    order,
			})
		}
		body["guidances"] = map[string]any{"image_reference": references}
	} else if i.EndFrameURL != "" {
		if !isSeedanceHTTPImageURL(i.StartFrameURL) || !isSeedanceHTTPImageURL(i.EndFrameURL) {
			return nil, errors.New("inline first/last frame must be uploaded before forwarding")
		}
		body["start_frame_url"] = i.StartFrameURL
		body["end_frame_url"] = i.EndFrameURL
	} else if i.StartFrameURL != "" {
		if !isSeedanceHTTPImageURL(i.StartFrameURL) {
			return nil, errors.New("inline first-frame image must be uploaded before forwarding")
		}
		if profile, ok := ffLinkVideoModelProfileFor(i.Model); ok && (profile.Platform == PlatformLTX || profile.Platform == PlatformHappyHorse) {
			body["start_frame_url"] = i.StartFrameURL
		} else {
			body["image_url"] = i.StartFrameURL
		}
	}
	if len(i.VideoReferences) > 0 || len(i.AudioReferences) > 0 {
		guidances, _ := body["guidances"].(map[string]any)
		if guidances == nil {
			guidances = map[string]any{}
		}
		if len(i.VideoReferences) > 0 {
			references := make([]map[string]any, 0, len(i.VideoReferences))
			for _, reference := range i.VideoReferences {
				if !isSeedanceHTTPImageURL(reference.URL) {
					return nil, errors.New("inline/reference video must be uploaded before forwarding")
				}
				references = append(references, map[string]any{
					"video": map[string]any{"url": reference.URL, "type": "UPLOADED"},
				})
			}
			guidances["video_reference_base"] = references
		}
		if len(i.AudioReferences) > 0 {
			references := make([]map[string]any, 0, len(i.AudioReferences))
			for _, reference := range i.AudioReferences {
				if !isSeedanceHTTPImageURL(reference.URL) {
					return nil, errors.New("inline/reference audio must be uploaded before forwarding")
				}
				references = append(references, map[string]any{
					"audio": map[string]any{"url": reference.URL, "type": "UPLOADED"},
				})
			}
			guidances["audio_reference"] = references
		}
		body["guidances"] = guidances
	}
	return json.Marshal(body)
}

func isSeedanceHTTPImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func SeedanceTaskSessionHash(taskID string, userID, apiKeyID int64) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return "seedance-task:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d:%d:%s", userID, apiKeyID, taskID))
}

func SeedanceUsageRequestID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return "seedance:" + taskID
}

func (s *OpenAIGatewayService) BindSeedanceTaskAccount(
	ctx context.Context,
	groupID *int64,
	taskID string,
	userID, apiKeyID, accountID int64,
	model string,
) error {
	return s.BindSeedanceTaskAccountWithFallback(ctx, groupID, taskID, taskID, userID, apiKeyID, accountID, model, "", nil, "")
}

func (s *OpenAIGatewayService) BindSeedanceTaskAccountWithFallback(
	ctx context.Context,
	groupID *int64,
	publicTaskID string,
	upstreamTaskID string,
	userID, apiKeyID, accountID int64,
	model, fallbackModel string,
	requestSnapshot []byte,
	fallbackStatus string,
) error {
	group := derefGroupID(groupID)
	publicTaskID = strings.TrimSpace(publicTaskID)
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	model = strings.TrimSpace(model)
	cacheKey := ""
	if s != nil {
		cacheKey = s.openAISessionCacheKey(SeedanceTaskSessionHash(publicTaskID, userID, apiKeyID))
	}
	if s == nil || cacheKey == "" || group <= 0 || accountID <= 0 || model == "" || publicTaskID == "" {
		return errors.New("seedance task binding is invalid")
	}
	if upstreamTaskID == "" {
		upstreamTaskID = publicTaskID
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskBindingRepository)
	if !ok || repo == nil {
		return errors.New("seedance task binding repository is unavailable")
	}
	if err := repo.SaveSeedanceTaskBinding(ctx, &SeedanceTaskBinding{
		UserID:          userID,
		APIKeyID:        apiKeyID,
		GroupID:         group,
		AccountID:       accountID,
		JobID:           publicTaskID,
		UpstreamJobID:   upstreamTaskID,
		Model:           model,
		FallbackModel:   strings.TrimSpace(fallbackModel),
		FallbackStatus:  strings.TrimSpace(fallbackStatus),
		RequestSnapshot: append([]byte(nil), requestSnapshot...),
	}); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.SetSessionAccountID(ctx, group, cacheKey, accountID, seedanceTaskBindingTTL)
	}
	return nil
}

func (s *OpenAIGatewayService) GetSeedanceTaskBinding(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
) (*SeedanceTaskBinding, error) {
	group := derefGroupID(groupID)
	jobID = strings.TrimSpace(jobID)
	if s == nil || group <= 0 || jobID == "" || userID <= 0 || apiKeyID <= 0 {
		return nil, errors.New("seedance task binding is invalid")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskBindingRepository)
	if !ok || repo == nil {
		return nil, errors.New("seedance task binding repository is unavailable")
	}
	return repo.GetSeedanceTaskBinding(ctx, userID, apiKeyID, group, jobID)
}

func (s *OpenAIGatewayService) ClaimSeedanceTaskFallback(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
) (bool, string, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskFallbackRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, "", errors.New("seedance task fallback repository is unavailable")
	}
	return repo.ClaimSeedanceTaskFallback(ctx, userID, apiKeyID, group, jobID)
}

func (s *OpenAIGatewayService) ActivateSeedanceTaskFallback(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
	accountID int64,
	upstreamJobID string,
) (bool, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskFallbackRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" || userID <= 0 || apiKeyID <= 0 || accountID <= 0 {
		return false, errors.New("seedance task fallback repository is unavailable")
	}
	activated, err := repo.ActivateSeedanceTaskFallback(ctx, userID, apiKeyID, group, jobID, claimToken, accountID, upstreamJobID)
	if err == nil && activated && s.cache != nil {
		cacheKey := s.openAISessionCacheKey(SeedanceTaskSessionHash(jobID, userID, apiKeyID))
		if cacheKey != "" {
			_ = s.cache.SetSessionAccountID(ctx, group, cacheKey, accountID, seedanceTaskBindingTTL)
		}
	}
	return activated, err
}

func (s *OpenAIGatewayService) FailSeedanceTaskFallback(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (bool, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskFallbackRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, errors.New("seedance task fallback repository is unavailable")
	}
	return repo.FailSeedanceTaskFallback(ctx, userID, apiKeyID, group, jobID, claimToken)
}

func (s *OpenAIGatewayService) ReleaseSeedanceTaskFallback(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (bool, error) {
	group := derefGroupID(groupID)
	if s == nil || group <= 0 || userID <= 0 || apiKeyID <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" {
		return false, errors.New("seedance fallback release owner is invalid")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskFallbackRepository)
	if !ok || repo == nil {
		return false, errors.New("seedance fallback repository is unavailable")
	}
	return repo.ReleaseSeedanceTaskFallback(ctx, userID, apiKeyID, group, jobID, claimToken)
}

func (s *OpenAIGatewayService) RenewSeedanceTaskFallback(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (bool, error) {
	group := derefGroupID(groupID)
	if s == nil || group <= 0 || userID <= 0 || apiKeyID <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" {
		return false, errors.New("seedance fallback renewal owner is invalid")
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskFallbackRepository)
	if !ok || repo == nil {
		return false, errors.New("seedance fallback repository is unavailable")
	}
	return repo.RenewSeedanceTaskFallback(ctx, userID, apiKeyID, group, jobID, claimToken)
}

func (s *OpenAIGatewayService) ClaimSeedanceTaskCancellation(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
) (bool, string, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskCancellationRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, "", errors.New("seedance task cancellation repository is unavailable")
	}
	return repo.ClaimSeedanceTaskCancellation(ctx, userID, apiKeyID, group, jobID)
}

func (s *OpenAIGatewayService) CompleteSeedanceTaskCancellation(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (bool, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskCancellationRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, errors.New("seedance task cancellation repository is unavailable")
	}
	return repo.CompleteSeedanceTaskCancellation(ctx, userID, apiKeyID, group, jobID, claimToken)
}

func (s *OpenAIGatewayService) ReleaseSeedanceTaskCancellation(
	ctx context.Context,
	groupID *int64,
	jobID string,
	userID, apiKeyID int64,
	claimToken string,
) (bool, error) {
	group := derefGroupID(groupID)
	repo, ok := s.seedanceTaskCancellationRepository()
	if !ok || group <= 0 || strings.TrimSpace(jobID) == "" || strings.TrimSpace(claimToken) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, errors.New("seedance task cancellation repository is unavailable")
	}
	return repo.ReleaseSeedanceTaskCancellation(ctx, userID, apiKeyID, group, jobID, claimToken)
}

func (s *OpenAIGatewayService) seedanceTaskFallbackRepository() (SeedanceTaskFallbackRepository, bool) {
	if s == nil || s.usageLogRepo == nil {
		return nil, false
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskFallbackRepository)
	return repo, ok && repo != nil
}

func (s *OpenAIGatewayService) seedanceTaskCancellationRepository() (SeedanceTaskCancellationRepository, bool) {
	if s == nil || s.usageLogRepo == nil {
		return nil, false
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskCancellationRepository)
	return repo, ok && repo != nil
}

func (s *OpenAIGatewayService) ResolveSeedanceTaskAccount(ctx context.Context, groupID *int64, taskID string, userID, apiKeyID int64) (int64, error) {
	group := derefGroupID(groupID)
	taskID = strings.TrimSpace(taskID)
	cacheKey := ""
	if s != nil {
		cacheKey = s.openAISessionCacheKey(SeedanceTaskSessionHash(taskID, userID, apiKeyID))
	}
	if s == nil || cacheKey == "" || group <= 0 {
		return 0, errors.New("seedance task binding is invalid")
	}
	if s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, group, cacheKey); err == nil && accountID > 0 {
			return accountID, nil
		}
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskBindingRepository)
	if !ok || repo == nil {
		return 0, errors.New("seedance task binding repository is unavailable")
	}
	binding, err := repo.GetSeedanceTaskBinding(ctx, userID, apiKeyID, group, taskID)
	if err != nil {
		return 0, err
	}
	if binding == nil || binding.AccountID <= 0 {
		return 0, errors.New("seedance task binding is invalid")
	}
	if s.cache != nil {
		_ = s.cache.SetSessionAccountID(ctx, group, cacheKey, binding.AccountID, seedanceTaskBindingTTL)
	}
	return binding.AccountID, nil
}

func (s *OpenAIGatewayService) SeedanceTaskAccountSelection(ctx context.Context, accountID int64, groupID *int64) (*AccountSelectionResult, error) {
	selection, err := s.SeedanceBoundTaskAccountSelection(ctx, accountID, groupID)
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil || !selection.Account.IsHuiquVideo() {
		if selection != nil && selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, errors.New("seedance task account is unavailable")
	}
	return selection, nil
}

func (s *OpenAIGatewayService) SeedanceBoundTaskAccountSelection(ctx context.Context, accountID int64, groupID *int64) (*AccountSelectionResult, error) {
	group := derefGroupID(groupID)
	if s == nil || s.accountRepo == nil || accountID <= 0 || group <= 0 {
		return nil, errors.New("seedance task account is invalid")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsFFLinkVideo() || !account.IsSchedulable() || account.GetVideoProvider() == "" || !openAIStickyAccountMatchesGroup(account, &group) {
		return nil, errors.New("seedance task account is unavailable")
	}
	maxConcurrency := account.Concurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	scheduling := s.schedulingConfig()
	waitTimeout := scheduling.FallbackWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	maxWaiting := scheduling.FallbackMaxWaiting
	if maxWaiting <= 0 {
		maxWaiting = 100
	}
	selection, err := s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
		AccountID:      account.ID,
		MaxConcurrency: maxConcurrency,
		Timeout:        waitTimeout,
		MaxWaiting:     maxWaiting,
	})
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil || !selection.Account.IsFFLinkVideo() || !selection.Account.IsSchedulable() ||
		selection.Account.GetVideoProvider() == "" || !openAIStickyAccountMatchesGroup(selection.Account, &group) {
		return nil, errors.New("seedance task account is unavailable")
	}
	return selection, nil
}

func (s *OpenAIGatewayService) ListOwnedSeedanceJobs(
	ctx context.Context,
	groupID *int64,
	userID, apiKeyID int64,
	limit int,
	status string,
) ([]map[string]any, error) {
	group := derefGroupID(groupID)
	if s == nil || group <= 0 || userID <= 0 || apiKeyID <= 0 {
		return nil, errors.New("seedance task owner is invalid")
	}
	if limit <= 0 {
		limit = DefaultSeedanceJobsLimit
	}
	if limit > MaxSeedanceJobsLimit {
		limit = MaxSeedanceJobsLimit
	}
	status = strings.ToLower(strings.TrimSpace(status))
	queryLimit := limit
	if status != "" {
		queryLimit = MaxSeedanceJobsLimit
	}
	repo, ok := s.usageLogRepo.(SeedanceTaskBindingRepository)
	if !ok || repo == nil {
		return nil, errors.New("seedance task binding repository is unavailable")
	}
	bindings, err := repo.ListSeedanceTaskBindings(ctx, userID, apiKeyID, group, queryLimit)
	if err != nil {
		return nil, err
	}
	ownedBindings := bindings[:0]
	for _, binding := range bindings {
		if binding.UserID == userID && binding.APIKeyID == apiKeyID && binding.GroupID == group {
			ownedBindings = append(ownedBindings, binding)
		}
	}
	bindings = ownedBindings
	if len(bindings) == 0 {
		return []map[string]any{}, nil
	}

	jobs := make([]map[string]any, len(bindings))
	semaphore := make(chan struct{}, seedanceTaskStatusConcurrency)
	var wg sync.WaitGroup
	for i := range bindings {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
				jobs[index] = s.loadSeedanceIndexedJob(ctx, bindings[index])
			case <-ctx.Done():
				jobs[index] = seedanceIndexedJobFallback(bindings[index])
			}
		}(i)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	filtered := make([]map[string]any, 0, min(limit, len(jobs)))
	for _, job := range jobs {
		if status != "" {
			jobStatus, _ := job["status"].(string)
			if strings.ToLower(strings.TrimSpace(jobStatus)) != status {
				continue
			}
		}
		filtered = append(filtered, job)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

func (s *OpenAIGatewayService) loadSeedanceIndexedJob(ctx context.Context, binding SeedanceTaskBinding) map[string]any {
	fallback := seedanceIndexedJobFallback(binding)
	if binding.FallbackStatus == SeedanceFallbackStatusStarting {
		fallback["status"] = "queued"
		return fallback
	}
	if binding.FallbackStatus == SeedanceFallbackStatusCancelling {
		fallback["status"] = "running"
		return fallback
	}
	if binding.FallbackStatus == SeedanceFallbackStatusCancelled {
		fallback["status"] = "cancelled"
		return fallback
	}
	if s.accountRepo == nil || s.httpUpstream == nil {
		return fallback
	}
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil || account == nil {
		return fallback
	}
	if !account.IsFFLinkVideo() || account.GetVideoProvider() == "" {
		return fallback
	}
	if account.IsHuiquVideo() && (!account.IsSchedulable() || !openAIStickyAccountMatchesGroup(account, &binding.GroupID)) {
		return fallback
	}
	upstreamJobID := strings.TrimSpace(binding.UpstreamJobID)
	if upstreamJobID == "" {
		upstreamJobID = binding.JobID
	}
	forwarded, err := s.ForwardSeedance(ctx, nil, account, http.MethodGet, upstreamJobID, nil)
	if err != nil || forwarded == nil || len(forwarded.Body) == 0 {
		return fallback
	}
	normalized, err := NormalizeSeedanceJobForRoute(forwarded.Body, binding.JobID, account.GetVideoProvider(), binding.Model)
	if err != nil {
		return fallback
	}
	var job map[string]any
	if err := json.Unmarshal(normalized, &job); err != nil {
		return fallback
	}
	job["job_id"] = binding.JobID
	if strings.TrimSpace(binding.Model) != "" {
		job["model"] = PublicSeedanceModelID(binding.Model)
	}
	if value, ok := job["status"].(string); !ok || strings.TrimSpace(value) == "" {
		job["status"] = "unknown"
	}
	if _, ok := job["created_at"]; !ok && !binding.CreatedAt.IsZero() {
		job["created_at"] = binding.CreatedAt.Format(time.RFC3339)
	}
	return job
}

func seedanceIndexedJobFallback(binding SeedanceTaskBinding) map[string]any {
	job := map[string]any{
		"job_id":     strings.TrimSpace(binding.JobID),
		"model":      PublicSeedanceModelID(binding.Model),
		"status":     "unknown",
		"status_url": SeedancePublicJobsEndpoint + "/" + url.PathEscape(strings.TrimSpace(binding.JobID)),
	}
	if !binding.CreatedAt.IsZero() {
		job["created_at"] = binding.CreatedAt.Format(time.RFC3339)
	}
	return job
}

func (s *OpenAIGatewayService) ForwardSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
) (*SeedanceUpstreamResponse, error) {
	return s.forwardSeedance(ctx, c, account, method, taskID, requestInfo, nil)
}

func (s *OpenAIGatewayService) ForwardSeedanceContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	taskID string,
	rangeHeader string,
) (*SeedanceUpstreamResponse, error) {
	return s.forwardSeedance(ctx, c, account, http.MethodGet, taskID, nil, &rangeHeader)
}

func (s *OpenAIGatewayService) ForwardSeedanceJobsList(
	ctx context.Context,
	c *gin.Context,
	account *Account,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsFFLinkVideo() || account.Type != AccountTypeAPIKey {
		return nil, errors.New("FFLink video forwarding requires a compatible API key account")
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEndpointURL(baseURL, seedanceUpstreamJobsPath)
	if c != nil && c.Request != nil && strings.TrimSpace(c.Request.URL.RawQuery) != "" {
		parsedTarget, err := url.Parse(targetURL)
		if err != nil {
			return nil, fmt.Errorf("build Seedance upstream request: %w", err)
		}
		parsedTarget.RawQuery = c.Request.URL.RawQuery
		targetURL = parsedTarget.String()
	}
	SetActualOpenAIUpstreamEndpoint(c, seedanceUpstreamJobsPath)

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Seedance upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	upstreamReq.Header.Set("Accept", "application/json")
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("Seedance upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		responseBody := sanitizeSeedanceUpstreamErrorBody(s.readUpstreamErrorBody(resp))
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(responseBody))
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: []byte(message)}
	}
	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	return &SeedanceUpstreamResponse{
		StatusCode:  resp.StatusCode,
		Header:      resp.Header.Clone(),
		Body:        responseBody,
		ContentType: strings.TrimSpace(resp.Header.Get("Content-Type")),
	}, nil
}

func (s *OpenAIGatewayService) forwardSeedance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	taskID string,
	requestInfo *SeedanceRequestInfo,
	contentRangeOverride *string,
) (*SeedanceUpstreamResponse, error) {
	if account == nil || !account.IsFFLinkVideo() || account.Type != AccountTypeAPIKey {
		return nil, errors.New("FFLink video forwarding requires a compatible API key account")
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	provider := account.GetVideoProvider()
	if provider == "" {
		return nil, fmt.Errorf("account %d has an invalid video_provider", account.ID)
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	path := seedanceUpstreamCreatePath
	var requestBody []byte
	var multipartBody *seedanceHuiquMultipartBody
	requestContentType := "application/json"
	requestModel := ""
	upstreamModel := ""
	if method == http.MethodPost {
		if requestInfo == nil {
			return nil, errors.New("Seedance create request is required")
		}
		requestModel = requestInfo.Model
		var err error
		mappedModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestModel))
		upstreamModel = mappedModel
		if provider == VideoProviderHuiqu {
			upstreamModel, err = huiquUpstreamModelFor(mappedModel, requestInfo.DurationSeconds)
			if err != nil {
				return nil, err
			}
		}
		if provider == VideoProviderHuiqu {
			path = huiquVideoCreatePath
			if requestInfo.HasReferenceMedia() {
				multipartBody, err = buildHuiquMultipartBody(requestInfo, upstreamModel)
				if err == nil {
					requestContentType = multipartBody.ContentType
				}
			} else {
				requestBody, err = requestInfo.HuiquUpstreamBody(upstreamModel)
			}
		} else {
			requestBody, err = requestInfo.UpstreamBody(upstreamModel)
		}
		if err != nil {
			return nil, err
		}
	} else {
		upstreamTaskID, err := upstreamSeedanceTaskID(provider, taskID)
		if err != nil {
			return nil, err
		}
		if provider == VideoProviderHuiqu {
			if method == http.MethodDelete {
				return nil, &SeedanceUpstreamError{StatusCode: http.StatusMethodNotAllowed, Body: []byte("this video provider does not support task cancellation")}
			}
			path = huiquVideoTaskPath + "/" + url.PathEscape(upstreamTaskID)
		} else {
			path = seedanceUpstreamJobsPath + "/" + url.PathEscape(upstreamTaskID)
		}
		if c != nil && c.Request != nil && strings.HasSuffix(c.Request.URL.Path, "/content") {
			path += "/content"
		}
	}
	if multipartBody != nil {
		defer multipartBody.Close()
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEndpointURL(baseURL, path)
	SetActualOpenAIUpstreamEndpoint(c, path)

	var bodyReader io.Reader
	if multipartBody != nil {
		bodyReader = multipartBody.File
	} else if len(requestBody) > 0 {
		bodyReader = bytes.NewReader(requestBody)
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build Seedance upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	upstreamReq.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		upstreamReq.Header.Set("Content-Type", requestContentType)
		if multipartBody != nil {
			upstreamReq.ContentLength = multipartBody.SizeBytes
			upstreamReq.GetBody = multipartBody.GetBody
		}
		if provider == VideoProviderFFLink {
			upstreamReq.Header.Set("Prefer", "respond-async")
		}
		if c != nil {
			if idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key")); idempotencyKey != "" {
				upstreamReq.Header.Set("Idempotency-Key", idempotencyKey)
			}
		}
		if upstreamReq.Header.Get("Idempotency-Key") == "" {
			if idempotencyKey := seedanceIdempotencyKeyFromContext(ctx); idempotencyKey != "" {
				upstreamReq.Header.Set("Idempotency-Key", idempotencyKey)
			}
		}
	}
	if c != nil && strings.HasSuffix(path, "/content") {
		rangeHeader := strings.TrimSpace(c.GetHeader("Range"))
		if contentRangeOverride != nil {
			rangeHeader = strings.TrimSpace(*contentRangeOverride)
		}
		if rangeHeader != "" {
			upstreamReq.Header.Set("Range", rangeHeader)
		}
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		forwardErr := fmt.Errorf("Seedance upstream request failed: %s", sanitizeUpstreamErrorMessage(err.Error()))
		if method == http.MethodPost {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: forwardErr}
		}
		return nil, forwardErr
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	isContentResponse := strings.HasSuffix(path, "/content")
	if resp.StatusCode >= http.StatusBadRequest && !(isContentResponse && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		defer func() { _ = resp.Body.Close() }()
		responseBody := s.readUpstreamErrorBody(resp)
		if provider == VideoProviderHuiqu {
			responseBody = sanitizeHuiquSeedanceUpstreamErrorBody(responseBody)
		} else {
			responseBody = sanitizeSeedanceUpstreamErrorBody(responseBody)
		}
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(responseBody))
		if method == http.MethodPost && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, message, responseBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           responseBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return nil, &SeedanceUpstreamError{StatusCode: resp.StatusCode, Body: responseBody}
	}

	response := &SeedanceUpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), ContentType: contentType}
	if isContentResponse {
		response.BodyStream = resp.Body
		return response, nil
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if method == http.MethodPost {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: err}
		}
		return nil, err
	}
	response.Body = responseBody
	if method == http.MethodPost {
		upstreamTaskID := extractSeedanceUpstreamTaskID(responseBody)
		if upstreamTaskID == "" {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{
				Err: errors.New("Seedance upstream response did not include job_id"),
			}
		}
		publicTaskID, err := publicSeedanceTaskID(provider, upstreamTaskID)
		if err != nil {
			return nil, &SeedanceUpstreamAcceptanceUnknownError{Err: err}
		}
		response.Result = &OpenAIForwardResult{
			RequestID:            firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id"), "seedance:"+publicTaskID),
			ResponseID:           publicTaskID,
			Model:                requestModel,
			BillingModel:         requestModel,
			UpstreamModel:        upstreamModel,
			UpstreamEndpoint:     path,
			ResponseHeaders:      resp.Header.Clone(),
			Duration:             time.Since(startedAt),
			VideoCount:           1,
			VideoResolution:      requestInfo.Resolution,
			VideoDurationSeconds: requestInfo.DurationSeconds,
		}
	}
	return response, nil
}

func extractSeedanceUpstreamTaskID(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"job_id", "id", "task_id"} {
		if value, ok := payload[key].(string); ok && seedanceTaskIDPattern.MatchString(strings.TrimSpace(value)) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func FilterSeedanceJobsList(body []byte, allowTask func(string) bool) ([]byte, error) {
	if allowTask == nil {
		return nil, errors.New("Seedance task filter is required")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid Seedance upstream jobs response")
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return nil, errors.New("Seedance upstream jobs response does not contain a data array")
	}
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		job, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Seedance upstream jobs response contains an invalid job")
		}
		taskID := ""
		for _, key := range []string{"job_id", "id", "task_id"} {
			if value, ok := job[key].(string); ok && seedanceTaskIDPattern.MatchString(strings.TrimSpace(value)) {
				taskID = strings.TrimSpace(value)
				break
			}
		}
		if taskID != "" && allowTask(taskID) {
			normalizeSeedancePublicJob(job, taskID, IsHuiquSeedanceTaskID(taskID), "")
			filtered = append(filtered, item)
		}
	}
	payload["data"] = filtered
	return json.Marshal(payload)
}

func NormalizeSeedanceJob(body []byte, taskID string) ([]byte, error) {
	provider := VideoProviderFFLink
	if IsHuiquSeedanceTaskID(taskID) {
		provider = VideoProviderHuiqu
	}
	return NormalizeSeedanceJobForRoute(body, taskID, provider, "")
}

func NormalizeSeedanceJobForRoute(body []byte, taskID, provider, publicModel string) ([]byte, error) {
	taskID = strings.TrimSpace(taskID)
	if !seedanceTaskIDPattern.MatchString(taskID) {
		return nil, errors.New("invalid Seedance task id")
	}
	var job map[string]any
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, errors.New("invalid Seedance upstream job response")
	}
	normalizeSeedancePublicJob(job, taskID, provider == VideoProviderHuiqu, publicModel)
	return json.Marshal(job)
}

func normalizeSeedancePublicJob(job map[string]any, taskID string, isHuiquTask bool, publicModel string) {
	if job == nil || taskID == "" {
		return
	}
	statusPath := SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID)
	contentPath := statusPath + "/content"
	if isHuiquTask {
		sanitizeHuiquSeedanceResponse(job, statusPath, contentPath)
		job["id"] = taskID
		job["job_id"] = taskID
		job["task_id"] = taskID
	}
	job["status_url"] = statusPath
	if publicModel = PublicSeedanceModelID(publicModel); publicModel != "" {
		job["model"] = publicModel
	}
	synthesizeHuiquResult := func() {
		status, _ := job["status"].(string)
		if isHuiquTask && MapSeedanceTaskStatus(status) == "succeeded" {
			job["result"] = map[string]any{"data": []any{map[string]any{
				"mp4_url":   contentPath,
				"url":       contentPath,
				"local_url": contentPath,
			}}}
		}
	}
	result, ok := job["result"].(map[string]any)
	if !ok {
		synthesizeHuiquResult()
		return
	}
	files, ok := result["data"].([]any)
	if !ok || len(files) == 0 {
		synthesizeHuiquResult()
		return
	}
	rewritten := false
	for _, item := range files {
		file, ok := item.(map[string]any)
		if !ok {
			continue
		}
		file["mp4_url"] = contentPath
		file["url"] = contentPath
		file["local_url"] = contentPath
		rewritten = true
	}
	if !rewritten {
		synthesizeHuiquResult()
	}
}

func sanitizeHuiquSeedanceResponse(value any, statusPath, contentPath string) bool {
	_, keep := sanitizeHuiquSeedanceValue(value, statusPath, contentPath)
	return keep
}

func sanitizeHuiquSeedanceValue(value any, statusPath, contentPath string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isHuiquSensitiveResponseKey(key) {
				delete(typed, key)
				continue
			}
			switch huiquPublicURLKind(key) {
			case "status":
				typed[key] = statusPath
				continue
			case "content":
				if _, isString := child.(string); isString {
					typed[key] = contentPath
					continue
				}
			case "remove":
				delete(typed, key)
				continue
			}
			sanitized, keep := sanitizeHuiquSeedanceValue(child, statusPath, contentPath)
			if !keep {
				delete(typed, key)
				continue
			}
			typed[key] = sanitized
		}
		return typed, len(typed) > 0
	case []any:
		filtered := make([]any, 0, len(typed))
		for _, child := range typed {
			sanitized, keep := sanitizeHuiquSeedanceValue(child, statusPath, contentPath)
			if keep {
				filtered = append(filtered, sanitized)
			}
		}
		return filtered, len(filtered) > 0
	case string:
		return typed, !containsHuiquPrivateResponseValue(typed)
	default:
		return value, true
	}
}

func huiquPublicURLKind(key string) string {
	normalized := normalizeHuiquResponseKey(key)
	switch normalized {
	case "status_url", "statusurl", "task_url", "taskurl", "job_url", "joburl", "poll_url", "pollurl", "polling_url", "pollingurl":
		return "status"
	case "video_url", "videourl", "mp4_url", "mp4url", "url", "local_url", "localurl", "download_url", "downloadurl", "content_url", "contenturl", "playback_url", "playbackurl", "file_url", "fileurl":
		return "content"
	case "thumbnail_url", "thumbnailurl", "cover_url", "coverurl", "image_url", "imageurl", "source_url", "sourceurl", "preview_url", "previewurl", "poster_url", "posterurl", "web_url", "weburl", "share_url", "shareurl", "upload_url", "uploadurl":
		return "remove"
	default:
		if strings.HasSuffix(normalized, "_url") || strings.HasSuffix(normalized, "url") || strings.HasSuffix(normalized, "_uri") || strings.HasSuffix(normalized, "uri") {
			return "remove"
		}
		return ""
	}
}

func isHuiquSensitiveResponseKey(key string) bool {
	normalized := normalizeHuiquResponseKey(key)
	if strings.HasPrefix(normalized, "x_amz_") || strings.HasPrefix(normalized, "x_oss_") ||
		strings.HasPrefix(normalized, "x_goog_") || strings.HasPrefix(normalized, "q_") ||
		strings.HasPrefix(normalized, "xamz") || strings.HasPrefix(normalized, "xoss") || strings.HasPrefix(normalized, "xgoog") {
		return true
	}
	if strings.Contains(normalized, "signature") || strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") {
		return true
	}
	switch normalized {
	case "authorization", "apikey", "api_key", "accesskey", "access_key", "accesskeyid", "access_key_id",
		"policy", "signedheaders", "signed_headers", "ossaccesskeyid", "model", "provider", "provider_name",
		"video_provider", "provider_model", "upstream_model", "mapped_model", "model_mapping", "channel_model":
		return true
	default:
		return false
	}
}

func normalizeHuiquResponseKey(key string) string {
	key = strings.TrimSpace(key)
	key = huiquCamelCaseResponseKeyPattern.ReplaceAllString(key, `${1}_${2}`)
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "-", "_")
	return strings.ReplaceAll(key, ".", "_")
}

func containsHuiquPrivateResponseValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.HasPrefix(lower, "//") {
		return true
	}
	return seedanceSensitiveQueryParamPattern.MatchString(value) ||
		huiquInternalModelPattern.MatchString(value) || huiquProviderNamePattern.MatchString(value)
}

func BuildSeedanceOfficialTaskResponse(taskID string, upstreamBody []byte, contentURL string) (map[string]any, error) {
	provider := VideoProviderFFLink
	if IsHuiquSeedanceTaskID(taskID) {
		provider = VideoProviderHuiqu
	}
	return BuildSeedanceOfficialTaskResponseForRoute(taskID, upstreamBody, contentURL, provider, "")
}

func BuildSeedanceOfficialTaskResponseForRoute(taskID string, upstreamBody []byte, contentURL, provider, publicModel string) (map[string]any, error) {
	var upstream map[string]any
	if err := json.Unmarshal(upstreamBody, &upstream); err != nil {
		return nil, errors.New("invalid Seedance upstream task response")
	}
	isHuiquTask := provider == VideoProviderHuiqu
	if isHuiquTask {
		statusPath := SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID)
		contentPath := statusPath + "/content"
		// Normalize the complete upstream object before copying any fields into
		// the official response. This prevents an unexpected URL or credential
		// field in metadata from bypassing the small field allowlist below.
		sanitizeHuiquSeedanceResponse(upstream, statusPath, contentPath)
	}
	status, _ := upstream["status"].(string)
	officialStatus := MapSeedanceTaskStatus(status)
	response := map[string]any{"id": taskID, "status": officialStatus}
	for _, key := range []string{"model", "created_at", "updated_at", "completed_at", "seed", "resolution", "duration", "ratio"} {
		if value, exists := upstream[key]; exists && value != nil {
			response[key] = value
		}
	}
	if publicModel = PublicSeedanceModelID(publicModel); publicModel != "" {
		response["model"] = publicModel
	}
	if isHuiquTask {
		if _, exists := response["duration"]; !exists {
			if value, ok := upstream["seconds"]; ok && value != nil {
				response["duration"] = value
			}
		}
		if _, exists := response["ratio"]; !exists {
			if value, ok := upstream["aspect_ratio"]; ok && value != nil {
				response["ratio"] = value
			}
		}
	}
	if officialStatus == "succeeded" {
		response["content"] = map[string]any{"video_url": strings.TrimSpace(contentURL)}
	}
	if officialStatus == "failed" {
		if value, exists := upstream["error"]; exists {
			if isHuiquTask {
				statusPath := SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID)
				contentPath := statusPath + "/content"
				if sanitized, keep := sanitizeHuiquSeedanceValue(value, statusPath, contentPath); keep {
					response["error"] = sanitized
				}
			} else {
				response["error"] = value
			}
		} else if value, exists := upstream["error_message"]; exists {
			if isHuiquTask {
				statusPath := SeedancePublicJobsEndpoint + "/" + url.PathEscape(taskID)
				contentPath := statusPath + "/content"
				if sanitized, keep := sanitizeHuiquSeedanceValue(value, statusPath, contentPath); keep {
					response["error"] = map[string]any{"message": sanitized}
				}
			} else {
				response["error"] = map[string]any{"message": value}
			}
		}
	}
	return response, nil
}

func MapSeedanceTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued":
		return "queued"
	case "running", "processing", "settling":
		return "running"
	case "completed", "succeeded", "success":
		return "succeeded"
	case "failed", "error":
		return "failed"
	case "canceled", "cancelled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func SeedanceUpstreamErrorMessage(body []byte) string {
	message := strings.TrimSpace(extractUpstreamErrorMessage(body))
	if message == "" {
		return "Seedance upstream request failed"
	}
	return sanitizeUpstreamErrorMessage(message)
}

func sanitizeSeedanceUpstreamErrorBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	return seedanceSensitiveQueryParamPattern.ReplaceAll(body, []byte("${1}***"))
}

func sanitizeHuiquSeedanceUpstreamErrorBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err == nil {
		if sanitized, keep := sanitizeHuiquSeedanceErrorValue(payload); keep {
			if encoded, marshalErr := json.Marshal(sanitized); marshalErr == nil {
				return encoded
			}
		}
		return []byte(`{"error":{"message":"Seedance upstream request failed"}}`)
	}
	return []byte(redactHuiquSeedanceErrorText(string(body)))
}

func sanitizeHuiquSeedanceErrorValue(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isHuiquSensitiveResponseKey(key) || huiquPublicURLKind(key) != "" {
				delete(typed, key)
				continue
			}
			sanitized, keep := sanitizeHuiquSeedanceErrorValue(child)
			if !keep {
				delete(typed, key)
				continue
			}
			typed[key] = sanitized
		}
		return typed, len(typed) > 0
	case []any:
		filtered := make([]any, 0, len(typed))
		for _, child := range typed {
			if sanitized, keep := sanitizeHuiquSeedanceErrorValue(child); keep {
				filtered = append(filtered, sanitized)
			}
		}
		return filtered, len(filtered) > 0
	case string:
		sanitized := strings.TrimSpace(redactHuiquSeedanceErrorText(typed))
		return sanitized, sanitized != ""
	default:
		return value, true
	}
}

func redactHuiquSeedanceErrorText(value string) string {
	value = huiquAbsoluteURLPattern.ReplaceAllString(value, "[redacted-url]")
	value = seedanceSensitiveQueryParamPattern.ReplaceAllString(value, "${1}***")
	value = huiquSensitiveAssignmentPattern.ReplaceAllString(value, "${1}***")
	value = huiquInternalModelPattern.ReplaceAllString(value, "[upstream-model]")
	return huiquProviderNamePattern.ReplaceAllString(value, "upstream provider")
}

func writeSeedanceContentResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	writeOpenAIMediaResponseHeaders(dst, src, filter)
	if mediaType, _, err := mime.ParseMediaType(src.Get("Content-Type")); err == nil && strings.HasPrefix(mediaType, "video/") {
		dst.Set("Content-Type", mediaType)
	}
}

func (s *OpenAIGatewayService) WriteSeedanceContentResponseHeaders(dst http.Header, src http.Header) {
	if s == nil {
		return
	}
	writeSeedanceContentResponseHeaders(dst, src, s.responseHeaderFilter)
}
