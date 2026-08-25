package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Weijin's content endpoint can close a large response after roughly a minute
// without returning a useful HTTP error. A bounded range reader lets a client
// finish the object while still placing a hard cap on retry amplification.
const (
	weijinContentResumeMaxAttempts   = 256
	weijinContentResumeMaxEmptyReads = 100
	// A provider response can remain open without producing another byte. A
	// per-segment deadline turns that silent stall into a resumable read error.
	weijinContentSegmentTimeout = 90 * time.Second
	// The upstream content server may terminate a long response at roughly
	// one minute. Keep retries below that window so each request has a chance
	// to finish instead of repeatedly asking for the entire remaining object.
	weijinContentResumeChunkSize int64 = 512 << 10
)

// weijinContentTimedBody keeps the segment context alive while its body is
// being consumed, then cancels it as soon as the segment is closed. Without
// this wrapper a timeout context would either be cancelled before the body is
// read or leak until its deadline after a successful segment.
type weijinContentTimedBody struct {
	body     io.ReadCloser
	cancel   context.CancelFunc
	once     sync.Once
	closeErr error
}

func (b *weijinContentTimedBody) Read(p []byte) (int, error) {
	if b == nil || b.body == nil {
		return 0, io.ErrClosedPipe
	}
	return b.body.Read(p)
}

func (b *weijinContentTimedBody) Close() error {
	if b == nil {
		return nil
	}
	b.once.Do(func() {
		if b.body != nil {
			b.closeErr = b.body.Close()
		}
		if b.cancel != nil {
			b.cancel()
		}
	})
	return b.closeErr
}

type weijinContentResumeReader struct {
	service   *OpenAIGatewayService
	ctx       context.Context
	ginCtx    *gin.Context
	account   *Account
	targetURL string
	endpoint  string
	apiKey    string

	body       io.ReadCloser
	offset     int64 // absolute byte offset represented by the next read
	segmentEnd int64 // end of the currently open response, inclusive
	targetEnd  int64 // end of the client-requested object/range, inclusive
	total      int64
	attempts   int
	emptyReads int
	closed     bool
}

// newWeijinContentResumeReader wraps only responses with enough byte metadata
// to prove that a premature EOF is a short read. Unknown/chunked responses are
// returned unchanged because retrying them could duplicate or lose bytes.
func newWeijinContentResumeReader(
	service *OpenAIGatewayService,
	ctx context.Context,
	ginCtx *gin.Context,
	account *Account,
	targetURL, endpoint, apiKey, requestedRange string,
	response *SeedanceUpstreamResponse,
) io.ReadCloser {
	if response == nil || response.BodyStream == nil || service == nil {
		if response == nil {
			return nil
		}
		return response.BodyStream
	}

	start, segmentEnd, total, ok := weijinContentRangeFromResponse(response)
	if !ok {
		return response.BodyStream
	}
	if total <= 0 || start < 0 || segmentEnd < start || segmentEnd >= total {
		return response.BodyStream
	}

	targetEnd := segmentEnd
	unranged := strings.TrimSpace(requestedRange) == ""
	requestedStart, requestedEnd, hasRange := parseWeijinSingleRange(requestedRange)
	if unranged || !hasRange {
		// A provider may answer an un-ranged request with a partial response;
		// continue through the complete object in that case.
		targetEnd = total - 1
		// Without a client Range there is no safe way to reconstruct bytes that
		// precede a provider response starting in the middle of the object.
		if !unranged && !hasRange {
			return response.BodyStream
		}
		if unranged && response.StatusCode == http.StatusPartialContent && start != 0 {
			return response.BodyStream
		}
	} else {
		// A ranged client response must retain the provider's original range
		// semantics. Do not turn a provider's range-ignoring 200 response into a
		// truncated 200 response by wrapping it as though it were a partial one.
		if response.StatusCode != http.StatusPartialContent {
			return response.BodyStream
		}
		if requestedStart >= 0 && requestedStart != start {
			// The upstream did not honor the requested range. Preserve the old
			// pass-through behavior rather than trying to rewrite its semantics.
			return response.BodyStream
		}
		if requestedEnd >= 0 {
			targetEnd = requestedEnd
		} else {
			// An open-ended client range extends through the end of the object,
			// not merely through the provider's first response segment.
			targetEnd = total - 1
		}
	}
	if targetEnd < segmentEnd || targetEnd >= total {
		return response.BodyStream
	}

	// The first upstream segment may be shorter than the requested object. Keep
	// the outward response metadata consistent with the bytes this reader will
	// produce, otherwise clients can stop after the first segment's length.
	if response.Header == nil {
		response.Header = make(http.Header)
	}
	if unranged {
		if response.StatusCode == http.StatusPartialContent {
			response.StatusCode = http.StatusOK
			response.Header.Del("Content-Range")
			response.Header.Set("Content-Length", strconv.FormatInt(total, 10))
		}
	} else if hasRange {
		response.StatusCode = http.StatusPartialContent
		response.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", requestedStart, targetEnd, total))
		response.Header.Set("Content-Length", strconv.FormatInt(targetEnd-requestedStart+1, 10))
	}

	if ctx == nil {
		ctx = context.Background()
	}
	return &weijinContentResumeReader{
		service:    service,
		ctx:        ctx,
		ginCtx:     ginCtx,
		account:    account,
		targetURL:  targetURL,
		endpoint:   endpoint,
		apiKey:     apiKey,
		body:       response.BodyStream,
		offset:     start,
		segmentEnd: segmentEnd,
		targetEnd:  targetEnd,
		total:      total,
	}
}

func (r *weijinContentResumeReader) Read(p []byte) (int, error) {
	if r == nil || r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}

	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		if r.offset > r.targetEnd {
			if r.body != nil {
				_ = r.body.Close()
				r.body = nil
			}
			return 0, io.EOF
		}
		if r.body == nil || r.offset > r.segmentEnd {
			if r.body != nil {
				_ = r.body.Close()
				r.body = nil
			}
			if err := r.resume(); err != nil {
				return 0, err
			}
			continue
		}

		remaining := r.segmentEnd - r.offset + 1
		if remaining <= 0 {
			continue
		}
		readBuffer := p
		if int64(len(readBuffer)) > remaining {
			readBuffer = readBuffer[:remaining]
		}
		n, readErr := r.body.Read(readBuffer)
		if n > 0 {
			r.offset += int64(n)
			r.emptyReads = 0
		}

		if readErr == nil {
			if n == 0 {
				r.emptyReads++
				if r.emptyReads >= weijinContentResumeMaxEmptyReads {
					return 0, io.ErrNoProgress
				}
			}
			return n, nil
		}
		if r.offset > r.targetEnd {
			if r.body != nil {
				_ = r.body.Close()
				r.body = nil
			}
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}

		// A clean EOF at the end of one response still needs another range
		// request when that response covered only part of the object. A short
		// read is handled by requesting from the exact byte offset reached.
		if r.body != nil {
			_ = r.body.Close()
			r.body = nil
		}
		resumeErr := r.resume()
		if resumeErr != nil {
			if n > 0 {
				return n, resumeErr
			}
			return 0, resumeErr
		}
		if n > 0 {
			// Return the bytes already read and let the next Read consume the
			// resumed response. Returning nil here prevents io.Copy from
			// discarding the successful continuation.
			return n, nil
		}
	}
}

func (r *weijinContentResumeReader) resume() error {
	if r == nil || r.closed {
		return io.ErrClosedPipe
	}
	if r.offset > r.targetEnd {
		return nil
	}
	if r.attempts >= weijinContentResumeMaxAttempts {
		return fmt.Errorf("Weijin video content resume limit exceeded")
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.attempts++

	resumeEnd := r.targetEnd
	if remaining := r.targetEnd - r.offset + 1; remaining > weijinContentResumeChunkSize {
		resumeEnd = r.offset + weijinContentResumeChunkSize - 1
	}
	rangeHeader := fmt.Sprintf("bytes=%d-%d", r.offset, resumeEnd)
	response, err := r.service.doWeijinSeedanceRequest(
		r.ctx,
		r.ginCtx,
		r.account,
		http.MethodGet,
		r.targetURL,
		r.endpoint,
		r.apiKey,
		nil,
		rangeHeader,
	)
	if err != nil {
		return fmt.Errorf("resume Weijin video content: %w", err)
	}
	if response == nil || response.BodyStream == nil {
		return fmt.Errorf("resume Weijin video content: empty upstream body")
	}
	start, end, total, ok := weijinContentRangeFromResponse(response)
	if !ok || response.StatusCode != http.StatusPartialContent || start != r.offset || end < start || end > resumeEnd || total != r.total {
		_ = response.BodyStream.Close()
		return fmt.Errorf("resume Weijin video content: upstream returned an invalid byte range")
	}
	r.body = response.BodyStream
	r.segmentEnd = end
	return nil
}

func (r *weijinContentResumeReader) Close() error {
	if r == nil || r.closed {
		return nil
	}
	r.closed = true
	if r.body == nil {
		return nil
	}
	err := r.body.Close()
	r.body = nil
	return err
}

func weijinContentRangeFromResponse(response *SeedanceUpstreamResponse) (start, end, total int64, ok bool) {
	if response == nil {
		return 0, 0, 0, false
	}
	if raw := strings.TrimSpace(response.Header.Get("Content-Range")); raw != "" {
		start, end, total, ok = parseWeijinContentRange(raw)
		if ok {
			return start, end, total, true
		}
		return 0, 0, 0, false
	}
	if response.StatusCode != http.StatusOK {
		return 0, 0, 0, false
	}
	length := weijinResponseContentLength(response)
	if length <= 0 {
		return 0, 0, 0, false
	}
	return 0, length - 1, length, true
}

func weijinResponseContentLength(response *SeedanceUpstreamResponse) int64 {
	if response == nil {
		return -1
	}
	if value := strings.TrimSpace(response.Header.Get("Content-Length")); value != "" {
		if length, err := strconv.ParseInt(value, 10, 64); err == nil {
			return length
		}
	}
	return -1
}

func parseWeijinContentRange(value string) (start, end, total int64, ok bool) {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bytes") {
		return 0, 0, 0, false
	}
	rangeAndTotal := strings.SplitN(parts[1], "/", 2)
	if len(rangeAndTotal) != 2 || strings.TrimSpace(rangeAndTotal[1]) == "*" {
		return 0, 0, 0, false
	}
	bounds := strings.SplitN(strings.TrimSpace(rangeAndTotal[0]), "-", 2)
	if len(bounds) != 2 {
		return 0, 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(bounds[0]), 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	end, err = strconv.ParseInt(strings.TrimSpace(bounds[1]), 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	total, err = strconv.ParseInt(strings.TrimSpace(rangeAndTotal[1]), 10, 64)
	if err != nil || start < 0 || end < start || total <= end {
		return 0, 0, 0, false
	}
	return start, end, total, true
}

func parseWeijinSingleRange(value string) (start, end int64, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, -1, false
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "bytes") || strings.Contains(parts[1], ",") {
		return 0, -1, false
	}
	bounds := strings.SplitN(strings.TrimSpace(parts[1]), "-", 2)
	if len(bounds) != 2 || strings.TrimSpace(bounds[0]) == "" {
		return 0, -1, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(bounds[0]), 10, 64)
	if err != nil || start < 0 {
		return 0, -1, false
	}
	end = -1
	if rawEnd := strings.TrimSpace(bounds[1]); rawEnd != "" {
		end, err = strconv.ParseInt(rawEnd, 10, 64)
		if err != nil || end < start {
			return 0, -1, false
		}
	}
	return start, end, true
}
