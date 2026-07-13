package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const settingKeyAgentArtifactStorageConfig = "agent_artifact_storage_config"

const maxAgentArtifactStorageConfigHistory = 20

var ErrAgentArtifactStorageConfigInvalid = infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_CONFIG_INVALID", "agent artifact storage config is invalid")

type AgentArtifactStorageConfigView struct {
	Enabled                        bool   `json:"enabled"`
	Provider                       string `json:"provider"`
	AccountID                      string `json:"account_id,omitempty"`
	Endpoint                       string `json:"endpoint,omitempty"`
	Region                         string `json:"region,omitempty"`
	Bucket                         string `json:"bucket,omitempty"`
	AccessKeyID                    string `json:"access_key_id,omitempty"`
	SecretAccessKey                string `json:"secret_access_key,omitempty"`
	SecretAccessKeyConfigured      bool   `json:"secret_access_key_configured"`
	Prefix                         string `json:"prefix,omitempty"`
	PublicBaseURL                  string `json:"public_base_url,omitempty"`
	ForcePathStyle                 bool   `json:"force_path_style"`
	VirtualHostStyle               bool   `json:"virtual_host_style"`
	DisableChecksum                bool   `json:"disable_checksum"`
	MaxUploadBytes                 int64  `json:"max_upload_bytes"`
	DownloadURLTTLSeconds          int    `json:"download_url_ttl_seconds"`
	RetentionDays                  int    `json:"retention_days"`
	CleanupExpiredArtifactsEnabled bool   `json:"cleanup_expired_artifacts_enabled"`
	EncryptionKeyConfigured        bool   `json:"encryption_key_configured"`
	RuntimeError                   string `json:"runtime_error,omitempty"`
	Source                         string `json:"source,omitempty"`
	ResolvedEndpoint               string `json:"resolved_endpoint,omitempty"`
}

type agentArtifactStorageConfigRecord struct {
	Enabled                        bool   `json:"enabled"`
	Provider                       string `json:"provider"`
	AccountID                      string `json:"account_id,omitempty"`
	Endpoint                       string `json:"endpoint,omitempty"`
	Region                         string `json:"region,omitempty"`
	Bucket                         string `json:"bucket,omitempty"`
	AccessKeyID                    string `json:"access_key_id,omitempty"`
	SecretAccessKeyEncrypted       string `json:"secret_access_key_encrypted,omitempty"`
	Prefix                         string `json:"prefix,omitempty"`
	PublicBaseURL                  string `json:"public_base_url,omitempty"`
	ForcePathStyle                 bool   `json:"force_path_style"`
	VirtualHostStyle               bool   `json:"virtual_host_style"`
	DisableChecksum                bool   `json:"disable_checksum"`
	MaxUploadBytes                 int64  `json:"max_upload_bytes"`
	DownloadURLTTLSeconds          int    `json:"download_url_ttl_seconds"`
	RetentionDays                  int    `json:"retention_days"`
	CleanupExpiredArtifactsEnabled bool   `json:"cleanup_expired_artifacts_enabled"`
	UpdatedAtUnix                  int64  `json:"updated_at_unix,omitempty"`
	SecretEncryptedAtRest          bool   `json:"-"`
}

type agentArtifactStorageConfigEnvelope struct {
	Current agentArtifactStorageConfigRecord   `json:"current"`
	History []agentArtifactStorageConfigRecord `json:"history,omitempty"`
}

type AgentArtifactStorageConfigService struct {
	repo      SettingRepository
	encryptor SecretEncryptor
	cfg       *config.Config
	store     *dynamicAgentArtifactStore
}

func NewAgentArtifactStorageConfigService(repo SettingRepository, encryptor SecretEncryptor, cfg *config.Config) *AgentArtifactStorageConfigService {
	return &AgentArtifactStorageConfigService{repo: repo, encryptor: encryptor, cfg: cfg}
}

func (s *AgentArtifactStorageConfigService) SetStore(store AgentArtifactStore) {
	if dynamic, ok := store.(*dynamicAgentArtifactStore); ok {
		s.store = dynamic
	}
}

func (s *AgentArtifactStorageConfigService) GetConfig(ctx context.Context) (*AgentArtifactStorageConfigView, error) {
	record, source, err := s.loadEffectiveRecord(ctx)
	if err != nil {
		return nil, err
	}
	return s.recordToView(record, source)
}

func (s *AgentArtifactStorageConfigService) UpdateConfig(ctx context.Context, input AgentArtifactStorageConfigView) (*AgentArtifactStorageConfigView, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("AGENT_ARTIFACT_STORAGE_SETTING_REPO_UNAVAILABLE", "setting repository is unavailable")
	}
	record, err := s.recordFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	if record.Enabled {
		if err := s.testConnection(ctx, record); err != nil {
			return nil, err
		}
	}
	oldEnvelope, oldOK, err := s.loadDBEnvelope(ctx)
	if err != nil {
		return nil, err
	}
	var old *agentArtifactStorageConfigRecord
	var history []agentArtifactStorageConfigRecord
	if oldOK && oldEnvelope != nil {
		old = &oldEnvelope.Current
		history = append(history, oldEnvelope.History...)
	}
	if old != nil && old.Enabled && artifactStorageRecordLocationKey(*old) != "" && !sameAgentArtifactStorageRecordLocation(*old, *record) {
		history = appendAgentArtifactStorageHistory(history, *old)
	}
	data, err := json.Marshal(agentArtifactStorageConfigEnvelope{Current: *record, History: history})
	if err != nil {
		return nil, fmt.Errorf("marshal agent artifact storage config: %w", err)
	}
	if err := s.repo.Set(ctx, settingKeyAgentArtifactStorageConfig, string(data)); err != nil {
		return nil, err
	}
	if s.store != nil {
		if err := s.store.Reload(ctx); err != nil {
			if oldOK && oldEnvelope != nil {
				if oldData, marshalErr := json.Marshal(oldEnvelope); marshalErr == nil {
					_ = s.repo.Set(ctx, settingKeyAgentArtifactStorageConfig, string(oldData))
				}
			} else {
				_ = s.repo.Delete(ctx, settingKeyAgentArtifactStorageConfig)
			}
			return nil, fmt.Errorf("reload agent artifact storage config: %w", err)
		}
	}
	return s.recordToView(record, "database")
}

func (s *AgentArtifactStorageConfigService) ValidateConfig(ctx context.Context, input AgentArtifactStorageConfigView) (*AgentArtifactStorageConfigView, error) {
	// Connection testing is independent from the runtime enable switch. An
	// administrator must never receive a successful test for an incomplete,
	// disabled draft that did not contact object storage at all.
	input.Enabled = true
	record, err := s.recordFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	if err := s.testConnection(ctx, record); err != nil {
		return nil, err
	}
	return s.recordToView(record, "preview")
}

func (s *AgentArtifactStorageConfigService) recordFromInput(ctx context.Context, input AgentArtifactStorageConfigView) (*agentArtifactStorageConfigRecord, error) {
	old, _, _ := s.loadDBRecord(ctx)
	record := &agentArtifactStorageConfigRecord{
		Enabled:                        input.Enabled,
		Provider:                       normalizeAgentArtifactProvider(input.Provider),
		AccountID:                      strings.TrimSpace(input.AccountID),
		Endpoint:                       strings.TrimRight(strings.TrimSpace(input.Endpoint), "/"),
		Region:                         strings.TrimSpace(input.Region),
		Bucket:                         strings.TrimSpace(input.Bucket),
		AccessKeyID:                    strings.TrimSpace(input.AccessKeyID),
		Prefix:                         normalizeArtifactPrefix(input.Prefix),
		PublicBaseURL:                  strings.TrimRight(strings.TrimSpace(input.PublicBaseURL), "/"),
		ForcePathStyle:                 input.ForcePathStyle,
		VirtualHostStyle:               input.VirtualHostStyle,
		DisableChecksum:                input.DisableChecksum,
		MaxUploadBytes:                 input.MaxUploadBytes,
		DownloadURLTTLSeconds:          input.DownloadURLTTLSeconds,
		RetentionDays:                  input.RetentionDays,
		CleanupExpiredArtifactsEnabled: input.CleanupExpiredArtifactsEnabled,
		UpdatedAtUnix:                  time.Now().Unix(),
	}
	if record.Provider == "" {
		record.Provider = "s3"
	}
	if record.Region == "" {
		record.Region = "auto"
	}
	if record.MaxUploadBytes <= 0 {
		record.MaxUploadBytes = int64(512 * 1024 * 1024)
	}
	if record.DownloadURLTTLSeconds <= 0 {
		record.DownloadURLTTLSeconds = 3600
	}
	if record.Enabled && (s.cfg == nil || !s.cfg.Totp.EncryptionKeyConfigured) {
		return nil, infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_ENCRYPTION_KEY_REQUIRED", "保存对象存储凭证前，请先在 Sub2API 配置中固定 totp.encryption_key，然后重启一次服务")
	}
	if input.SecretAccessKey != "" {
		if s.encryptor == nil {
			return nil, infraerrors.InternalServer("AGENT_ARTIFACT_STORAGE_ENCRYPTOR_UNAVAILABLE", "secret encryptor is unavailable")
		}
		encrypted, err := s.encryptor.Encrypt(input.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt agent artifact storage secret: %w", err)
		}
		record.SecretAccessKeyEncrypted = encrypted
		record.SecretEncryptedAtRest = true
	} else if old != nil {
		if storageCredentialIdentityChanged(*old, *record) {
			return nil, infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_SECRET_REENTRY_REQUIRED", "切换云厂商、Endpoint 或 Access Key ID 时必须重新填写 Secret Access Key")
		}
		record.SecretAccessKeyEncrypted = old.SecretAccessKeyEncrypted
		record.SecretEncryptedAtRest = old.SecretEncryptedAtRest
	}
	if err := validateAgentArtifactStorageRecord(*record, input.SecretAccessKey != "" || record.SecretAccessKeyEncrypted != ""); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *AgentArtifactStorageConfigService) testConnection(ctx context.Context, record *agentArtifactStorageConfigRecord) error {
	cfg, err := s.recordToRuntimeConfig(record)
	if err != nil {
		return err
	}
	store := NewAgentArtifactStore(&config.Config{AgentArtifacts: cfg})
	if !store.IsConfigured() {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_CONNECTION_INVALID", "对象存储客户端创建失败，请检查配置")
	}
	content := []byte("sub2api object storage connection test")
	result, err := store.Put(ctx, AgentArtifactStorePutInput{
		Key:         fmt.Sprintf("healthchecks/config-test-%d.txt", time.Now().UnixNano()),
		ContentType: "text/plain; charset=utf-8",
		SizeBytes:   int64(len(content)),
		Body:        bytes.NewReader(content),
		Metadata:    map[string]string{"purpose": "connection-test"},
	})
	if err != nil {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_CONNECTION_FAILED", "对象存储上传测试失败："+err.Error())
	}
	location := AgentArtifactObjectLocation{StorageProvider: result.Provider, Bucket: result.Bucket, ObjectKey: result.ObjectKey}
	if _, err := store.PresignGetObject(ctx, location, 5*time.Minute); err != nil {
		_ = store.DeleteObject(context.Background(), location)
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_PRESIGN_FAILED", "对象存储下载签名测试失败："+err.Error())
	}
	if err := store.DeleteObject(ctx, location); err != nil {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_DELETE_FAILED", "对象存储删除测试失败："+err.Error())
	}
	return nil
}

func storageCredentialIdentityChanged(old, next agentArtifactStorageConfigRecord) bool {
	return normalizeAgentArtifactProvider(old.Provider) != normalizeAgentArtifactProvider(next.Provider) ||
		strings.TrimSpace(old.AccountID) != strings.TrimSpace(next.AccountID) ||
		strings.TrimRight(strings.TrimSpace(old.Endpoint), "/") != strings.TrimRight(strings.TrimSpace(next.Endpoint), "/") ||
		strings.TrimSpace(old.AccessKeyID) != strings.TrimSpace(next.AccessKeyID)
}

func (s *AgentArtifactStorageConfigService) StoreConfigForLocation(ctx context.Context, location AgentArtifactObjectLocation) (config.AgentArtifactStorageConfig, bool, error) {
	record, _, err := s.loadEffectiveRecord(ctx)
	if err != nil || record == nil || !record.Enabled {
		return config.AgentArtifactStorageConfig{}, false, err
	}
	if artifactStorageRecordMatchesLocation(*record, location) {
		cfg, err := s.recordToRuntimeConfig(record)
		return cfg, err == nil, err
	}
	history, err := s.loadDBHistory(ctx)
	if err != nil {
		return config.AgentArtifactStorageConfig{}, false, err
	}
	for i := range history {
		if !history[i].Enabled || !artifactStorageRecordMatchesLocation(history[i], location) {
			continue
		}
		cfg, err := s.recordToRuntimeConfig(&history[i])
		return cfg, err == nil, err
	}
	return config.AgentArtifactStorageConfig{}, false, nil
}

func (s *AgentArtifactStorageConfigService) CurrentRuntimeConfig(ctx context.Context) (config.AgentArtifactStorageConfig, bool, error) {
	record, _, err := s.loadEffectiveRecord(ctx)
	if err != nil || record == nil {
		return config.AgentArtifactStorageConfig{}, false, err
	}
	cfg, err := s.recordToRuntimeConfig(record)
	if err != nil {
		return config.AgentArtifactStorageConfig{}, false, err
	}
	return cfg, true, nil
}

func (s *AgentArtifactStorageConfigService) loadEffectiveRecord(ctx context.Context) (*agentArtifactStorageConfigRecord, string, error) {
	if record, ok, err := s.loadDBRecord(ctx); err != nil {
		return nil, "", err
	} else if ok {
		return record, "database", nil
	}
	if s == nil || s.cfg == nil {
		return &agentArtifactStorageConfigRecord{}, "default", nil
	}
	return recordFromConfig(s.cfg.AgentArtifacts), "config", nil
}

func (s *AgentArtifactStorageConfigService) loadDBRecord(ctx context.Context) (*agentArtifactStorageConfigRecord, bool, error) {
	envelope, ok, err := s.loadDBEnvelope(ctx)
	if err != nil || !ok {
		return nil, false, err
	}
	return &envelope.Current, true, nil
}

func (s *AgentArtifactStorageConfigService) loadDBHistory(ctx context.Context) ([]agentArtifactStorageConfigRecord, error) {
	envelope, ok, err := s.loadDBEnvelope(ctx)
	if err != nil || !ok {
		return nil, err
	}
	return envelope.History, nil
}

func (s *AgentArtifactStorageConfigService) loadDBEnvelope(ctx context.Context) (*agentArtifactStorageConfigEnvelope, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, nil
	}
	raw, err := s.repo.GetValue(ctx, settingKeyAgentArtifactStorageConfig)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	var envelope agentArtifactStorageConfigEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && (envelope.Current.Provider != "" || envelope.Current.Bucket != "" || envelope.Current.Enabled || len(envelope.History) > 0) {
		markAgentArtifactSecretsEncrypted(&envelope)
		return &envelope, true, nil
	}
	var legacy agentArtifactStorageConfigRecord
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return nil, false, ErrAgentArtifactStorageConfigInvalid
	}
	envelope = agentArtifactStorageConfigEnvelope{Current: legacy}
	markAgentArtifactSecretsEncrypted(&envelope)
	return &envelope, true, nil
}

func markAgentArtifactSecretsEncrypted(envelope *agentArtifactStorageConfigEnvelope) {
	if envelope == nil {
		return
	}
	if envelope.Current.SecretAccessKeyEncrypted != "" {
		envelope.Current.SecretEncryptedAtRest = true
	}
	for i := range envelope.History {
		if envelope.History[i].SecretAccessKeyEncrypted != "" {
			envelope.History[i].SecretEncryptedAtRest = true
		}
	}
}

func (s *AgentArtifactStorageConfigService) recordToRuntimeConfig(record *agentArtifactStorageConfigRecord) (config.AgentArtifactStorageConfig, error) {
	if record == nil {
		return config.AgentArtifactStorageConfig{}, nil
	}
	secret := record.SecretAccessKeyEncrypted
	if secret != "" && record.SecretEncryptedAtRest {
		if s == nil || s.encryptor == nil {
			return config.AgentArtifactStorageConfig{}, infraerrors.InternalServer("AGENT_ARTIFACT_STORAGE_ENCRYPTOR_UNAVAILABLE", "对象存储凭证解密器不可用")
		}
		decrypted, err := s.encryptor.Decrypt(secret)
		if err != nil {
			return config.AgentArtifactStorageConfig{}, infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_SECRET_DECRYPT_FAILED", "对象存储凭证无法解密，请确认 totp.encryption_key 未被修改并重新保存凭证")
		}
		secret = decrypted
	}
	return config.AgentArtifactStorageConfig{
		Enabled:                        record.Enabled,
		Provider:                       record.Provider,
		AccountID:                      record.AccountID,
		Endpoint:                       record.Endpoint,
		Region:                         record.Region,
		Bucket:                         record.Bucket,
		AccessKeyID:                    record.AccessKeyID,
		SecretAccessKey:                secret,
		Prefix:                         record.Prefix,
		PublicBaseURL:                  record.PublicBaseURL,
		ForcePathStyle:                 record.ForcePathStyle,
		VirtualHostStyle:               record.VirtualHostStyle,
		DisableChecksum:                record.DisableChecksum,
		MaxUploadBytes:                 record.MaxUploadBytes,
		DownloadURLTTLSeconds:          record.DownloadURLTTLSeconds,
		RetentionDays:                  record.RetentionDays,
		CleanupExpiredArtifactsEnabled: record.CleanupExpiredArtifactsEnabled,
	}, nil
}

func (s *AgentArtifactStorageConfigService) recordToView(record *agentArtifactStorageConfigRecord, source string) (*AgentArtifactStorageConfigView, error) {
	if record == nil {
		record = &agentArtifactStorageConfigRecord{}
	}
	view := &AgentArtifactStorageConfigView{
		Enabled:                        record.Enabled,
		Provider:                       firstNonEmpty(record.Provider, "s3"),
		AccountID:                      record.AccountID,
		Endpoint:                       record.Endpoint,
		Region:                         firstNonEmpty(record.Region, "auto"),
		Bucket:                         record.Bucket,
		AccessKeyID:                    record.AccessKeyID,
		SecretAccessKeyConfigured:      record.SecretAccessKeyEncrypted != "",
		Prefix:                         record.Prefix,
		PublicBaseURL:                  record.PublicBaseURL,
		ForcePathStyle:                 record.ForcePathStyle,
		VirtualHostStyle:               record.VirtualHostStyle,
		DisableChecksum:                record.DisableChecksum,
		MaxUploadBytes:                 record.MaxUploadBytes,
		DownloadURLTTLSeconds:          record.DownloadURLTTLSeconds,
		RetentionDays:                  record.RetentionDays,
		CleanupExpiredArtifactsEnabled: record.CleanupExpiredArtifactsEnabled,
		EncryptionKeyConfigured:        s != nil && s.cfg != nil && s.cfg.Totp.EncryptionKeyConfigured,
		Source:                         source,
	}
	if view.MaxUploadBytes <= 0 {
		view.MaxUploadBytes = int64(512 * 1024 * 1024)
	}
	if view.DownloadURLTTLSeconds <= 0 {
		view.DownloadURLTTLSeconds = 3600
	}
	cfg, err := s.recordToRuntimeConfig(record)
	if err != nil {
		view.RuntimeError = err.Error()
		if infraerrors.Reason(err) == "AGENT_ARTIFACT_STORAGE_SECRET_DECRYPT_FAILED" {
			view.SecretAccessKeyConfigured = false
		}
	} else {
		if resolved, err := resolveAgentArtifactStorageSettings(cfg); err == nil {
			view.ResolvedEndpoint = resolved.endpoint
		} else {
			view.RuntimeError = err.Error()
		}
	}
	return view, nil
}

func recordFromConfig(cfg config.AgentArtifactStorageConfig) *agentArtifactStorageConfigRecord {
	return &agentArtifactStorageConfigRecord{
		Enabled:                        cfg.Enabled,
		Provider:                       cfg.Provider,
		AccountID:                      cfg.AccountID,
		Endpoint:                       cfg.Endpoint,
		Region:                         cfg.Region,
		Bucket:                         cfg.Bucket,
		AccessKeyID:                    cfg.AccessKeyID,
		SecretAccessKeyEncrypted:       cfg.SecretAccessKey,
		Prefix:                         cfg.Prefix,
		PublicBaseURL:                  cfg.PublicBaseURL,
		ForcePathStyle:                 cfg.ForcePathStyle,
		VirtualHostStyle:               cfg.VirtualHostStyle,
		DisableChecksum:                cfg.DisableChecksum,
		MaxUploadBytes:                 cfg.MaxUploadBytes,
		DownloadURLTTLSeconds:          cfg.DownloadURLTTLSeconds,
		RetentionDays:                  cfg.RetentionDays,
		CleanupExpiredArtifactsEnabled: cfg.CleanupExpiredArtifactsEnabled,
	}
}

func validateAgentArtifactStorageRecord(record agentArtifactStorageConfigRecord, secretConfigured bool) error {
	if !record.Enabled {
		return nil
	}
	if strings.TrimSpace(record.Bucket) == "" {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_BUCKET_REQUIRED", "bucket is required")
	}
	if strings.TrimSpace(record.AccessKeyID) == "" {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_ACCESS_KEY_REQUIRED", "access key id is required")
	}
	if !secretConfigured {
		return infraerrors.BadRequest("AGENT_ARTIFACT_STORAGE_SECRET_REQUIRED", "secret access key is required")
	}
	cfg := config.AgentArtifactStorageConfig{
		Enabled:                        true,
		Provider:                       record.Provider,
		AccountID:                      record.AccountID,
		Endpoint:                       record.Endpoint,
		Region:                         record.Region,
		Bucket:                         record.Bucket,
		AccessKeyID:                    record.AccessKeyID,
		SecretAccessKey:                "secret",
		Prefix:                         record.Prefix,
		PublicBaseURL:                  record.PublicBaseURL,
		ForcePathStyle:                 record.ForcePathStyle,
		VirtualHostStyle:               record.VirtualHostStyle,
		DisableChecksum:                record.DisableChecksum,
		MaxUploadBytes:                 record.MaxUploadBytes,
		DownloadURLTTLSeconds:          record.DownloadURLTTLSeconds,
		RetentionDays:                  record.RetentionDays,
		CleanupExpiredArtifactsEnabled: record.CleanupExpiredArtifactsEnabled,
	}
	_, err := resolveAgentArtifactStorageSettings(cfg)
	return err
}

func appendAgentArtifactStorageHistory(history []agentArtifactStorageConfigRecord, record agentArtifactStorageConfigRecord) []agentArtifactStorageConfigRecord {
	key := artifactStorageRecordLocationKey(record)
	if key == "" {
		return history
	}
	next := make([]agentArtifactStorageConfigRecord, 0, len(history)+1)
	next = append(next, record)
	for i := range history {
		if artifactStorageRecordLocationKey(history[i]) == key {
			continue
		}
		next = append(next, history[i])
	}
	if len(next) > maxAgentArtifactStorageConfigHistory {
		next = next[:maxAgentArtifactStorageConfigHistory]
	}
	return next
}

func artifactStorageRecordMatchesLocation(record agentArtifactStorageConfigRecord, location AgentArtifactObjectLocation) bool {
	provider := normalizeAgentArtifactProvider(location.StorageProvider)
	bucket := strings.TrimSpace(location.Bucket)
	if provider != "" && provider != normalizeAgentArtifactProvider(record.Provider) {
		return false
	}
	return bucket == "" || bucket == strings.TrimSpace(record.Bucket)
}

func sameAgentArtifactStorageRecordLocation(a, b agentArtifactStorageConfigRecord) bool {
	return artifactStorageRecordLocationKey(a) == artifactStorageRecordLocationKey(b)
}

func artifactStorageRecordLocationKey(record agentArtifactStorageConfigRecord) string {
	return artifactStoreLocationKey(record.Provider, record.Bucket)
}

type dynamicAgentArtifactStore struct {
	configService *AgentArtifactStorageConfigService
	fallback      AgentArtifactStore
	current       atomic.Value
	mu            sync.Mutex
	byLocation    map[string]AgentArtifactStore
}

type agentArtifactStoreHolder struct {
	store AgentArtifactStore
}

func NewDynamicAgentArtifactStore(cfg *config.Config, configService *AgentArtifactStorageConfigService) AgentArtifactStore {
	fallback := NewAgentArtifactStore(cfg)
	store := &dynamicAgentArtifactStore{
		configService: configService,
		fallback:      fallback,
		byLocation:    map[string]AgentArtifactStore{},
	}
	store.current.Store(&agentArtifactStoreHolder{store: fallback})
	if configService != nil {
		configService.SetStore(store)
		_ = store.Reload(context.Background())
	}
	return store
}

func (s *dynamicAgentArtifactStore) Reload(ctx context.Context) error {
	if s == nil || s.configService == nil {
		return nil
	}
	cfg, ok, err := s.configService.CurrentRuntimeConfig(ctx)
	if err != nil {
		return err
	}
	if !ok {
		s.current.Store(&agentArtifactStoreHolder{store: disabledAgentArtifactStore{}})
		return nil
	}
	store := NewAgentArtifactStore(&config.Config{AgentArtifacts: cfg})
	s.current.Store(&agentArtifactStoreHolder{store: store})
	s.cacheStore(store)
	return nil
}

func (s *dynamicAgentArtifactStore) IsConfigured() bool { return s.currentStore().IsConfigured() }
func (s *dynamicAgentArtifactStore) Provider() string   { return s.currentStore().Provider() }
func (s *dynamicAgentArtifactStore) Bucket() string     { return s.currentStore().Bucket() }

func (s *dynamicAgentArtifactStore) Put(ctx context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	store := s.currentStore()
	result, err := store.Put(ctx, input)
	if err == nil {
		s.cacheStore(store)
	}
	return result, err
}

func (s *dynamicAgentArtifactStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := s.Reload(ctx); err != nil {
		return "", err
	}
	return s.currentStore().PresignGet(ctx, key, ttl)
}

func (s *dynamicAgentArtifactStore) PresignGetObject(ctx context.Context, location AgentArtifactObjectLocation, ttl time.Duration) (string, error) {
	store, err := s.storeForLocation(ctx, location)
	if err != nil {
		return "", err
	}
	return store.PresignGetObject(ctx, location, ttl)
}

func (s *dynamicAgentArtifactStore) Delete(ctx context.Context, key string) error {
	if err := s.Reload(ctx); err != nil {
		return err
	}
	return s.currentStore().Delete(ctx, key)
}

func (s *dynamicAgentArtifactStore) DeleteObject(ctx context.Context, location AgentArtifactObjectLocation) error {
	store, err := s.storeForLocation(ctx, location)
	if err != nil {
		return err
	}
	return store.DeleteObject(ctx, location)
}

func (s *dynamicAgentArtifactStore) currentStore() AgentArtifactStore {
	if s == nil {
		return disabledAgentArtifactStore{}
	}
	if holder, ok := s.current.Load().(*agentArtifactStoreHolder); ok && holder != nil && holder.store != nil {
		return holder.store
	}
	if s.fallback != nil {
		return s.fallback
	}
	return disabledAgentArtifactStore{}
}

func (s *dynamicAgentArtifactStore) storeForLocation(ctx context.Context, location AgentArtifactObjectLocation) (AgentArtifactStore, error) {
	if s == nil {
		return disabledAgentArtifactStore{}, nil
	}
	key := artifactStoreLocationKey(location.StorageProvider, location.Bucket)
	if key != "" {
		s.mu.Lock()
		store := s.byLocation[key]
		s.mu.Unlock()
		if store != nil {
			return store, nil
		}
	}
	if s.configService != nil {
		cfg, ok, err := s.configService.StoreConfigForLocation(ctx, location)
		if err != nil {
			return nil, err
		}
		if ok {
			store := NewAgentArtifactStore(&config.Config{AgentArtifacts: cfg})
			s.cacheStore(store)
			return store, nil
		}
	}
	store := s.currentStore()
	if key == "" || artifactStoreMatchesLocation(store, location) {
		return store, nil
	}
	return disabledAgentArtifactStore{}, nil
}

func (s *dynamicAgentArtifactStore) cacheStore(store AgentArtifactStore) {
	if s == nil || store == nil || !store.IsConfigured() {
		return
	}
	key := artifactStoreLocationKey(store.Provider(), store.Bucket())
	if key == "" {
		return
	}
	s.mu.Lock()
	s.byLocation[key] = store
	s.mu.Unlock()
}

func artifactStoreLocationKey(provider, bucket string) string {
	provider = normalizeAgentArtifactProvider(provider)
	bucket = strings.TrimSpace(bucket)
	if provider == "" || bucket == "" {
		return ""
	}
	return provider + ":" + bucket
}

func artifactStoreMatchesLocation(store AgentArtifactStore, location AgentArtifactObjectLocation) bool {
	if store == nil || !store.IsConfigured() {
		return false
	}
	provider := normalizeAgentArtifactProvider(location.StorageProvider)
	bucket := strings.TrimSpace(location.Bucket)
	if provider != "" && provider != normalizeAgentArtifactProvider(store.Provider()) {
		return false
	}
	return bucket == "" || bucket == store.Bucket()
}
