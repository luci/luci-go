// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package artifact

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type artifactFetcher func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error

// ResolveParentType determines whether the target is a test result, work unit, or invocation based on provided IDs.
func ResolveParentType(invID, wuID, testID, resultID string) (ParentType, error) {
	if testID != "" || resultID != "" {
		if invID == "" || testID == "" || resultID == "" {
			return ParentTypeUnknown, errors.New("flags -invocationid, -testid, and -resultid are required for test result artifacts (run 'luci ids <url>' to extract ids)")
		}
		return ParentTypeTestResult, nil
	}
	if wuID != "" {
		if invID == "" {
			return ParentTypeUnknown, errors.New("flags -invocationid and -workunitid are required (run 'luci ids <url>' to extract ids)")
		}
		return ParentTypeWorkUnit, nil
	}
	if invID == "" {
		return ParentTypeUnknown, errors.New("flags -invocationid and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required (run 'luci ids <url>' to extract ids)")
	}
	return ParentTypeInvocation, nil
}

// ValidateArtifactFlags verifies that required ID flags are provided for an artifact query.
func ValidateArtifactFlags(parentType ParentType, invID, wuID, testID, resultID, artID string) error {
	if artID == "" {
		return errors.New("flag -artifactid is required (run 'luci ids <url>' to extract ids)")
	}
	switch parentType {
	case ParentTypeTestResult:
		if invID == "" || testID == "" || resultID == "" {
			return errors.New("flags -invocationid, -testid, -resultid, and -artifactid are required (run 'luci ids <url>' to extract ids)")
		}
	case ParentTypeWorkUnit:
		if invID == "" || wuID == "" {
			return errors.New("flags -invocationid, -workunitid, and -artifactid are required (run 'luci ids <url>' to extract ids)")
		}
	case ParentTypeInvocation:
		if invID == "" {
			return errors.New("flags -invocationid and -artifactid are required (run 'luci ids <url>' to extract ids)")
		}
	default:
		if testID != "" || resultID != "" {
			if invID == "" || testID == "" || resultID == "" {
				return errors.New("flags -invocationid, -testid, -resultid, and -artifactid are required for test result artifacts (run 'luci ids <url>' to extract ids)")
			}
		} else if wuID != "" {
			if invID == "" {
				return errors.New("flags -invocationid, -workunitid, and -artifactid are required (run 'luci ids <url>' to extract ids)")
			}
		} else {
			if invID == "" {
				return errors.New("flags -invocationid, -artifactid, and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required (run 'luci ids <url>' to extract ids)")
			}
		}
	}
	return nil
}

// ResolveTestResultResourceName resolves the canonical resource name for a test result.
// - In non-legacy mode when workUnitID is provided: formats "rootInvocations/<invID>/workUnits/<wuID>/tests/<testID>/results/<resultID>".
// - Otherwise: queries test results in the invocation to find the canonical result resource name.
func ResolveTestResultResourceName(ctx context.Context, client pb.ResultDBClient, invID, wuID, testID, resultID string, legacy bool) (string, error) {
	if !legacy && wuID != "" {
		return base.FormatTestResultWorkUnitResourceName(invID, wuID, testID, resultID), nil
	}
	results, _, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, client, invID, testID, "", legacy, 0)
	if err != nil {
		return "", errors.Fmt("failed to query test results: %w", err)
	}
	for _, tr := range results {
		if tr.ResultId == resultID {
			return tr.Name, nil
		}
	}
	return "", errors.Fmt("test result %q not found for test %q in invocation %q", resultID, testID, invID)
}

// ResolveTargetResourceName resolves the canonical resource name for a test result, work unit, or invocation.
func ResolveTargetResourceName(ctx context.Context, client pb.ResultDBClient, parentType ParentType, invID, wuID, testID, resultID string, legacy bool) (string, error) {
	if parentType == ParentTypeUnknown {
		var err error
		parentType, err = ResolveParentType(invID, wuID, testID, resultID)
		if err != nil {
			return "", err
		}
	}
	if parentType == ParentTypeInvocation {
		if legacy {
			return "invocations/" + base.NormalizeInvocation(invID), nil
		}
		return "rootInvocations/" + base.NormalizeInvocation(invID), nil
	}
	if parentType == ParentTypeWorkUnit {
		return base.FormatWorkUnitResourceName(invID, wuID), nil
	}
	return ResolveTestResultResourceName(ctx, client, invID, wuID, testID, resultID, legacy)
}

// ResolveArtifactResourceName resolves the full artifact resource name for a test result or work unit.
func ResolveArtifactResourceName(ctx context.Context, client pb.ResultDBClient, parentType ParentType, invID, wuID, testID, resultID, artID string, legacy bool) (string, error) {
	targetName, err := ResolveTargetResourceName(ctx, client, parentType, invID, wuID, testID, resultID, legacy)
	if err != nil {
		return "", err
	}
	return targetName + "/artifacts/" + url.PathEscape(artID), nil
}

func executeArtifactFetch(ctx context.Context, af *base.AuthFlags, host, outputFile string, parentType ParentType, invID, wuID, testID, resultID, artID string, fetcher artifactFetcher) int {
	if err := af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	client, _, httpClient, err := af.NewResultDBClient(ctx, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, invID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch artifact: %s\n", err)
		return 1
	}
	if resolved.Legacy && parentType == ParentTypeWorkUnit {
		fmt.Fprintf(os.Stderr, "work unit artifacts are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	artName, err := ResolveArtifactResourceName(ctx, client, parentType, resolved.ID, wuID, testID, resultID, artID, resolved.Legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	art, err := client.GetArtifact(ctx, &pb.GetArtifactRequest{Name: artName})
	if err != nil {
		if base.IsPermissionDenied(err) {
			fmt.Fprintf(os.Stderr, "GetArtifact RPC failed (missing ResultDB realm permission): %s\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "GetArtifact RPC failed: %s\n", err)
		}
		return 1
	}
	if art.FetchUrl == "" {
		fmt.Fprintf(os.Stderr, "artifact %q has no fetch URL\n", artName)
		return 1
	}

	var outWriter io.Writer = os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create output file: %s\n", err)
			return 1
		}
		defer f.Close()
		outWriter = f
	}

	if err := fetcher(ctx, httpClient, art.FetchUrl, outWriter); err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch artifact: %s\n", err)
		return 1
	}

	return 0
}

// FormatTestResultArtifactName formats canonical test result artifact resource name.
func FormatTestResultArtifactName(inv, testID, resultID, artID string) string {
	return base.FormatTestResultResourceName(inv, testID, resultID) + "/artifacts/" + url.PathEscape(artID)
}

// FormatTestResultWorkUnitArtifactName formats canonical test result artifact resource name under a work unit.
func FormatTestResultWorkUnitArtifactName(inv, wuID, testID, resultID, artID string) string {
	return base.FormatTestResultWorkUnitResourceName(inv, wuID, testID, resultID) + "/artifacts/" + url.PathEscape(artID)
}

// FormatWorkUnitArtifactName formats canonical work unit artifact resource name.
func FormatWorkUnitArtifactName(inv, wuID, artID string) string {
	return base.FormatWorkUnitResourceName(inv, wuID) + "/artifacts/" + url.PathEscape(artID)
}

// FetchHTTPByteRange downloads a byte range (or full content if byteRange is nil) from fetchURL directly into out.
// Returns the HTTP status code, total content size (from Content-Range, or Content-Length, or -1 if unknown), and error.
func FetchHTTPByteRange(ctx context.Context, httpClient *http.Client, fetchURL string, byteRange *ByteRange, out io.Writer) (int, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return 0, -1, errors.Fmt("failed to create HTTP request: %w", err)
	}

	if byteRange != nil {
		var rangeHeader string
		if byteRange.Start < 0 && byteRange.End >= 0 {
			rangeHeader = fmt.Sprintf("bytes=-%d", byteRange.End)
		} else if byteRange.Start >= 0 && byteRange.End < 0 {
			rangeHeader = fmt.Sprintf("bytes=%d-", byteRange.Start)
		} else if byteRange.Start >= 0 && byteRange.End >= 0 {
			rangeHeader = fmt.Sprintf("bytes=%d-%d", byteRange.Start, byteRange.End)
		}
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, -1, errors.Fmt("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		var totalSize int64 = 0
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			if idx := strings.LastIndex(cr, "/"); idx != -1 {
				if total, err := strconv.ParseInt(cr[idx+1:], 10, 64); err == nil && total >= 0 {
					totalSize = total
				}
			}
		}
		return resp.StatusCode, totalSize, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return resp.StatusCode, -1, errors.Fmt("HTTP GET failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var totalSize int64 = -1
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if idx := strings.LastIndex(cr, "/"); idx != -1 {
			if total, err := strconv.ParseInt(cr[idx+1:], 10, 64); err == nil && total > 0 {
				totalSize = total
			}
		}
	} else if resp.ContentLength > 0 && resp.StatusCode == http.StatusOK {
		totalSize = resp.ContentLength
	}

	// Some upstream servers (such as Google Cloud Storage when serving objects
	// with Content-Encoding: gzip via decompressive transcoding) ignore the HTTP
	// Range header and respond with 200 OK and the entire uncompressed payload.
	// Enforce the requested byteRange on the client side when that happens.
	if resp.StatusCode == http.StatusOK && byteRange != nil {
		// Case 1: Suffix range (bytes=-N, represented as Start < 0, End >= 0).
		// Read the stream to EOF using a bounded circular buffer so only the last
		// byteRange.End bytes are kept in memory and written to out.
		if byteRange.Start < 0 && byteRange.End >= 0 {
			nRead, err := copySuffix(out, resp.Body, byteRange.End)
			if totalSize <= 0 {
				totalSize = nRead
			}
			if err != nil {
				return resp.StatusCode, totalSize, errors.Fmt("failed to write response body: %w", err)
			}
			return resp.StatusCode, totalSize, nil
		}
		// For [Start, End] and [Start, -1] ranges, discard the first Start bytes.
		if byteRange.Start > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, byteRange.Start); err != nil {
				if errors.Is(err, io.EOF) {
					// Stream ended before reaching byteRange.Start; nothing to write.
					return resp.StatusCode, totalSize, nil
				}
				return resp.StatusCode, totalSize, errors.Fmt("failed to discard leading bytes: %w", err)
			}
		}
		// Case 2: Bounded range [Start, End] (Start >= 0, End >= 0).
		// Copy at most (End - Start + 1) bytes and return immediately so
		// defer resp.Body.Close() aborts downloading the remainder of the stream.
		if byteRange.Start >= 0 && byteRange.End >= 0 {
			length := byteRange.End - byteRange.Start + 1
			if length > 0 {
				if _, err := io.CopyN(out, resp.Body, length); err != nil && !errors.Is(err, io.EOF) {
					return resp.StatusCode, totalSize, errors.Fmt("failed to write response body: %w", err)
				}
			}
			return resp.StatusCode, totalSize, nil
		}
		// Case 3: Open-ended range [Start, -1] (bytes=Start-) falls through to
		// io.Copy below after discarding the leading Start bytes above.
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return resp.StatusCode, totalSize, errors.Fmt("failed to write response body: %w", err)
	}
	return resp.StatusCode, totalSize, nil
}

// copySuffix reads src until EOF, retaining at most the last suffixLen bytes in
// a ring buffer, and writes those trailing bytes to dst. It returns the total
// number of bytes read from src.
//
// To avoid pre-allocating a large buffer when suffixLen is much larger than the
// actual stream size, ring grows lazily until totalRead reaches suffixLen. Once
// suffixLen bytes have been buffered (wrapped == true), ring has fixed length
// suffixLen and acts as a circular buffer where writePos (0 <= writePos < suffixLen)
// points to the oldest byte (the next slot to be overwritten).
func copySuffix(dst io.Writer, src io.Reader, suffixLen int64) (int64, error) {
	if suffixLen <= 0 {
		return io.Copy(io.Discard, src)
	}
	var (
		ring      []byte
		writePos  int64
		wrapped   bool
		totalRead int64
		readBuf   = make([]byte, 32*1024)
	)
	for {
		n, err := src.Read(readBuf)
		if n > 0 {
			totalRead += int64(n)
			chunk := readBuf[:n]
			if int64(len(chunk)) >= suffixLen {
				// Current read chunk alone is at least suffixLen bytes; it completely
				// supersedes any previously buffered data. Keep only its last
				// suffixLen bytes at the start of ring and reset writePos to 0.
				chunk = chunk[int64(len(chunk))-suffixLen:]
				if int64(len(ring)) < suffixLen {
					ring = make([]byte, suffixLen)
				}
				copy(ring, chunk)
				writePos = 0
				wrapped = true
			} else if !wrapped {
				// Still accumulating the initial suffixLen bytes.
				if int64(len(ring)+len(chunk)) <= suffixLen {
					ring = append(ring, chunk...)
					if int64(len(ring)) == suffixLen {
						// Buffer just reached full capacity suffixLen.
						writePos = 0
						wrapped = true
					}
				} else {
					// Appending chunk crosses suffixLen for the first time. Allocate
					// the full suffixLen buffer, retain the newest (suffixLen - len(chunk))
					// bytes from ring followed by chunk, and mark the ring as full.
					fullRing := make([]byte, suffixLen)
					keepFromOld := suffixLen - int64(len(chunk))
					copy(fullRing, ring[int64(len(ring))-keepFromOld:])
					copy(fullRing[keepFromOld:], chunk)
					ring = fullRing
					writePos = 0
					wrapped = true
				}
			} else {
				// Ring is already at full capacity (len(ring) == suffixLen) and
				// len(chunk) < suffixLen. Overwrite starting at writePos, wrapping
				// around to index 0 if the chunk crosses the end of the slice.
				firstPart := suffixLen - writePos
				if int64(len(chunk)) <= firstPart {
					copy(ring[writePos:], chunk)
					writePos = (writePos + int64(len(chunk))) % suffixLen
				} else {
					copy(ring[writePos:], chunk[:firstPart])
					rem := int64(len(chunk)) - firstPart
					copy(ring[:rem], chunk[firstPart:])
					writePos = rem
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return totalRead, err
		}
	}

	if !wrapped {
		// Stream was shorter than suffixLen; ring holds all bytes in order.
		_, err := dst.Write(ring)
		return totalRead, err
	}
	// Stream was >= suffixLen bytes. In the circular buffer, ring[writePos:]
	// holds the older bytes and ring[:writePos] holds the newer wrapped bytes.
	if _, err := dst.Write(ring[writePos:]); err != nil {
		return totalRead, err
	}
	if writePos > 0 {
		if _, err := dst.Write(ring[:writePos]); err != nil {
			return totalRead, err
		}
	}
	return totalRead, nil
}

// FormatInvocationArtifactName formats canonical invocation artifact resource name.
func FormatInvocationArtifactName(inv, artID string, legacy bool) string {
	if legacy {
		return "invocations/" + base.NormalizeInvocation(inv) + "/artifacts/" + url.PathEscape(artID)
	}
	return "rootInvocations/" + base.NormalizeInvocation(inv) + "/artifacts/" + url.PathEscape(artID)
}

// ParentType indicates whether an artifact operation is scoped to a Test Result, Work Unit, or Invocation.
type ParentType int

const (
	ParentTypeTestResult ParentType = iota
	ParentTypeWorkUnit
	ParentTypeInvocation
	ParentTypeUnknown
)
