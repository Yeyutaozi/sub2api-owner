package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type AgentLocalArtifactStore struct {
	root, prefix, publicBaseURL, signingSecret string
}

func newAgentLocalArtifactStore(cfg config.AgentArtifactStorageConfig) AgentArtifactStore {
	root := strings.TrimSpace(cfg.Endpoint)
	publicBase := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	secret := strings.TrimSpace(cfg.SecretAccessKey)
	if root == "" || publicBase == "" || secret == "" {
		return disabledAgentArtifactStore{err: errors.New("local artifact storage requires endpoint, public_base_url, and secret_access_key")}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return disabledAgentArtifactStore{err: err}
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return disabledAgentArtifactStore{err: err}
	}
	return &AgentLocalArtifactStore{root: abs, prefix: normalizeArtifactPrefix(cfg.Prefix), publicBaseURL: publicBase, signingSecret: secret}
}

func (s *AgentLocalArtifactStore) IsConfigured() bool {
	return s != nil && s.root != "" && s.signingSecret != ""
}
func (s *AgentLocalArtifactStore) Provider() string { return "local" }
func (s *AgentLocalArtifactStore) Bucket() string   { return "local-media" }

func (s *AgentLocalArtifactStore) objectPath(key string) (string, string, error) {
	key = sanitizeArtifactObjectKey(key)
	if s.prefix != "" && !strings.HasPrefix(key, s.prefix+"/") {
		key = strings.TrimLeft(filepath.ToSlash(filepath.Join(s.prefix, key)), "/")
	}
	if key == "" {
		return "", "", infraerrors.BadRequest("AGENT_ARTIFACT_OBJECT_KEY_INVALID", "artifact object key is invalid")
	}
	full := filepath.Join(s.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", infraerrors.BadRequest("AGENT_ARTIFACT_OBJECT_KEY_INVALID", "artifact object key is invalid")
	}
	return full, key, nil
}

func (s *AgentLocalArtifactStore) Put(ctx context.Context, input AgentArtifactStorePutInput) (*AgentArtifactStorePutResult, error) {
	full, key, err := s.objectPath(input.Key)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	written, err := io.Copy(tmp, input.Body)
	if err != nil {
		return nil, fmt.Errorf("write local artifact: %w", err)
	}
	if input.SizeBytes > 0 && written != input.SizeBytes {
		return nil, fmt.Errorf("local artifact size mismatch: got %d, want %d", written, input.SizeBytes)
	}
	if err = tmp.Sync(); err != nil {
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(tmpName, full); err != nil {
		return nil, err
	}
	committed = true
	return &AgentArtifactStorePutResult{Provider: "local", Bucket: s.Bucket(), ObjectKey: key, SizeBytes: written}, nil
}

func (s *AgentLocalArtifactStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetObject(ctx, AgentArtifactObjectLocation{ObjectKey: key}, ttl)
}
func (s *AgentLocalArtifactStore) PresignGetObject(_ context.Context, location AgentArtifactObjectLocation, ttl time.Duration) (string, error) {
	_, key, err := s.objectPath(location.ObjectKey)
	if err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	expires := time.Now().Add(ttl).Unix()
	sig := s.signature(key, expires)
	return s.publicBaseURL + "?key=" + url.QueryEscape(base64.RawURLEncoding.EncodeToString([]byte(key))) + "&expires=" + strconv.FormatInt(expires, 10) + "&signature=" + url.QueryEscape(sig), nil
}
func (s *AgentLocalArtifactStore) signature(key string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(s.signingSecret))
	fmt.Fprintf(mac, "local-media\n%s\n%d", key, expires)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *AgentLocalArtifactStore) ReadObject(_ context.Context, location AgentArtifactObjectLocation, rangeHeader string) (*AgentArtifactObjectReadResult, error) {
	return s.open(location.ObjectKey, rangeHeader)
}
func (s *AgentLocalArtifactStore) OpenSignedObject(_ context.Context, encodedKey string, expires int64, signature, rangeHeader string) (*AgentArtifactObjectReadResult, error) {
	if expires <= time.Now().Unix() || expires > time.Now().Add(24*time.Hour+time.Minute).Unix() {
		return nil, infraerrors.Unauthorized("LOCAL_MEDIA_URL_EXPIRED", "media URL is invalid or expired")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, infraerrors.Unauthorized("LOCAL_MEDIA_URL_INVALID", "media URL is invalid or expired")
	}
	key := string(raw)
	expected := s.signature(key, expires)
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return nil, infraerrors.Unauthorized("LOCAL_MEDIA_URL_INVALID", "media URL is invalid or expired")
	}
	return s.open(key, rangeHeader)
}
func (s *AgentLocalArtifactStore) open(key, rangeHeader string) (*AgentArtifactObjectReadResult, error) {
	full, _, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, infraerrors.NotFound("LOCAL_MEDIA_NOT_FOUND", "media file not found")
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	start, end, status := int64(0), st.Size()-1, http.StatusOK
	if v := strings.TrimSpace(rangeHeader); v != "" {
		start, end, err = parseLocalMediaRange(v, st.Size())
		if err != nil {
			_ = f.Close()
			return &AgentArtifactObjectReadResult{StatusCode: http.StatusRequestedRangeNotSatisfiable, Header: http.Header{"Content-Range": []string{fmt.Sprintf("bytes */%d", st.Size())}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		status = http.StatusPartialContent
		_, err = f.Seek(start, io.SeekStart)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	length := end - start + 1
	h := make(http.Header)
	h.Set("Content-Length", strconv.FormatInt(length, 10))
	h.Set("Accept-Ranges", "bytes")
	h.Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
	if ct := mime.TypeByExtension(filepath.Ext(full)); ct != "" {
		h.Set("Content-Type", ct)
	} else {
		h.Set("Content-Type", "application/octet-stream")
	}
	if status == http.StatusPartialContent {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, st.Size()))
	}
	return &AgentArtifactObjectReadResult{StatusCode: status, Header: h, Body: struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(f, length), Closer: f}}, nil
}
func parseLocalMediaRange(v string, size int64) (int64, int64, error) {
	if !strings.HasPrefix(v, "bytes=") || strings.Contains(v, ",") || size <= 0 {
		return 0, 0, errors.New("invalid range")
	}
	p := strings.SplitN(strings.TrimPrefix(v, "bytes="), "-", 2)
	if len(p) != 2 {
		return 0, 0, errors.New("invalid range")
	}
	if p[0] == "" {
		n, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil || n <= 0 {
			return 0, 0, errors.New("invalid range")
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, nil
	}
	start, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || start < 0 || start >= size {
		return 0, 0, errors.New("invalid range")
	}
	end := size - 1
	if p[1] != "" {
		end, e = strconv.ParseInt(p[1], 10, 64)
		if e != nil || end < start {
			return 0, 0, errors.New("invalid range")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, nil
}
func (s *AgentLocalArtifactStore) Delete(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, AgentArtifactObjectLocation{ObjectKey: key})
}
func (s *AgentLocalArtifactStore) DeleteObject(_ context.Context, location AgentArtifactObjectLocation) error {
	full, _, err := s.objectPath(location.ObjectKey)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
