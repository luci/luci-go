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

func executeArtifactFetch(ctx context.Context, af *base.AuthFlags, host, outputFile string, args []string, parentType ParentType, legacy bool, fetcher artifactFetcher) int {
	if len(args) < 1 {
		if parentType == ParentTypeTestResult {
			fmt.Fprintf(os.Stderr, "Usage: luci test-result artifact <command> <target> <artifact_id> | <command> <inv> <test_id> <result_id> <artifact_id>\n")
		} else {
			fmt.Fprintf(os.Stderr, "Usage: luci work-unit artifact <command> <target> <artifact_id> | <command> <root_inv> <work_unit_id> <artifact_id>\n")
		}
		return 1
	}

	if err := af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	client, _, httpClient, err := af.NewResultDBClient(ctx, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	artName, err := ResolveArtifactTarget(ctx, client, args, parentType, legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
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

// ValidateTargetParent validates that target matches the expected parent type.
func ValidateTargetParent(target string, expectedParent ParentType) error {
	isWU := strings.Contains(target, "/workUnits/") || (strings.HasPrefix(target, "rootInvocations/") && !strings.Contains(target, "/tests/"))
	isTR := strings.Contains(target, "/tests/") || strings.Contains(target, "/results/") || strings.Contains(target, "/variants/") || strings.Contains(target, "/modules/")

	if expectedParent == ParentTypeTestResult && isWU && !isTR {
		return errors.Fmt("%q is a work unit target; please use 'luci work-unit artifact ...'", target)
	}
	if expectedParent == ParentTypeWorkUnit && isTR {
		return errors.Fmt("%q is a test result target; please use 'luci test-result artifact ...'", target)
	}
	return nil
}

// ResolveArtifactTarget normalizes target arguments into a ResultDB artifact resource name,
// validating parent type compatibility and resolving cached parameters with "-".
func ResolveArtifactTarget(ctx context.Context, client pb.ResultDBClient, args []string, parentType ParentType, legacy bool) (string, error) {
	if len(args) == 0 {
		return "", errors.New("missing artifact target")
	}

	cd, _ := base.LoadCache()

	// Case 1: single '-' reuses both cached parent and cached artifact.
	if len(args) == 1 && args[0] == "-" {
		var parent string
		var err error
		if parentType == ParentTypeTestResult {
			parent, err = base.ParseTestResultTargetArgs([]string{"-"})
		} else {
			parent, err = base.ParseWorkUnitTargetArgs([]string{"-"})
		}
		if err != nil {
			return "", err
		}
		if cd.Artifact == "" {
			return "", errors.New("no previous artifact found in cache; please specify an artifact ID")
		}
		return resolveTargetAndArtifactID(ctx, client, parent, cd.Artifact, parentType, legacy)
	}

	// Case 2: single full artifact name or URL.
	if len(args) == 1 {
		rawName := strings.TrimSpace(args[0])
		if err := ValidateTargetParent(rawName, parentType); err != nil {
			return "", err
		}
		return ResolveArtifactName(ctx, client, rawName, parentType, legacy)
	}

	// Case 3: N arguments where the last argument is artifact ID, and preceding args are parent target.
	parentArgs := args[:len(args)-1]
	artID := strings.TrimSpace(args[len(args)-1])
	if artID == "" {
		return "", errors.New("empty artifact ID")
	}

	var parentTarget string
	var err error
	if parentType == ParentTypeTestResult {
		parentTarget, err = base.ParseTestResultTargetArgs(parentArgs)
	} else {
		parentTarget, err = base.ParseWorkUnitTargetArgs(parentArgs)
	}
	if err != nil {
		return "", err
	}

	if err := ValidateTargetParent(parentTarget, parentType); err != nil {
		return "", err
	}

	res, err := resolveTargetAndArtifactID(ctx, client, parentTarget, artID, parentType, legacy)
	if err != nil {
		return "", err
	}
	if parentType == ParentTypeTestResult {
		base.RecordTestResultArtifact(parentTarget, "", "", "", artID)
	} else {
		base.RecordWorkUnitArtifact(parentTarget, "", "", artID)
	}
	return res, nil
}

func resolveTargetAndArtifactID(ctx context.Context, client pb.ResultDBClient, target, artID string, parentType ParentType, legacy bool) (string, error) {
	clean := base.TrimResourceURL(target)

	if strings.Contains(clean, "/results/") || strings.Contains(clean, "/workUnits/") {
		return clean + "/artifacts/" + artID, nil
	}

	if strings.HasPrefix(clean, "invocations/") && !strings.Contains(clean, "/tests/") && !strings.Contains(clean, "/modules/") {
		return clean + "/artifacts/" + artID, nil
	}

	// Try verdict target
	_, matchedTR, vErr := verdict.ResolveVerdictResults(ctx, client, target, legacy)
	if vErr == nil && matchedTR != nil {
		return matchedTR.Name + "/artifacts/" + artID, nil
	}

	return clean + "/artifacts/" + artID, nil
}

// ResolveArtifactName normalizes a target artifact identifier or URL into a ResultDB artifact resource name.
func ResolveArtifactName(ctx context.Context, client pb.ResultDBClient, rawName string, parentType ParentType, legacy bool) (string, error) {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		return "", errors.New("empty artifact name")
	}

	if strings.Contains(rawName, "artifact=") {
		u, err := url.Parse(rawName)
		if err == nil {
			artID := u.Query().Get("artifact")
			if artID != "" {
				res, err := resolveTargetAndArtifactID(ctx, client, rawName, artID, parentType, legacy)
				if err == nil {
					if parentType == ParentTypeWorkUnit {
						base.RecordWorkUnitArtifact(res, "", "", artID)
					} else {
						base.RecordTestResultArtifact(res, "", "", "", artID)
					}
				}
				return res, err
			}
		}
	}

	clean := base.TrimResourceURL(rawName)

	if !strings.Contains(clean, "/artifacts/") {
		return "", errors.Fmt("invalid artifact name %q: must contain /artifacts/ or specify target and artifact_id", rawName)
	}

	parts := strings.Split(clean, "/artifacts/")
	if len(parts) == 2 {
		if parentType == ParentTypeWorkUnit {
			base.RecordWorkUnitArtifact(clean, "", "", parts[1])
		} else {
			base.RecordTestResultArtifact(clean, "", "", "", parts[1])
		}
	}

	return clean, nil
}
