package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CreazyCanvasHandler struct {
	svc *service.CreazyCanvasService
}

func NewCreazyCanvasHandler(svc *service.CreazyCanvasService) *CreazyCanvasHandler {
	return &CreazyCanvasHandler{svc: svc}
}

type createCreazyCanvasWorkRequest struct {
	APIKeyID        int64          `json:"api_key_id" binding:"required"`
	Kind            string         `json:"kind" binding:"required"`
	PublicModel     string         `json:"public_model"`
	Prompt          string         `json:"prompt"`
	Params          map[string]any `json:"params"`
	GatewayType     string         `json:"gateway_type"`
	GatewayRemoteID string         `json:"gateway_remote_id"`
	Status          string         `json:"status"`
	ErrorMessage    string         `json:"error_message"`
	PreviewURL      string         `json:"preview_url"`
	ObjectURL       string         `json:"object_url"`
	MimeType        string         `json:"mime_type"`
	SizeBytes       int64          `json:"size_bytes"`
}

type createCreazyCanvasDocumentRequest struct {
	Name  string         `json:"name"`
	Graph map[string]any `json:"graph"`
}

type updateCreazyCanvasDocumentRequest struct {
	Name             *string        `json:"name"`
	Graph            map[string]any `json:"graph"`
	ExpectedRevision int64          `json:"expected_revision"`
}

type updateCreazyCanvasWorkRequest struct {
	Status          *string        `json:"status"`
	ErrorMessage    *string        `json:"error_message"`
	Params          map[string]any `json:"params"`
	GatewayType     *string        `json:"gateway_type"`
	GatewayRemoteID *string        `json:"gateway_remote_id"`
	PreviewURL      *string        `json:"preview_url"`
	ObjectURL       *string        `json:"object_url"`
	MimeType        *string        `json:"mime_type"`
	SizeBytes       *int64         `json:"size_bytes"`
	PublicModel     *string        `json:"public_model"`
	Prompt          *string        `json:"prompt"`
}

type creazyCanvasWorkResponse struct {
	ID              int64          `json:"id"`
	UserID          int64          `json:"user_id"`
	APIKeyID        int64          `json:"api_key_id"`
	GroupID         *int64         `json:"group_id,omitempty"`
	Kind            string         `json:"kind"`
	PublicModel     string         `json:"public_model"`
	Status          string         `json:"status"`
	Prompt          string         `json:"prompt"`
	Params          map[string]any `json:"params"`
	GatewayType     string         `json:"gateway_type,omitempty"`
	GatewayRemoteID string         `json:"gateway_remote_id,omitempty"`
	ObjectKey       string         `json:"object_key,omitempty"`
	StorageProvider string         `json:"storage_provider,omitempty"`
	Bucket          string         `json:"bucket,omitempty"`
	ObjectURL       string         `json:"object_url,omitempty"`
	PreviewURL      string         `json:"preview_url,omitempty"`
	MimeType        string         `json:"mime_type,omitempty"`
	SizeBytes       int64          `json:"size_bytes"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	ExpiresAt       string         `json:"expires_at"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

type creazyCanvasDocumentResponse struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Graph     map[string]any `json:"graph,omitempty"`
	Revision  int64          `json:"revision"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

func (h *CreazyCanvasHandler) ListKeys(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.svc.ListKeys(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *CreazyCanvasHandler) Catalog(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	apiKeyID, err := strconv.ParseInt(strings.TrimSpace(c.Query("api_key_id")), 10, 64)
	if err != nil || apiKeyID <= 0 {
		response.BadRequest(c, "api_key_id is required")
		return
	}
	catalog, err := h.svc.Catalog(c.Request.Context(), subject.UserID, apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, catalog)
}

func (h *CreazyCanvasHandler) ListDocuments(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.svc.ListDocuments(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]creazyCanvasDocumentResponse, 0, len(items))
	for i := range items {
		out = append(out, *creazyCanvasDocumentToResponse(&items[i], false))
	}
	response.Success(c, out)
}

func (h *CreazyCanvasHandler) CreateDocument(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req createCreazyCanvasDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	document, err := h.svc.CreateDocument(c.Request.Context(), service.CreateCreazyCanvasDocumentInput{
		UserID:    subject.UserID,
		Name:      req.Name,
		GraphJSON: req.Graph,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, creazyCanvasDocumentToResponse(document, true))
}

func (h *CreazyCanvasHandler) GetDocument(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	documentID, ok := parseIDParam(c, "id", "Invalid document ID")
	if !ok {
		return
	}
	document, err := h.svc.GetDocument(c.Request.Context(), subject.UserID, documentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, creazyCanvasDocumentToResponse(document, true))
}

func (h *CreazyCanvasHandler) UpdateDocument(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	documentID, ok := parseIDParam(c, "id", "Invalid document ID")
	if !ok {
		return
	}
	var req updateCreazyCanvasDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	document, err := h.svc.UpdateDocument(c.Request.Context(), service.UpdateCreazyCanvasDocumentInput{
		UserID:           subject.UserID,
		DocumentID:       documentID,
		Name:             req.Name,
		GraphJSON:        req.Graph,
		ExpectedRevision: req.ExpectedRevision,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, creazyCanvasDocumentToResponse(document, true))
}

func (h *CreazyCanvasHandler) DeleteDocument(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	documentID, ok := parseIDParam(c, "id", "Invalid document ID")
	if !ok {
		return
	}
	if err := h.svc.DeleteDocument(c.Request.Context(), subject.UserID, documentID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": documentID, "deleted": true})
}

func (h *CreazyCanvasHandler) ListWorks(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filters := service.CreazyCanvasWorkListFilters{
		Kind:   c.Query("kind"),
		Status: c.Query("status"),
	}
	if raw := strings.TrimSpace(c.Query("api_key_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filters.APIKeyID = &id
		}
	}
	items, result, err := h.svc.ListWorks(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]creazyCanvasWorkResponse, 0, len(items))
	for i := range items {
		out = append(out, *creazyCanvasWorkToResponse(&items[i]))
	}
	response.Paginated(c, out, result.Total, result.Page, result.PageSize)
}

func (h *CreazyCanvasHandler) CreateWork(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req createCreazyCanvasWorkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	work, err := h.svc.CreateWork(c.Request.Context(), service.CreateCreazyCanvasWorkInput{
		UserID:          subject.UserID,
		APIKeyID:        req.APIKeyID,
		Kind:            req.Kind,
		PublicModel:     req.PublicModel,
		Prompt:          req.Prompt,
		ParamsJSON:      req.Params,
		GatewayType:     req.GatewayType,
		GatewayRemoteID: req.GatewayRemoteID,
		Status:          req.Status,
		ErrorMessage:    req.ErrorMessage,
		PreviewURL:      req.PreviewURL,
		ObjectURL:       req.ObjectURL,
		MimeType:        req.MimeType,
		SizeBytes:       req.SizeBytes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, creazyCanvasWorkToResponse(work))
}

func (h *CreazyCanvasHandler) UpdateWork(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || workID <= 0 {
		response.BadRequest(c, "invalid work id")
		return
	}
	var req updateCreazyCanvasWorkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	work, err := h.svc.UpdateWork(c.Request.Context(), service.UpdateCreazyCanvasWorkInput{
		UserID:          subject.UserID,
		WorkID:          workID,
		Status:          req.Status,
		ErrorMessage:    req.ErrorMessage,
		ParamsJSON:      req.Params,
		GatewayType:     req.GatewayType,
		GatewayRemoteID: req.GatewayRemoteID,
		PreviewURL:      req.PreviewURL,
		ObjectURL:       req.ObjectURL,
		MimeType:        req.MimeType,
		SizeBytes:       req.SizeBytes,
		PublicModel:     req.PublicModel,
		Prompt:          req.Prompt,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, creazyCanvasWorkToResponse(work))
}

func (h *CreazyCanvasHandler) GetWork(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	work, err := h.svc.GetWork(c.Request.Context(), subject.UserID, workID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, creazyCanvasWorkToResponse(work))
}

func (h *CreazyCanvasHandler) DeleteWork(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	if err := h.svc.DeleteWork(c.Request.Context(), subject.UserID, workID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": workID, "deleted": true})
}

func (h *CreazyCanvasHandler) GetDownloadURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	result, err := h.svc.GetDownloadURL(c.Request.Context(), subject.UserID, workID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetPlaybackURL returns a short-lived URL that native media elements can use
// without sending the application's bearer token in a request header.
func (h *CreazyCanvasHandler) GetPlaybackURL(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	result, err := h.svc.GetPlaybackURL(c.Request.Context(), subject.UserID, workID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, result)
}

// GetWorkContent streams work media for the owning user via JWT session.
// Successful works no longer require the user to re-paste API key secret for preview.
func (h *CreazyCanvasHandler) GetWorkContent(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	content, err := h.svc.OpenWorkContent(c.Request.Context(), subject.UserID, workID, c.GetHeader("Range"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if content == nil {
		response.ErrorFrom(c, fmt.Errorf("empty content"))
		return
	}
	writeCreazyCanvasWorkContent(c, content)
}

// StreamWorkPlayback serves a signed, Range-aware work stream for <video>.
// The token is scoped to the work and user and does not grant API access.
func (h *CreazyCanvasHandler) StreamWorkPlayback(c *gin.Context) {
	workID, ok := parseIDParam(c, "id", "Invalid work ID")
	if !ok {
		return
	}
	content, err := h.svc.OpenPlayback(c.Request.Context(), workID, strings.TrimSpace(c.Query("token")), c.GetHeader("Range"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if content == nil {
		response.ErrorFrom(c, fmt.Errorf("empty content"))
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	writeCreazyCanvasWorkContent(c, content)
}

func writeCreazyCanvasWorkContent(c *gin.Context, content *service.CreazyCanvasWorkContent) {
	if content.RedirectURL != "" {
		if strings.HasPrefix(content.RedirectURL, "data:") {
			// data: URLs cannot be HTTP-redirected; return JSON for the client.
			response.Success(c, gin.H{"url": content.RedirectURL, "source": "inline"})
			return
		}
		c.Redirect(http.StatusFound, content.RedirectURL)
		return
	}
	if content.Body != nil {
		defer func() { _ = content.Body.Close() }()
	}
	status := content.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	if content.ContentType != "" {
		c.Header("Content-Type", content.ContentType)
	}
	if content.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(content.ContentLength, 10))
	}
	if content.Filename != "" {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", content.Filename))
	}
	// Pass through useful range headers from upstream when present.
	if content.Header != nil {
		for _, key := range []string{"Accept-Ranges", "Content-Range", "ETag", "Last-Modified", "Cache-Control"} {
			if v := content.Header.Get(key); v != "" {
				c.Header(key, v)
			}
		}
	}
	c.Status(status)
	if c.Request.Method != http.MethodHead && content.Body != nil {
		_, _ = io.Copy(c.Writer, content.Body)
	}
}

func creazyCanvasWorkToResponse(work *service.CreazyCanvasWork) *creazyCanvasWorkResponse {
	if work == nil {
		return nil
	}
	params := work.ParamsJSON
	if params == nil {
		params = map[string]any{}
	}
	return &creazyCanvasWorkResponse{
		ID:              work.ID,
		UserID:          work.UserID,
		APIKeyID:        work.APIKeyID,
		GroupID:         work.GroupID,
		Kind:            work.Kind,
		PublicModel:     work.PublicModel,
		Status:          work.Status,
		Prompt:          work.Prompt,
		Params:          params,
		GatewayType:     work.GatewayType,
		GatewayRemoteID: work.GatewayRemoteID,
		ObjectKey:       work.ObjectKey,
		StorageProvider: work.StorageProvider,
		Bucket:          work.Bucket,
		ObjectURL:       work.ObjectURL,
		PreviewURL:      work.PreviewURL,
		MimeType:        work.MimeType,
		SizeBytes:       work.SizeBytes,
		ErrorMessage:    work.ErrorMessage,
		ExpiresAt:       work.ExpiresAt.Format(time.RFC3339),
		CreatedAt:       work.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       work.UpdatedAt.Format(time.RFC3339),
	}
}

func creazyCanvasDocumentToResponse(document *service.CreazyCanvasDocument, includeGraph bool) *creazyCanvasDocumentResponse {
	if document == nil {
		return nil
	}
	var graph map[string]any
	if includeGraph {
		graph = document.GraphJSON
		if graph == nil {
			graph = map[string]any{}
		}
	}
	return &creazyCanvasDocumentResponse{
		ID:        document.ID,
		Name:      document.Name,
		Graph:     graph,
		Revision:  document.Revision,
		CreatedAt: document.CreatedAt.Format(time.RFC3339),
		UpdatedAt: document.UpdatedAt.Format(time.RFC3339),
	}
}
