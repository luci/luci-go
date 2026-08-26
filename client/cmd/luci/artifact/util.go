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
	"os"
	"strconv"
	"strings"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type artifactFetcher func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error

func executeArtifactFetch(ctx context.Context, af *base.AuthFlags, host, outputFile, artName string, fetcher artifactFetcher) int {
	if err := af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	client, _, httpClient, err := af.NewResultDBClient(ctx, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	art, err := client.GetArtifact(ctx, &pb.GetArtifactRequest{Name: artName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetArtifact RPC failed: %s\n", err)
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
	return base.FormatTestResultResourceName(inv, testID, resultID) + "/artifacts/" + artID
}

// FormatWorkUnitArtifactName formats canonical work unit artifact resource name.
func FormatWorkUnitArtifactName(inv, wuID, artID string) string {
	return base.FormatWorkUnitResourceName(inv, wuID) + "/artifacts/" + artID
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

	if _, err := io.Copy(out, resp.Body); err != nil {
		return resp.StatusCode, totalSize, errors.Fmt("failed to write response body: %w", err)
	}
	return resp.StatusCode, totalSize, nil
}

// ParentType indicates whether an artifact operation is scoped to a Test Result or a Work Unit.
type ParentType int

const (
	ParentTypeTestResult ParentType = iota
	ParentTypeWorkUnit
)
