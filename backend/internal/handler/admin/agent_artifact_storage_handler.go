package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AgentArtifactStorageHandler struct {
	configService *service.AgentArtifactStorageConfigService
}

func NewAgentArtifactStorageHandler(configService *service.AgentArtifactStorageConfigService) *AgentArtifactStorageHandler {
	return &AgentArtifactStorageHandler{configService: configService}
}

type agentArtifactStorageRequest struct {
	Enabled                        bool   `json:"enabled"`
	Provider                       string `json:"provider"`
	AccountID                      string `json:"account_id"`
	Endpoint                       string `json:"endpoint"`
	Region                         string `json:"region"`
	Bucket                         string `json:"bucket"`
	AccessKeyID                    string `json:"access_key_id"`
	SecretAccessKey                string `json:"secret_access_key"`
	Prefix                         string `json:"prefix"`
	PublicBaseURL                  string `json:"public_base_url"`
	ForcePathStyle                 bool   `json:"force_path_style"`
	VirtualHostStyle               bool   `json:"virtual_host_style"`
	DisableChecksum                bool   `json:"disable_checksum"`
	MaxUploadBytes                 int64  `json:"max_upload_bytes"`
	DownloadURLTTLSeconds          int    `json:"download_url_ttl_seconds"`
	RetentionDays                  int    `json:"retention_days"`
	CleanupExpiredArtifactsEnabled bool   `json:"cleanup_expired_artifacts_enabled"`
}

func (h *AgentArtifactStorageHandler) GetConfig(c *gin.Context) {
	view, err := h.configService.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *AgentArtifactStorageHandler) UpdateConfig(c *gin.Context) {
	var req agentArtifactStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, err := h.configService.UpdateConfig(c.Request.Context(), service.AgentArtifactStorageConfigView{
		Enabled:                        req.Enabled,
		Provider:                       strings.TrimSpace(req.Provider),
		AccountID:                      strings.TrimSpace(req.AccountID),
		Endpoint:                       strings.TrimSpace(req.Endpoint),
		Region:                         strings.TrimSpace(req.Region),
		Bucket:                         strings.TrimSpace(req.Bucket),
		AccessKeyID:                    strings.TrimSpace(req.AccessKeyID),
		SecretAccessKey:                req.SecretAccessKey,
		Prefix:                         strings.TrimSpace(req.Prefix),
		PublicBaseURL:                  strings.TrimSpace(req.PublicBaseURL),
		ForcePathStyle:                 req.ForcePathStyle,
		VirtualHostStyle:               req.VirtualHostStyle,
		DisableChecksum:                req.DisableChecksum,
		MaxUploadBytes:                 req.MaxUploadBytes,
		DownloadURLTTLSeconds:          req.DownloadURLTTLSeconds,
		RetentionDays:                  req.RetentionDays,
		CleanupExpiredArtifactsEnabled: req.CleanupExpiredArtifactsEnabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *AgentArtifactStorageHandler) ValidateConfig(c *gin.Context) {
	var req agentArtifactStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	_, err := h.configService.ValidateConfig(c.Request.Context(), service.AgentArtifactStorageConfigView{
		Enabled:                        req.Enabled,
		Provider:                       strings.TrimSpace(req.Provider),
		AccountID:                      strings.TrimSpace(req.AccountID),
		Endpoint:                       strings.TrimSpace(req.Endpoint),
		Region:                         strings.TrimSpace(req.Region),
		Bucket:                         strings.TrimSpace(req.Bucket),
		AccessKeyID:                    strings.TrimSpace(req.AccessKeyID),
		SecretAccessKey:                req.SecretAccessKey,
		Prefix:                         strings.TrimSpace(req.Prefix),
		PublicBaseURL:                  strings.TrimSpace(req.PublicBaseURL),
		ForcePathStyle:                 req.ForcePathStyle,
		VirtualHostStyle:               req.VirtualHostStyle,
		DisableChecksum:                req.DisableChecksum,
		MaxUploadBytes:                 req.MaxUploadBytes,
		DownloadURLTTLSeconds:          req.DownloadURLTTLSeconds,
		RetentionDays:                  req.RetentionDays,
		CleanupExpiredArtifactsEnabled: req.CleanupExpiredArtifactsEnabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"valid": true})
}
