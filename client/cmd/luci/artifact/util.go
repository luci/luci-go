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
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type artifactFetcher func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error

// artifactCommonRun holds the common flags and client state shared across
// artifact subcommands (list, get, head, tail).
type artifactCommonRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       rdbpb.ResultDBClient
	httpClient   *http.Client
	parentType   ParentType
	host         string
	invocationID string
	testID       string
	resultID     string
	workUnitID   string
	artifactID   string
	outputFile   string
	legacy       bool // Unused; will be removed in the future.
}

func (r *artifactCommonRun) registerTargetFlags(af *base.AuthFlags, parentType ParentType) {
	r.af = af
	r.parentType = parentType
	if r.af != nil {
		r.af.Register(&r.Flags)
	}
	r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
	r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
	if parentType == ParentTypeTestResult {
		r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
		r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
		r.Flags.StringVar(&r.workUnitID, "workunitid", "", "Work unit ID (optional)")
		// -legacy is now unused and will be removed in the future.
		r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
	} else if parentType == ParentTypeWorkUnit {
		r.Flags.StringVar(&r.workUnitID, "workunitid", "", "Work unit ID (e.g. run-tests or ants-wu...)")
	} else {
		r.Flags.StringVar(&r.workUnitID, "workunitid", "", "Work unit ID (e.g. run-tests or ants-wu...)")
		r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
		r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
		// -legacy is now unused and will be removed in the future.
		r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
	}
}

func (r *artifactCommonRun) registerFetchFlags(af *base.AuthFlags, parentType ParentType) {
	r.registerTargetFlags(af, parentType)
	r.Flags.StringVar(&r.artifactID, "artifactid", "", "Artifact ID (e.g. text_log or test.xml)")
	r.Flags.StringVar(&r.outputFile, "o", "", "Optional file path to save the artifact content to")
	r.Flags.StringVar(&r.outputFile, "output", "", "Optional file path to save the artifact content to (alias for -o)")
}

func (r *artifactCommonRun) checkHelpAndPositionalArgs(args []string) (handled bool, exitCode int) {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return true, 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments (run 'luci ids <url>' to extract ids)\n")
		return true, 1
	}
	return false, 0
}

func (r *artifactCommonRun) validateFetchFlags(args []string) (effectiveParentType ParentType, handled bool, exitCode int) {
	if handled, code := r.checkHelpAndPositionalArgs(args); handled {
		return ParentTypeUnknown, true, code
	}
	if err := ValidateArtifactFlags(r.parentType, r.invocationID, r.workUnitID, r.testID, r.resultID, r.artifactID); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return ParentTypeUnknown, true, 1
	}
	effectiveParentType = r.parentType
	if effectiveParentType == ParentTypeUnknown {
		effectiveParentType, _ = ResolveParentType(r.invocationID, r.workUnitID, r.testID, r.resultID)
	}
	return effectiveParentType, false, 0
}

func (r *artifactCommonRun) ensureClients(ctx context.Context, needHTTP bool) bool {
	if r.client != nil && (!needHTTP || r.httpClient != nil) {
		return true
	}
	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return false
	}
	c, _, hc, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return false
	}
	if r.client == nil {
		r.client = c
	}
	if r.httpClient == nil {
		r.httpClient = hc
	}
	return true
}

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
func ResolveTestResultResourceName(ctx context.Context, client rdbpb.ResultDBClient, invID, wuID, testID, resultID string, legacy bool) (string, error) {
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
func ResolveTargetResourceName(ctx context.Context, client rdbpb.ResultDBClient, parentType ParentType, invID, wuID, testID, resultID string, legacy bool) (string, error) {
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

// ResolveArtifactResourceName resolves the full artifact resource name for a test result, work unit, or invocation.
func ResolveArtifactResourceName(ctx context.Context, client rdbpb.ResultDBClient, parentType ParentType, invID, wuID, testID, resultID, artID string, legacy bool) (string, error) {
	if parentType == ParentTypeUnknown {
		var err error
		parentType, err = ResolveParentType(invID, wuID, testID, resultID)
		if err != nil {
			return "", err
		}
	}
	if parentType == ParentTypeInvocation && !legacy && client != nil {
		art, err := ResolveInvocationArtifact(ctx, client, invID, artID, io.Discard)
		if err != nil {
			return "", err
		}
		return art.Name, nil
	}
	targetName, err := ResolveTargetResourceName(ctx, client, parentType, invID, wuID, testID, resultID, legacy)
	if err != nil {
		return "", err
	}
	return targetName + "/artifacts/" + url.PathEscape(artID), nil
}

const (
	// DefaultMaxArtifacts is the default maximum number of artifacts to return
	// and display when listing or resolving artifacts across an invocation.
	DefaultMaxArtifacts = 100

	// maxQueryArtifactsPageSize is the maximum page size sent in a single
	// QueryArtifacts RPC when fetching more than DefaultMaxArtifacts.
	maxQueryArtifactsPageSize = 500
)

// queryInvocationArtifacts pages through ResultDB.QueryArtifacts across a root
// invocation with an optional predicate, read mask, and maximum item limit (0
// for unlimited). It returns the collected artifacts and whether more artifacts
// exist beyond maxArtifacts.
func queryInvocationArtifacts(ctx context.Context, client rdbpb.ResultDBClient, invTarget string, predicate *rdbpb.ArtifactPredicate, readMask *fieldmaskpb.FieldMask, maxArtifacts int) ([]*rdbpb.Artifact, bool, error) {
	normInvID := base.NormalizeInvocation(invTarget)
	var all []*rdbpb.Artifact
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(maxQueryArtifactsPageSize)
		if maxArtifacts > 0 {
			remaining := maxArtifacts - len(all)
			if remaining <= 0 {
				break
			}
			if int32(remaining) < pageSize {
				pageSize = int32(remaining)
			}
		}

		req := &rdbpb.QueryArtifactsRequest{
			Parent:    "rootInvocations/" + normInvID,
			PageSize:  pageSize,
			PageToken: pageToken,
			Predicate: predicate,
			ReadMask:  readMask,
		}
		res, err := client.QueryArtifacts(ctx, req)
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, false, errors.Fmt("QueryArtifacts RPC failed (missing ResultDB realm permission): %w", err)
			}
			return nil, false, errors.Fmt("QueryArtifacts RPC failed: %w", err)
		}
		for i, a := range res.Artifacts {
			all = append(all, a)
			if maxArtifacts > 0 && len(all) >= maxArtifacts {
				if i < len(res.Artifacts)-1 || res.NextPageToken != "" {
					hasMore = true
				}
				break
			}
		}
		if maxArtifacts > 0 && len(all) >= maxArtifacts {
			break
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return all, hasMore, nil
}

// queryArtifactsByRegexp queries artifacts across a root invocation matching
// the given artifactIDRegexp fragment, up to DefaultMaxArtifacts.
func queryArtifactsByRegexp(ctx context.Context, client rdbpb.ResultDBClient, normInvID, artifactIDRegexp string) ([]*rdbpb.Artifact, bool, error) {
	return queryInvocationArtifacts(ctx, client, normInvID, &rdbpb.ArtifactPredicate{
		ArtifactIdRegexp: artifactIDRegexp,
	}, nil, DefaultMaxArtifacts)
}

// extractFallbackPath extracts a normalized path and trailing basename from
// artID for suffix/basename fallback matching (supporting both fetch_artifact
// path prefixes and bare filenames whose stored ArtifactId has a directory prefix).
func extractFallbackPath(artID string) (trimmedPath, baseName string, ok bool) {
	trimmedPath = strings.Trim(artID, "/")
	if trimmedPath == "" {
		return "", "", false
	}
	baseName = path.Base(trimmedPath)
	if baseName == "" || baseName == "." || baseName == ".." || baseName == "/" {
		return "", "", false
	}
	return trimmedPath, baseName, true
}

// rankPathFallbackMatches ranks candidate artifacts against a slash-delimited
// path, preferring multi-segment path suffixes over exact basename matches and
// trailing basename matches.
func rankPathFallbackMatches(candidates []*rdbpb.Artifact, trimmedPath, baseName string) []*rdbpb.Artifact {
	var suffixMatches, baseMatches, tailMatches []*rdbpb.Artifact
	for _, a := range candidates {
		if strings.Contains(a.ArtifactId, "/") && (a.ArtifactId == trimmedPath || strings.HasSuffix(trimmedPath, "/"+a.ArtifactId)) {
			suffixMatches = append(suffixMatches, a)
		} else if a.ArtifactId == baseName {
			baseMatches = append(baseMatches, a)
		} else if strings.HasSuffix(a.ArtifactId, "/"+baseName) {
			tailMatches = append(tailMatches, a)
		}
	}
	if len(suffixMatches) > 0 {
		return suffixMatches
	}
	if len(baseMatches) > 0 {
		return baseMatches
	}
	return tailMatches
}

// formatAmbiguousArtifactsError builds a numbered disambiguation error listing
// the copy-pasteable CLI flags, size, and type for each matching artifact.
func formatAmbiguousArtifactsError(matches []*rdbpb.Artifact, hasMore bool, normInvID, artID string) error {
	var b strings.Builder
	if hasMore && len(matches) >= DefaultMaxArtifacts {
		fmt.Fprintf(&b, "multiple artifacts (%d+) match %q in invocation %q (showing first %d); please re-run with one of the following flag sets to disambiguate:", len(matches), artID, normInvID, len(matches))
	} else {
		fmt.Fprintf(&b, "multiple artifacts (%d) match %q in invocation %q; please re-run with one of the following flag sets to disambiguate:", len(matches), artID, normInvID)
	}
	for i, m := range matches {
		meta := base.ExtractArtifactMetadata(m.Name)
		flags := meta.FormatFlags()
		if m.ArtifactId != artID {
			if flags != "" {
				flags += " "
			}
			flags += "-artifactid=" + base.ShellQuoteIfNeeded(m.ArtifactId)
		}
		if flags == "" {
			flags = m.Name
		}
		var details []string
		if m.SizeBytes > 0 {
			details = append(details, format.FormatSize(m.SizeBytes))
		}
		if m.ArtifactType != "" {
			details = append(details, m.ArtifactType)
		} else if m.ContentType != "" {
			details = append(details, m.ContentType)
		}
		if len(details) > 0 {
			fmt.Fprintf(&b, "\n  %d. %s (%s)", i+1, flags, strings.Join(details, ", "))
		} else {
			fmt.Fprintf(&b, "\n  %d. %s", i+1, flags)
		}
	}
	return errors.New(b.String())
}

// ResolveInvocationArtifact queries ResultDB.QueryArtifacts across a root
// invocation to resolve a single artifact by artID when -workunitid, -testid,
// and -resultid are omitted.
//
// If exact match on artID returns 0 results and artID contains '/', it falls
// back to matching the trailing filename/path suffix (supporting fetch_artifact
// paths like "/builds/I.../<bid>/<target>/inv_<n>/<artifact_name>").
// If multiple artifacts match, it returns a disambiguation error listing the
// copy-pasteable flags for each match.
func ResolveInvocationArtifact(ctx context.Context, client rdbpb.ResultDBClient, invID, artID string, errOut io.Writer) (*rdbpb.Artifact, error) {
	normInvID := base.NormalizeInvocation(invID)
	candidates, hasMore, err := queryArtifactsByRegexp(ctx, client, normInvID, regexp.QuoteMeta(artID))
	if err != nil {
		return nil, err
	}

	var matches []*rdbpb.Artifact
	for _, a := range candidates {
		if a.ArtifactId == artID {
			matches = append(matches, a)
		}
	}

	usedPathFallback := false
	if len(matches) == 0 {
		if trimmedPath, baseName, ok := extractFallbackPath(artID); ok {
			fbRegexp := "(?:.*/)?" + regexp.QuoteMeta(baseName)
			if !strings.Contains(artID, "/") {
				fbRegexp = ".+/" + regexp.QuoteMeta(baseName)
			}
			fbCandidates, fbHasMore, err := queryArtifactsByRegexp(ctx, client, normInvID, fbRegexp)
			if err != nil {
				return nil, err
			}
			matches = rankPathFallbackMatches(fbCandidates, trimmedPath, baseName)
			hasMore = fbHasMore
			usedPathFallback = len(matches) > 0
		}
	}

	if len(matches) == 0 {
		return nil, errors.Fmt("artifact %q not found in invocation %q (run 'luci artifact list -invocationid=%s' to view available artifacts)", artID, normInvID, normInvID)
	}
	if len(matches) > 1 {
		return nil, formatAmbiguousArtifactsError(matches, hasMore, normInvID, artID)
	}

	matched := matches[0]
	if usedPathFallback && errOut != nil {
		meta := base.ExtractArtifactMetadata(matched.Name)
		if flags := meta.FormatFlags(); flags != "" {
			fmt.Fprintf(errOut, "Note: resolved path %q to artifact %q (%s)\n", artID, matched.ArtifactId, flags)
		} else {
			fmt.Fprintf(errOut, "Note: resolved path %q to artifact %q\n", artID, matched.ArtifactId)
		}
	}
	return matched, nil
}

func (r *artifactCommonRun) executeFetch(ctx context.Context, parentType ParentType, fetcher artifactFetcher) int {
	if !r.ensureClients(ctx, true) {
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, r.client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch artifact: %s\n", err)
		return 1
	}
	if resolved.Legacy && parentType == ParentTypeWorkUnit {
		fmt.Fprintf(os.Stderr, "work unit artifacts are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	var art *rdbpb.Artifact
	var artName string
	if parentType == ParentTypeInvocation && !resolved.Legacy {
		art, err = ResolveInvocationArtifact(ctx, r.client, resolved.ID, r.artifactID, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
		artName = art.Name
	} else {
		artName, err = ResolveArtifactResourceName(ctx, r.client, parentType, resolved.ID, r.workUnitID, r.testID, r.resultID, r.artifactID, resolved.Legacy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
	}

	if art == nil || art.FetchUrl == "" {
		art, err = r.client.GetArtifact(ctx, &rdbpb.GetArtifactRequest{Name: artName})
		if err != nil && parentType != ParentTypeInvocation &&
			(status.Code(err) == codes.NotFound || status.Code(err) == codes.InvalidArgument) {
			if _, baseName, ok := extractFallbackPath(r.artifactID); ok && baseName != r.artifactID {
				if idx := strings.LastIndex(artName, "/artifacts/"); idx != -1 {
					fbName := artName[:idx] + "/artifacts/" + url.PathEscape(baseName)
					if fbArt, fbErr := r.client.GetArtifact(ctx, &rdbpb.GetArtifactRequest{Name: fbName}); fbErr == nil {
						fmt.Fprintf(os.Stderr, "Note: resolved path %q to artifact %q\n", r.artifactID, baseName)
						art = fbArt
						artName = fbName
						err = nil
					}
				}
			}
		}
		if err != nil {
			if base.IsPermissionDenied(err) {
				fmt.Fprintf(os.Stderr, "GetArtifact RPC failed (missing ResultDB realm permission): %s\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "GetArtifact RPC failed: %s\n", err)
			}
			return 1
		}
	}
	if art.FetchUrl == "" {
		fmt.Fprintf(os.Stderr, "artifact %q has no fetch URL\n", artName)
		return 1
	}

	var outWriter io.Writer = os.Stdout
	if r.outputFile != "" {
		f, err := os.Create(r.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create output file: %s\n", err)
			return 1
		}
		defer f.Close()
		outWriter = f
	}

	if err := fetcher(ctx, r.httpClient, art.FetchUrl, outWriter); err != nil {
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
