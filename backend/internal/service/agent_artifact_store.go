package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrAgentArtifactStorageNotConfigured = infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_NOT_CONFIGURED", "agent artifact storage is not configured")

type AgentS3ArtifactStore struct {
	client        *s3.Client
	provider      string
	bucket        string
	prefix        string
	publicBaseURL string
}

type agentArtifactStorageSettings struct {
	provider        string
	endpoint        string
	region          string
	bucket          string
	accessKeyID     string
	secretAccessKey string
	prefix          string
	publicBaseURL   string
	forcePathStyle  bool
	disableChecksum bool
}

func NewAgentArtifactStore(cfg *config.Config) AgentArtifactStore {
	if cfg == nil || !cfg.AgentArtifacts.Enabled {
		return disabledAgentArtifactStore{}
	}
	return newAgentArtifactStoreFromConfig(cfg.AgentArtifacts)
}

func newAgentArtifactStoreFromConfig(storageConfig config.AgentArtifactStorageConfig) AgentArtifactStore {
	storage, err := resolveAgentArtifactStorageSettings(storageConfig)
	if err != nil {
		return disabledAgentArtifactStore{err: err}
	}
	return newAgentS3ArtifactStore(storage)
}

func newAgentS3ArtifactStore(storage agentArtifactStorageSettings) AgentArtifactStore {
	if storage.bucket == "" || storage.accessKeyID == "" || storage.secretAccessKey == "" {
		return disabledAgentArtifactStore{}
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(storage.region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(storage.accessKeyID, storage.secretAccessKey, "")),
	)
	if err != nil {
		return disabledAgentArtifactStore{err: fmt.Errorf("load agent artifact storage config: %w", err)}
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if storage.endpoint != "" {
			o.BaseEndpoint = &storage.endpoint
		}
		if storage.forcePathStyle {
			o.UsePathStyle = true
		}
		if storage.disableChecksum {
			o.APIOptions = append(o.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		}
	})
	return &AgentS3ArtifactStore{
		client:        client,
		provider:      storage.provider,
		bucket:        storage.bucket,
		prefix:        normalizeArtifactPrefix(storage.prefix),
		publicBaseURL: storage.publicBaseURL,
	}
}

func (s *AgentS3ArtifactStore) IsConfigured() bool {
	return s != nil && s.client != nil && s.bucket != ""
}

func (s *AgentS3ArtifactStore) Provider() string {
	if s == nil || strings.TrimSpace(s.provider) == "" {
		return "s3"
	}
	return s.provider
}

func (s *AgentS3ArtifactStore) Bucket() string {
	if s == nil {
		return ""
	}
	return s.bucket
}

func resolveAgentArtifactStorageSettings(storage config.AgentArtifactStorageConfig) (agentArtifactStorageSettings, error) {
	provider := normalizeAgentArtifactProvider(storage.Provider)
	region := strings.TrimSpace(storage.Region)
	endpoint := strings.TrimRight(strings.TrimSpace(storage.Endpoint), "/")
	publicBaseURL := strings.TrimRight(strings.TrimSpace(storage.PublicBaseURL), "/")
	if publicBaseURL != "" {
		parsed, err := url.Parse(publicBaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return agentArtifactStorageSettings{}, infraerrors.BadRequest(
				"AGENT_ARTIFACT_STORAGE_PUBLIC_BASE_URL_INVALID",
				"public base URL must be an absolute HTTP or HTTPS URL",
			)
		}
	}
	accountID := strings.TrimSpace(storage.AccountID)
	forcePathStyle := storage.ForcePathStyle
	if storage.VirtualHostStyle {
		forcePathStyle = false
	}
	if region == "" || strings.EqualFold(region, "auto") {
		region = defaultAgentArtifactRegion(provider, endpoint)
	}
	if endpoint == "" {
		var err error
		endpoint, err = defaultAgentArtifactEndpoint(provider, region, accountID)
		if err != nil {
			return agentArtifactStorageSettings{}, err
		}
	}
	if !storage.VirtualHostStyle && defaultAgentArtifactForcePathStyle(provider) {
		forcePathStyle = true
	}
	return agentArtifactStorageSettings{
		provider:        provider,
		endpoint:        endpoint,
		region:          region,
		bucket:          strings.TrimSpace(storage.Bucket),
		accessKeyID:     strings.TrimSpace(storage.AccessKeyID),
		secretAccessKey: strings.TrimSpace(storage.SecretAccessKey),
		prefix:          normalizeArtifactPrefix(storage.Prefix),
		publicBaseURL:   publicBaseURL,
		forcePathStyle:  forcePathStyle,
		disableChecksum: storage.DisableChecksum,
	}, nil
}

func normalizeAgentArtifactProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	provider = strings.ReplaceAll(provider, "_", "-")
	switch provider {
	case "", "aws", "amazon", "amazon-s3":
		return "s3"
	case "tencent", "qcloud", "tencent-cos":
		return "cos"
	case "aliyun", "ali-oss", "aliyun-oss":
		return "oss"
	case "huawei", "huaweicloud", "huawei-obs":
		return "obs"
	case "volcengine", "volc", "volc-tos", "volcengine-tos":
		return "tos"
	case "cloudflare", "cloudflare-r2":
		return "r2"
	case "minio", "min-io":
		return "minio"
	case "wasabi":
		return "wasabi"
	case "backblaze", "backblaze-b2":
		return "b2"
	case "digitalocean", "digital-ocean", "do", "do-spaces":
		return "spaces"
	case "scaleway", "scw":
		return "scaleway"
	case "vultr":
		return "vultr"
	case "google", "gcs", "google-cloud-storage":
		return "gcs"
	case "baidu", "baidu-bos":
		return "bos"
	default:
		return provider
	}
}

func defaultAgentArtifactRegion(provider, endpoint string) string {
	if strings.TrimSpace(endpoint) != "" {
		return "auto"
	}
	switch provider {
	case "s3":
		return "us-east-1"
	default:
		return "auto"
	}
}

func defaultAgentArtifactEndpoint(provider, region, accountID string) (string, error) {
	region = strings.TrimSpace(region)
	switch provider {
	case "s3":
		return "", nil
	case "r2":
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			return "", fmt.Errorf("agent_artifacts.account_id is required when provider=r2 and endpoint is empty")
		}
		return "https://" + accountID + ".r2.cloudflarestorage.com", nil
	case "gcs":
		return "https://storage.googleapis.com", nil
	case "minio", "custom":
		return "", fmt.Errorf("agent_artifacts.endpoint is required when provider=%s", provider)
	}
	switch provider {
	case "cos", "oss", "obs", "tos", "wasabi", "b2", "spaces", "scaleway", "vultr", "bos":
	default:
		return "", fmt.Errorf("agent_artifacts.endpoint is required for unknown provider %q", provider)
	}
	if region == "" || strings.EqualFold(region, "auto") {
		return "", fmt.Errorf("agent_artifacts.region is required when provider=%s and endpoint is empty", provider)
	}
	switch provider {
	case "cos":
		return fmt.Sprintf("https://cos.%s.myqcloud.com", region), nil
	case "oss":
		return fmt.Sprintf("https://oss-%s.aliyuncs.com", region), nil
	case "obs":
		return fmt.Sprintf("https://obs.%s.myhuaweicloud.com", region), nil
	case "tos":
		return fmt.Sprintf("https://tos-s3-%s.volces.com", region), nil
	case "wasabi":
		return fmt.Sprintf("https://s3.%s.wasabisys.com", region), nil
	case "b2":
		return fmt.Sprintf("https://s3.%s.backblazeb2.com", region), nil
	case "spaces":
		return fmt.Sprintf("https://%s.digitaloceanspaces.com", region), nil
	case "scaleway":
		return fmt.Sprintf("https://s3.%s.scw.cloud", region), nil
	case "vultr":
		return fmt.Sprintf("https://%s.vultrobjects.com", region), nil
	case "bos":
		return fmt.Sprintf("https://s3.%s.bcebos.com", region), nil
	default:
		return "", fmt.Errorf("agent_artifacts.endpoint is required for unknown provider %q", provider)
	}
}

func defaultAgentArtifactForcePathStyle(provider string) bool {
	switch provider {
	case "r2", "minio", "gcs":
		return true
	default:
		return false
	}
}

func (s *AgentS3ArtifactStore) Put(ctx context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	if !s.IsConfigured() {
		return nil, ErrAgentArtifactStorageNotConfigured
	}
	key := s.fullKey(input.Key)
	if key == "" {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_OBJECT_KEY_INVALID", "artifact object key is invalid")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	put := &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        input.Body,
		ContentType: &contentType,
		Metadata:    input.Metadata,
	}
	if input.SizeBytes > 0 {
		put.ContentLength = &input.SizeBytes
	}
	if _, err := s.client.PutObject(ctx, put); err != nil {
		return nil, fmt.Errorf("put agent artifact: %w", err)
	}
	return &AgentArtifactStorePutResult{
		Provider:  s.Provider(),
		Bucket:    s.bucket,
		ObjectKey: key,
		ObjectURL: s.objectURL(key),
		SizeBytes: input.SizeBytes,
	}, nil
}

func (s *AgentS3ArtifactStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetObject(ctx, AgentArtifactObjectLocation{ObjectKey: key}, ttl)
}

func (s *AgentS3ArtifactStore) PresignGetObject(ctx context.Context, location AgentArtifactObjectLocation, ttl time.Duration) (string, error) {
	if !s.IsConfigured() {
		return "", ErrAgentArtifactStorageNotConfigured
	}
	key := strings.TrimLeft(strings.TrimSpace(location.ObjectKey), "/")
	if key == "" {
		return "", infraerrors.BadRequest("AGENT_ARTIFACT_OBJECT_KEY_INVALID", "artifact object key is invalid")
	}
	bucket := strings.TrimSpace(location.Bucket)
	if bucket == "" {
		bucket = s.bucket
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	presigner := s3.NewPresignClient(s.client)
	result, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign agent artifact: %w", err)
	}
	return result.URL, nil
}

func (s *AgentS3ArtifactStore) Delete(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, AgentArtifactObjectLocation{ObjectKey: key})
}

func (s *AgentS3ArtifactStore) DeleteObject(ctx context.Context, location AgentArtifactObjectLocation) error {
	if !s.IsConfigured() {
		return ErrAgentArtifactStorageNotConfigured
	}
	key := strings.TrimLeft(strings.TrimSpace(location.ObjectKey), "/")
	if key == "" {
		return infraerrors.BadRequest("AGENT_ARTIFACT_OBJECT_KEY_INVALID", "artifact object key is invalid")
	}
	bucket := strings.TrimSpace(location.Bucket)
	if bucket == "" {
		bucket = s.bucket
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("delete agent artifact: %w", err)
	}
	return nil
}

func (s *AgentS3ArtifactStore) fullKey(key string) string {
	key = sanitizeArtifactObjectKey(key)
	if key == "" {
		return ""
	}
	if s.prefix == "" {
		return key
	}
	return strings.TrimLeft(path.Join(s.prefix, key), "/")
}

func (s *AgentS3ArtifactStore) objectURL(key string) string {
	if s.publicBaseURL != "" {
		escaped := strings.Split(key, "/")
		for i := range escaped {
			escaped[i] = url.PathEscape(escaped[i])
		}
		return s.publicBaseURL + "/" + strings.Join(escaped, "/")
	}
	return fmt.Sprintf("%s://%s/%s", s.Provider(), s.bucket, key)
}

type disabledAgentArtifactStore struct {
	err error
}

func (s disabledAgentArtifactStore) IsConfigured() bool { return false }
func (s disabledAgentArtifactStore) Provider() string   { return "external" }
func (s disabledAgentArtifactStore) Bucket() string     { return "" }

func (s disabledAgentArtifactStore) Put(context.Context, AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return nil, ErrAgentArtifactStorageNotConfigured
}

func (s disabledAgentArtifactStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetObject(ctx, AgentArtifactObjectLocation{ObjectKey: key}, ttl)
}

func (s disabledAgentArtifactStore) PresignGetObject(context.Context, AgentArtifactObjectLocation, time.Duration) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return "", ErrAgentArtifactStorageNotConfigured
}

func (s disabledAgentArtifactStore) Delete(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, AgentArtifactObjectLocation{ObjectKey: key})
}

func (s disabledAgentArtifactStore) DeleteObject(context.Context, AgentArtifactObjectLocation) error {
	if s.err != nil {
		return s.err
	}
	return ErrAgentArtifactStorageNotConfigured
}

func normalizeArtifactPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "." {
		return ""
	}
	return sanitizeArtifactObjectKey(prefix)
}

func sanitizeArtifactObjectKey(key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	key = strings.ReplaceAll(key, "\\", "/")
	parts := strings.Split(key, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		clean = append(clean, part)
	}
	return strings.Join(clean, "/")
}

func isArtifactStorageNotConfigured(err error) bool {
	return errors.Is(err, ErrAgentArtifactStorageNotConfigured) || infraerrors.Reason(err) == "AGENT_ARTIFACT_STORAGE_NOT_CONFIGURED"
}

func limitedArtifactReader(body io.Reader, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		return body
	}
	return io.LimitReader(body, maxBytes+1)
}
