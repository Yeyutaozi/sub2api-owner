package service

import (
	"context"
	"io"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	AgentWorkerProtocolV1 = "sub2api-worker-v1"

	AgentWorkerHostStatusActive    = "active"
	AgentWorkerHostStatusDisabled  = "disabled"
	AgentWorkerHostStatusUnhealthy = "unhealthy"

	AgentWorkerHostHealthHealthy   = "healthy"
	AgentWorkerHostHealthUnhealthy = "unhealthy"
	AgentWorkerHostHealthUnknown   = "unknown"

	AgentWorkerAuthNone         = "none"
	AgentWorkerAuthHMACRunToken = "hmac_run_token"
	AgentWorkerAuthBearer       = "bearer"

	AgentAppTypePrompt   = "prompt"
	AgentAppTypeWorkflow = "workflow"
	AgentAppTypeAgent    = "agent"
	AgentAppTypeExternal = "external"

	AgentAppVisibilityPublic  = "public"
	AgentAppVisibilityPrivate = "private"

	AgentAppStatusDraft     = "draft"
	AgentAppStatusPublished = "published"
	AgentAppStatusDisabled  = "disabled"
	AgentAppStatusArchived  = "archived"

	AgentRuntimeTypeWorker   = "worker"
	AgentRuntimeTypePrompt   = "prompt"
	AgentRuntimeTypeInternal = "internal"

	AgentRunStatusQueued    = "queued"
	AgentRunStatusRunning   = "running"
	AgentRunStatusSucceeded = "succeeded"
	AgentRunStatusFailed    = "failed"
	AgentRunStatusCanceled  = "canceled"
	AgentRunStatusTimeout   = "timeout"

	AgentRunEventQueued         = "queued"
	AgentRunEventDispatching    = "dispatching"
	AgentRunEventRunning        = "running"
	AgentRunEventWorkerAccepted = "worker_accepted"
	AgentRunEventProgress       = "progress"
	AgentRunEventLog            = "log"
	AgentRunEventModelProxy     = "model_proxy"
	AgentRunEventArtifact       = "artifact"
	AgentRunEventSucceeded      = "succeeded"
	AgentRunEventFailed         = "failed"
	AgentRunEventCanceled       = "canceled"
	AgentRunEventTimeout        = "timeout"

	AgentArtifactTypeInput   = "input"
	AgentArtifactTypeOutput  = "output"
	AgentArtifactTypeLog     = "log"
	AgentArtifactTypePreview = "preview"

	AgentInputAssetTypeImage = "image"
	AgentInputAssetTypeFile  = "file"
	AgentInputAssetTypeAudio = "audio"
	AgentInputAssetTypeVideo = "video"
)

var (
	ErrAgentWorkerHostNotFound = infraerrors.NotFound("AGENT_WORKER_HOST_NOT_FOUND", "agent worker host not found")
	ErrAgentWorkerHostExists   = infraerrors.Conflict("AGENT_WORKER_HOST_EXISTS", "agent worker host already exists")
	ErrAgentAppNotFound        = infraerrors.NotFound("AGENT_APP_NOT_FOUND", "agent app not found")
	ErrAgentAppExists          = infraerrors.Conflict("AGENT_APP_EXISTS", "agent app already exists")
	ErrAgentAppVersionNotFound = infraerrors.NotFound("AGENT_APP_VERSION_NOT_FOUND", "agent app version not found")
	ErrAgentAppVersionExists   = infraerrors.Conflict("AGENT_APP_VERSION_EXISTS", "agent app version already exists")
	ErrAgentRunNotFound        = infraerrors.NotFound("AGENT_RUN_NOT_FOUND", "agent run not found")
	ErrAgentRunTokenInvalid    = infraerrors.Unauthorized("AGENT_RUN_TOKEN_INVALID", "agent run token invalid")
	ErrAgentArtifactNotFound   = infraerrors.NotFound("AGENT_ARTIFACT_NOT_FOUND", "agent artifact not found")
	ErrAgentInputAssetNotFound = infraerrors.NotFound("AGENT_INPUT_ASSET_NOT_FOUND", "agent input asset not found")
)

type AgentWorkerHost struct {
	ID                  int64
	Name                string
	BaseURL             string
	Protocol            string
	AuthType            string
	SecretRef           string
	HealthPath          string
	RunPath             string
	CancelPath          string
	MaxConcurrency      int
	TimeoutSeconds      int
	Status              string
	LastHealthStatus    string
	LastHealthMessage   string
	LastHealthLatencyMS *int
	LastCheckedAt       *time.Time
	Metadata            map[string]any
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

type AgentWorkerHostListFilters struct {
	Status    string
	Search    string
	SortBy    string
	SortOrder string
}

type AgentWorkerHostHealthResult struct {
	Success    bool
	Status     string
	StatusCode int
	LatencyMS  int
	Message    string
	CheckedAt  time.Time
	Response   *WorkerHealthResponse
	Host       *AgentWorkerHost
}

type AgentWorkerHostRepository interface {
	Create(ctx context.Context, host *AgentWorkerHost) error
	GetByID(ctx context.Context, id int64) (*AgentWorkerHost, error)
	List(ctx context.Context, params pagination.PaginationParams, filters AgentWorkerHostListFilters) ([]AgentWorkerHost, *pagination.PaginationResult, error)
	ListAll(ctx context.Context, status string) ([]AgentWorkerHost, error)
	Update(ctx context.Context, host *AgentWorkerHost) error
	UpdateHealth(ctx context.Context, id int64, status, healthStatus, message string, latencyMS *int, checkedAt time.Time) error
	Delete(ctx context.Context, id int64) error
}

type AgentApp struct {
	ID                 int64
	Name               string
	Slug               string
	Description        string
	IconURL            string
	Category           string
	AppType            string
	Visibility         string
	Status             string
	PublishedVersionID *int64
	CreatedBy          *int64
	UpdatedBy          *int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type AgentAppVersion struct {
	ID                     int64
	AppID                  int64
	Version                string
	Status                 string
	RuntimeType            string
	WorkerHostID           *int64
	WorkerRoute            string
	WorkerHealthRoute      string
	ImageRef               string
	SourceRef              string
	InputSchemaJSON        map[string]any
	OutputSchemaJSON       map[string]any
	CapabilitiesJSON       map[string]any
	DefaultModelConfigJSON map[string]any
	NodeModelPolicyJSON    map[string]any
	ArtifactPolicyJSON     map[string]any
	Changelog              string
	CreatedBy              *int64
	PublishedAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	DeletedAt              *time.Time
	WorkerHost             *AgentWorkerHost
}

type AgentAppListFilters struct {
	Status                  string
	AppType                 string
	Visibility              string
	RequirePublishedVersion bool
	Search                  string
	SortBy                  string
	SortOrder               string
}

type AgentAppRepository interface {
	CreateApp(ctx context.Context, app *AgentApp) error
	CreateAppWithVersion(ctx context.Context, app *AgentApp, version *AgentAppVersion) error
	UpdateApp(ctx context.Context, app *AgentApp) error
	DeleteApp(ctx context.Context, id int64, updatedBy *int64) error
	GetAppByID(ctx context.Context, id int64) (*AgentApp, error)
	ListApps(ctx context.Context, params pagination.PaginationParams, filters AgentAppListFilters) ([]AgentApp, *pagination.PaginationResult, error)
	CreateVersion(ctx context.Context, version *AgentAppVersion) error
	GetVersionByID(ctx context.Context, id int64) (*AgentAppVersion, error)
	GetPublishedVersionForApp(ctx context.Context, appID, versionID int64) (*AgentApp, *AgentAppVersion, error)
	ListVersions(ctx context.Context, appID int64) ([]AgentAppVersion, error)
	PublishVersion(ctx context.Context, appID, versionID int64, updatedBy *int64) error
	SetVersionStatus(ctx context.Context, appID, versionID int64, status string, updatedBy *int64) error
}

type AgentRun struct {
	ID                int64
	AppID             int64
	AppVersionID      int64
	UserID            int64
	APIKeyID          int64
	WorkerHostID      *int64
	RunTokenHash      string
	Status            string
	InputRefURL       string
	InputSummaryJSON  map[string]any
	OutputRefURL      string
	OutputSummaryJSON map[string]any
	ErrorCode         string
	ErrorMessage      string
	UsageJSON         map[string]any
	StartedAt         *time.Time
	CompletedAt       *time.Time
	ExpiresAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type AgentRunKeyBinding struct {
	ID           int64
	RunID        int64
	UserID       int64
	APIKeyID     int64
	PolicyKey    string
	NodeID       string
	Role         string
	ModelGroupID *int64
	Capability   string
	IsDefault    bool
	CreatedAt    time.Time
}

type CreateAgentRunKeyBindingInput struct {
	PolicyKey string `json:"policy_key"`
	NodeID    string `json:"node_id,omitempty"`
	Role      string `json:"role,omitempty"`
	APIKeyID  int64  `json:"api_key_id"`
}

type AgentRunListFilters struct {
	AppID     *int64
	Status    string
	Search    string
	SortBy    string
	SortOrder string
}

type AgentRunEvent struct {
	ID           int64
	RunID        int64
	UserID       int64
	EventType    string
	Status       string
	NodeID       string
	Role         string
	Message      string
	Progress     *float64
	MetadataJSON map[string]any
	CreatedAt    time.Time
}

type AgentCleanupObjectRef struct {
	ID              int64
	StorageProvider string
	Bucket          string
	ObjectKey       string
	ObjectURL       string
}

type AgentRunCleanupResult struct {
	ArtifactsDeleted   int64
	InputAssetsDeleted int64
	ObjectsDeleted     int64
	ObjectDeleteErrors int64
}

type AgentInputAssetListFilters struct {
	AppID     *int64
	AssetType string
	Search    string
	SortBy    string
	SortOrder string
}

type AgentRunRepository interface {
	CreateRun(ctx context.Context, run *AgentRun) error
	CreateRunWithKeyBindings(ctx context.Context, run *AgentRun, bindings []AgentRunKeyBinding) error
	GetRunByID(ctx context.Context, id int64) (*AgentRun, error)
	GetRunByIDForUser(ctx context.Context, id, userID int64) (*AgentRun, error)
	ListRunsByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters AgentRunListFilters) ([]AgentRun, *pagination.PaginationResult, error)
	ListRunKeyBindings(ctx context.Context, runID int64) ([]AgentRunKeyBinding, error)
	CreateInputAsset(ctx context.Context, asset *AgentInputAsset) error
	ListInputAssetsByUser(ctx context.Context, userID int64, params pagination.PaginationParams, filters AgentInputAssetListFilters) ([]AgentInputAsset, *pagination.PaginationResult, error)
	ListInputAssetsByIDsForUser(ctx context.Context, userID int64, ids []int64) ([]AgentInputAsset, error)
	GetInputAssetByID(ctx context.Context, assetID int64) (*AgentInputAsset, error)
	MarkRunning(ctx context.Context, id int64, startedAt time.Time) error
	MarkFailed(ctx context.Context, id int64, code, message string, completedAt time.Time) error
	MarkTimeout(ctx context.Context, id int64, completedAt time.Time) error
	MarkCanceled(ctx context.Context, id, userID int64, code, message string, completedAt time.Time) error
	UpdateFromCallback(ctx context.Context, run *AgentRun) error
	CreateRunEvent(ctx context.Context, event *AgentRunEvent) error
	ListRunEventsByRunForUser(ctx context.Context, runID, userID int64, params pagination.PaginationParams) ([]AgentRunEvent, *pagination.PaginationResult, error)
	CreateArtifact(ctx context.Context, artifact *AgentArtifact) error
	ListArtifactsByRun(ctx context.Context, runID int64) ([]AgentArtifact, error)
	GetArtifactByID(ctx context.Context, artifactID int64) (*AgentArtifact, error)
	ListExpiredArtifacts(ctx context.Context, now time.Time, limit int) ([]AgentCleanupObjectRef, error)
	MarkArtifactsDeleted(ctx context.Context, ids []int64, deletedAt time.Time) (int64, error)
	ListExpiredInputAssets(ctx context.Context, now time.Time, limit int) ([]AgentCleanupObjectRef, error)
	MarkInputAssetsDeleted(ctx context.Context, ids []int64, deletedAt time.Time) (int64, error)
}

type AgentArtifact struct {
	ID              int64
	RunID           int64
	UserID          int64
	ArtifactType    string
	Name            string
	MimeType        string
	StorageProvider string
	Bucket          string
	ObjectKey       string
	ObjectURL       string
	SizeBytes       int64
	SHA256          string
	MetadataJSON    map[string]any
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

type AgentInputAsset struct {
	ID              int64
	RunID           *int64
	UserID          int64
	AppID           *int64
	FieldName       string
	AssetType       string
	AssetRole       string
	Name            string
	MimeType        string
	StorageProvider string
	Bucket          string
	ObjectKey       string
	ObjectURL       string
	SizeBytes       int64
	SHA256          string
	MetadataJSON    map[string]any
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

type WorkerHealthResponse struct {
	Status         string         `json:"status"`
	Protocol       string         `json:"protocol,omitempty"`
	Version        string         `json:"version,omitempty"`
	Message        string         `json:"message,omitempty"`
	Capabilities   []string       `json:"capabilities,omitempty"`
	Routes         map[string]any `json:"routes,omitempty"`
	MaxConcurrency int            `json:"max_concurrency,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type WorkerRunRequest struct {
	RunID           int64                  `json:"run_id"`
	AppID           int64                  `json:"app_id"`
	AppVersionID    int64                  `json:"app_version_id"`
	RunToken        string                 `json:"run_token"`
	CallbackURL     string                 `json:"callback_url"`
	ModelProxyURL   string                 `json:"model_proxy_url"`
	ArtifactURL     string                 `json:"artifact_url"`
	TimeoutSeconds  int                    `json:"timeout_seconds"`
	User            WorkerRunUserContext   `json:"user"`
	Input           map[string]any         `json:"input"`
	InputArtifacts  []WorkerArtifactRef    `json:"input_artifacts,omitempty"`
	InputAssets     []WorkerArtifactRef    `json:"input_assets,omitempty"`
	NodeModelPolicy map[string]ModelPolicy `json:"node_model_policy,omitempty"`
	Metadata        map[string]any         `json:"metadata,omitempty"`
}

type WorkerRunUserContext struct {
	UserID   int64  `json:"user_id"`
	APIKeyID int64  `json:"api_key_id"`
	GroupID  *int64 `json:"group_id,omitempty"`
}

type ModelPolicy struct {
	NodeID       string `json:"node_id,omitempty"`
	Role         string `json:"role,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Model        string `json:"model,omitempty"`
	ModelGroupID *int64 `json:"model_group_id,omitempty"`
	Capability   string `json:"capability,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Optional     bool   `json:"optional,omitempty"`
}

type WorkerRunResponse struct {
	Accepted      bool           `json:"accepted"`
	WorkerRunID   string         `json:"worker_run_id,omitempty"`
	Status        string         `json:"status,omitempty"`
	Message       string         `json:"message,omitempty"`
	PollURL       string         `json:"poll_url,omitempty"`
	EstimatedTime int            `json:"estimated_time_seconds,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type WorkerCancelRequest struct {
	RunID    int64  `json:"run_id"`
	RunToken string `json:"run_token"`
	Reason   string `json:"reason,omitempty"`
}

type WorkerCancelResponse struct {
	Accepted bool           `json:"accepted"`
	Status   string         `json:"status,omitempty"`
	Message  string         `json:"message,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type WorkerCallbackRequest struct {
	RunID     int64               `json:"run_id"`
	RunToken  string              `json:"run_token"`
	EventType string              `json:"event_type"`
	Status    string              `json:"status,omitempty"`
	NodeID    string              `json:"node_id,omitempty"`
	Role      string              `json:"role,omitempty"`
	Progress  *float64            `json:"progress,omitempty"`
	Message   string              `json:"message,omitempty"`
	Output    map[string]any      `json:"output,omitempty"`
	Artifacts []WorkerArtifactRef `json:"artifacts,omitempty"`
	Error     *WorkerRunError     `json:"error,omitempty"`
	Metadata  map[string]any      `json:"metadata,omitempty"`
}

type WorkerRunError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type WorkerArtifactRef struct {
	ArtifactID int64          `json:"artifact_id,omitempty"`
	Type       string         `json:"type"`
	Name       string         `json:"name,omitempty"`
	MimeType   string         `json:"mime_type,omitempty"`
	URL        string         `json:"url,omitempty"`
	ObjectKey  string         `json:"object_key,omitempty"`
	SizeBytes  int64          `json:"size_bytes,omitempty"`
	SHA256     string         `json:"sha256,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ModelProxyRequest struct {
	RunID       int64                          `json:"run_id"`
	NodeID      string                         `json:"node_id,omitempty"`
	Role        string                         `json:"role,omitempty"`
	Model       string                         `json:"model"`
	GroupID     *int64                         `json:"group_id,omitempty"`
	Platform    string                         `json:"platform,omitempty"`
	Endpoint    string                         `json:"endpoint"`
	Method      string                         `json:"method,omitempty"`
	ContentType string                         `json:"content_type,omitempty"`
	BodyBase64  string                         `json:"body_base64,omitempty"`
	Multipart   []ModelProxyMultipartReference `json:"multipart,omitempty"`
	Request     map[string]any                 `json:"request,omitempty"`
	Metadata    map[string]any                 `json:"metadata,omitempty"`
}

type ModelProxyMultipartReference struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	BodyBase64  string `json:"body_base64,omitempty"`
}

type ModelProxyResponse struct {
	Response    map[string]any    `json:"response"`
	Usage       map[string]any    `json:"usage,omitempty"`
	Status      int               `json:"status,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	BodyBase64  string            `json:"body_base64,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

type ModelProxyGatewayCaller interface {
	CallModelProxy(ctx context.Context, req ModelProxyRequest, apiKey *APIKey) (*ModelProxyResponse, error)
}

type AgentRunAPIKeyService interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
	CheckAPIKeyQuotaAndExpiry(apiKey *APIKey) error
}

type ArtifactCreateRequest struct {
	RunID           int64          `json:"run_id"`
	RunToken        string         `json:"run_token"`
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	MimeType        string         `json:"mime_type,omitempty"`
	SizeBytes       int64          `json:"size_bytes,omitempty"`
	SHA256          string         `json:"sha256,omitempty"`
	StorageProvider string         `json:"storage_provider,omitempty"`
	ObjectKey       string         `json:"object_key,omitempty"`
	ObjectURL       string         `json:"object_url,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type ArtifactCreateResponse struct {
	ArtifactID      int64          `json:"artifact_id"`
	URL             string         `json:"url"`
	DownloadURL     string         `json:"download_url,omitempty"`
	ObjectKey       string         `json:"object_key,omitempty"`
	StorageProvider string         `json:"storage_provider,omitempty"`
	Bucket          string         `json:"bucket,omitempty"`
	SizeBytes       int64          `json:"size_bytes,omitempty"`
	SHA256          string         `json:"sha256,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type ArtifactUploadInput struct {
	RunID     int64
	RunToken  string
	Type      string
	Name      string
	MimeType  string
	SizeBytes int64
	SHA256    string
	Metadata  map[string]any
	Body      io.Reader
}

type InputAssetUploadInput struct {
	UserID    int64
	AppID     *int64
	FieldName string
	AssetType string
	AssetRole string
	Name      string
	MimeType  string
	SizeBytes int64
	SHA256    string
	Metadata  map[string]any
	Body      io.Reader
}

type ArtifactDownloadURL struct {
	ArtifactID int64  `json:"artifact_id"`
	URL        string `json:"url"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

type InputAssetDownloadURL struct {
	InputAssetID int64  `json:"input_asset_id"`
	URL          string `json:"url"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type AgentAppIconURL struct {
	AppID     int64  `json:"app_id"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type AgentArtifactStorePutInput struct {
	Key         string
	Body        io.Reader
	ContentType string
	SizeBytes   int64
	Metadata    map[string]string
}

type AgentArtifactStorePutResult struct {
	Provider  string
	Bucket    string
	ObjectKey string
	ObjectURL string
	SizeBytes int64
}

type AgentArtifactObjectLocation struct {
	StorageProvider string
	Bucket          string
	ObjectKey       string
}

type AgentArtifactStore interface {
	IsConfigured() bool
	Put(ctx context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGetObject(ctx context.Context, location AgentArtifactObjectLocation, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
	DeleteObject(ctx context.Context, location AgentArtifactObjectLocation) error
	Provider() string
	Bucket() string
}
