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

package base

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// IsPermissionDenied reports whether err has a PermissionDenied gRPC or appstatus code.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	code := status.Code(err)
	if code == codes.Unknown {
		code = appstatus.Code(err)
	}
	return code == codes.PermissionDenied
}

// LooksLikeAndroidTarget heuristically reports whether target (an invocation ID,
// work unit ID, test result ID, URL, or ResultDB resource name) appears to
// refer to Android (AnTS) data based on naming conventions.
//
// NOTE: This check is a best-effort heuristic and is NOT definitive. It must
// only be used for deciding whether to display user-facing hints in error
// messages, never for enabling, disabling, or gating functionality.
func LooksLikeAndroidTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if IsAntsURL(target) || IsAntsTestResultID(target) || IsAntsWorkUnitID(target) {
		return true
	}
	if meta := ExtractArtifactMetadata(target); meta.InvocationID != "" || meta.WorkUnitID != "" {
		if meta.InvocationID != "" && IsAntsInvocationID(NormalizeInvocation(meta.InvocationID)) {
			return true
		}
		if meta.WorkUnitID != "" && IsAntsWorkUnitID(meta.WorkUnitID) {
			return true
		}
		return false
	}
	return IsAntsInvocationID(NormalizeInvocation(target))
}

// ExtractDeniedProjectAndRealm extracts the denied LUCI project and (when
// disclosed) full realm from a ResultDB PermissionDenied error, inspecting both
// structured errdetails.ErrorInfo metadata and the error message text.
func ExtractDeniedProjectAndRealm(err error) (project, realm string) {
	if err == nil {
		return "", ""
	}
	st := status.Convert(err)
	if st.Code() == codes.Unknown {
		if appSt, ok := appstatus.Get(err); ok {
			st = appSt
		}
	}
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			project = info.Metadata["project"]
			realm = info.Metadata["realm"]
			if project == "" && realm != "" {
				if p, _, ok := strings.Cut(realm, ":"); ok {
					project = p
				}
			}
			if project != "" || realm != "" {
				return project, realm
			}
		}
	}

	msg := err.Error()
	if _, after, ok := strings.Cut(msg, `in realm "`); ok {
		if r, _, ok := strings.Cut(after, `"`); ok && r != "" {
			realm = r
			if p, _, ok := strings.Cut(realm, ":"); ok {
				project = p
			}
			return project, realm
		}
	}
	for _, marker := range []string{`(project "`, `in any realm in project "`} {
		if _, after, ok := strings.Cut(msg, marker); ok {
			if p, _, ok := strings.Cut(after, `"`); ok && p != "" {
				return p, ""
			}
		}
	}
	return "", ""
}

// ResultDBPermissionReason returns the parenthetical explanation for a ResultDB
// PermissionDenied error on target. When the error or target includes a LUCI
// project or realm, it is included in the message; when it refers to Android
// (AnTS) data, actionable MDB group onboarding instructions are appended.
func ResultDBPermissionReason(target string, errs ...error) string {
	var project, realm string
	for _, err := range errs {
		if p, r := ExtractDeniedProjectAndRealm(err); p != "" || r != "" {
			project, realm = p, r
			break
		}
	}
	reason := "missing ResultDB realm permission"
	if realm != "" {
		reason = fmt.Sprintf("missing ResultDB realm permission in realm %q", realm)
	} else if project != "" {
		reason = fmt.Sprintf("missing ResultDB realm permission in project %q", project)
	}
	if project == "android" || (project == "" && LooksLikeAndroidTarget(target)) {
		return reason + "; " + AndroidAccessHint
	}
	return reason
}

// ResolvedInvocation holds the normalized invocation ID, whether it is a legacy
// invocation, and the retrieved RootInvocation or legacy Invocation proto.
type ResolvedInvocation struct {
	ID             string
	Legacy         bool
	RootInvocation *rdbpb.RootInvocation
	Invocation     *rdbpb.Invocation
}

// ResolveInvocation normalizes rawInvID and queries ResultDB for both the
// RootInvocation and legacy Invocation in parallel to determine whether the
// target is a RootInvocation or a legacy Invocation, returning the retrieved
// invocation proto along with the resolved legacy status.
//
// If either probe returns codes.PermissionDenied, the PermissionDenied error is
// returned immediately.
func ResolveInvocation(ctx context.Context, client rdbpb.ResultDBClient, rawInvID string) (*ResolvedInvocation, error) {
	normalized := NormalizeInvocation(rawInvID)
	if client == nil || normalized == "" {
		return &ResolvedInvocation{ID: normalized}, nil
	}

	probeCtx := logging.SetLevel(ctx, logging.Error)
	var (
		rootInv   *rdbpb.RootInvocation
		rootErr   error
		legacyInv *rdbpb.Invocation
		legacyErr error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		rootInv, rootErr = client.GetRootInvocation(probeCtx, &rdbpb.GetRootInvocationRequest{
			Name: "rootInvocations/" + normalized,
		})
	}()
	go func() {
		defer wg.Done()
		legacyInv, legacyErr = client.GetInvocation(probeCtx, &rdbpb.GetInvocationRequest{
			Name: "invocations/" + normalized,
		})
	}()
	wg.Wait()

	if rootErr == nil {
		return &ResolvedInvocation{
			ID:             normalized,
			Legacy:         false,
			RootInvocation: rootInv,
			Invocation:     legacyInv,
		}, nil
	}
	if IsPermissionDenied(rootErr) {
		return nil, errors.Fmt("permission denied for rootInvocations/%s (%s): %w", normalized, ResultDBPermissionReason(normalized, rootErr), rootErr)
	}
	if legacyErr == nil {
		return &ResolvedInvocation{
			ID:         normalized,
			Legacy:     true,
			Invocation: legacyInv,
		}, nil
	}
	if IsPermissionDenied(legacyErr) {
		return nil, errors.Fmt("permission denied for invocations/%s (%s): %w", normalized, ResultDBPermissionReason(normalized, legacyErr), legacyErr)
	}

	return &ResolvedInvocation{
		ID:     normalized,
		Legacy: false,
	}, nil
}

// NormalizeInvocation strips "rootInvocations/" or "invocations/" prefix from an invocation ID.
// If the invocation ID is an AnTS invocation (I<digits> or i<digits>), it is normalized to ants-i<digits>.
// If the invocation ID starts with a digit, it is normalized to build-<id>.
func NormalizeInvocation(inv string) string {
	inv = strings.TrimSpace(inv)
	inv = strings.TrimPrefix(inv, "invocations/")
	inv = strings.TrimPrefix(inv, "rootInvocations/")
	if strings.HasPrefix(inv, "I") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "ants-i" + inv[1:]
	}
	if strings.HasPrefix(inv, "i") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "ants-i" + inv[1:]
	}
	if strings.HasPrefix(strings.ToLower(inv), "ants-i") && len(inv) > 6 && isAllDigits(inv[6:]) {
		return "ants-i" + inv[6:]
	}
	if strings.HasPrefix(inv, "b") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "build-" + inv[1:]
	}
	if !strings.HasPrefix(inv, "build-") && !strings.HasPrefix(inv, "task-") && !strings.HasPrefix(inv, "u-") && !strings.HasPrefix(inv, "ants-") {
		if _, err := url.PathUnescape(inv); err == nil && len(inv) > 0 && (inv[0] >= '0' && inv[0] <= '9') {
			inv = "build-" + inv
		}
	}
	return inv
}

// NormalizeWorkUnit strips a leading "workUnits/" prefix from a work unit ID.
// If the ID is an AnTS work unit (e.g. WU<digits> or ants-wu<digits>), it is
// normalized to lowercase "ants-wu<digits>" while preserving any optional
// attempt (-attempt__<N>) or TradeFed composite (:u-<uuid>) suffixes.
func NormalizeWorkUnit(wu string) string {
	wu = strings.TrimPrefix(strings.TrimSpace(wu), "workUnits/")
	if m := antsWURegex.FindStringSubmatch(wu); m != nil {
		return "ants-wu" + m[1] + strings.ToLower(m[2])
	}
	return wu
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// TrimResourceURL strips URL scheme/host prefix and query/hash fragments from a resource target or URL.
func TrimResourceURL(raw string) string {
	clean := strings.TrimSpace(raw)
	if idx := strings.Index(clean, "invocations/"); idx != -1 {
		clean = clean[idx:]
	} else if idx := strings.Index(clean, "rootInvocations/"); idx != -1 {
		clean = clean[idx:]
	}
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	return clean
}

// FormatTestResultResourceName formats an invocation, test ID, and result ID into a canonical resource name.
func FormatTestResultResourceName(inv, testID, resultID string) string {
	inv = NormalizeInvocation(inv)
	encodedTestID := url.PathEscape(testID)
	return "invocations/" + inv + "/tests/" + encodedTestID + "/results/" + resultID
}

// FormatTestResultWorkUnitResourceName formats a root invocation, work unit ID, test ID, and result ID into a canonical resource name.
func FormatTestResultWorkUnitResourceName(rootInv, wuID, testID, resultID string) string {
	rootInv = NormalizeInvocation(rootInv)
	wuID = NormalizeWorkUnit(wuID)
	encodedTestID := url.PathEscape(testID)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID + "/tests/" + encodedTestID + "/results/" + resultID
}

// FormatWorkUnitResourceName formats a root invocation and work unit ID into a canonical resource name.
func FormatWorkUnitResourceName(rootInv, wuID string) string {
	rootInv = NormalizeInvocation(rootInv)
	wuID = NormalizeWorkUnit(wuID)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID
}

func unescapePathSegment(seg string) string {
	if unescaped, err := url.PathUnescape(seg); err == nil {
		return unescaped
	}
	return seg
}

// ExtractTestResultComponents parses an invocation, test ID, and result ID from a resource name.
func ExtractTestResultComponents(name string) (inv, testID, resultID string) {
	meta := ExtractArtifactMetadata(name)
	return meta.InvocationID, meta.TestID, meta.ResultID
}

// ExtractWorkUnitComponents parses a root invocation and work unit ID from a resource name.
func ExtractWorkUnitComponents(name string) (rootInv, wuID string) {
	meta := ExtractArtifactMetadata(name)
	return meta.InvocationID, meta.WorkUnitID
}

// ArtifactMetadata holds the resource metadata (invocation, work unit, test ID,
// result ID, and artifact ID) parsed from an artifact or target resource name.
type ArtifactMetadata struct {
	InvocationID string
	WorkUnitID   string
	TestID       string
	ResultID     string
	ArtifactID   string
}

// ExtractArtifactMetadata parses the owning invocation, work unit, test ID,
// result ID, and artifact ID from a resource name in a single pass.
func ExtractArtifactMetadata(name string) ArtifactMetadata {
	if idx := strings.Index(name, "rootInvocations/"); idx != -1 {
		name = name[idx:]
	} else if idx := strings.Index(name, "invocations/"); idx != -1 {
		name = name[idx:]
	}
	var meta ArtifactMetadata
	parts := strings.Split(name, "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "rootInvocations", "invocations":
			meta.InvocationID = parts[i+1]
			i++
		case "workUnits":
			meta.WorkUnitID = unescapePathSegment(parts[i+1])
			i++
		case "tests":
			meta.TestID = unescapePathSegment(parts[i+1])
			i++
		case "results":
			meta.ResultID = parts[i+1]
			i++
		case "variants":
			i++
		case "artifacts":
			meta.ArtifactID = unescapePathSegment(strings.Join(parts[i+1:], "/"))
			return meta
		}
	}
	return meta
}

// ShellQuoteIfNeeded returns s single-quoted for POSIX shells if s is empty or
// contains characters unsafe to leave unquoted, or s as-is otherwise.
func ShellQuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("_@%+=:,./-", c) != -1) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// FormatFlags returns the copy-pasteable CLI flags (-workunitid, -testid,
// and/or -resultid) needed to target this artifact's owning resource.
func (m ArtifactMetadata) FormatFlags() string {
	var parts []string
	if m.WorkUnitID != "" {
		parts = append(parts, "-workunitid="+ShellQuoteIfNeeded(m.WorkUnitID))
	}
	if m.TestID != "" && m.ResultID != "" {
		parts = append(parts,
			"-testid="+ShellQuoteIfNeeded(m.TestID),
			"-resultid="+ShellQuoteIfNeeded(m.ResultID),
		)
	}
	return strings.Join(parts, " ")
}

// ParseVerdictName parses a Milo or ResultDB verdict URL/resource name into its
// invocation name, variant hash, test ID regexp, and a matching predicate.
func ParseVerdictName(name string) (invName, variantHash, testIDRegexp string, matchFunc func(*rdbpb.TestResult) bool, err error) {
	name = TrimResourceURL(name)

	varIdx := strings.Index(name, "/variants/")
	if varIdx == -1 {
		return "", "", "", nil, errors.New("invalid verdict name: missing /variants/")
	}

	afterVar := name[varIdx+len("/variants/"):]
	parts := strings.SplitN(afterVar, "/", 2)
	variantHash = parts[0]

	beforeVar := name[:varIdx]
	if modIdx := strings.Index(beforeVar, "/modules/"); modIdx != -1 {
		invName = beforeVar[:modIdx]
		caseIdx := strings.Index(name, "/cases/")
		if caseIdx != -1 {
			afterCase := name[caseIdx+len("/cases/"):]
			caseParts := strings.Split(afterCase, "/")
			caseName, err := url.PathUnescape(caseParts[0])
			if err != nil {
				return "", "", "", nil, errors.Fmt("failed to unescape case name %q: %w", caseParts[0], err)
			}
			testIDRegexp = ".*" + regexp.QuoteMeta(caseName)
			matchFunc = func(tr *rdbpb.TestResult) bool {
				if tr.VariantHash != variantHash {
					return false
				}
				if tr.TestIdStructured != nil && tr.TestIdStructured.CaseName == caseName {
					return true
				}
				return strings.HasSuffix(tr.TestId, caseName)
			}
			return invName, variantHash, testIDRegexp, matchFunc, nil
		}
	}

	if testIdx := strings.Index(beforeVar, "/tests/"); testIdx != -1 {
		invName = beforeVar[:testIdx]
		encodedTestID := beforeVar[testIdx+len("/tests/"):]
		testID, err := url.PathUnescape(encodedTestID)
		if err != nil {
			return "", "", "", nil, errors.Fmt("failed to unescape test ID %q: %w", encodedTestID, err)
		}
		testIDRegexp = regexp.QuoteMeta(testID)
		matchFunc = func(tr *rdbpb.TestResult) bool {
			return tr.VariantHash == variantHash && tr.TestId == testID
		}
		return invName, variantHash, testIDRegexp, matchFunc, nil
	}

	return "", "", "", nil, errors.New("invalid verdict name format: could not parse test ID or case name")
}

// QueryVerdictResultsAndExonerations queries test results and exonerations for a verdict by invocation ID and test ID.
// Defaults to querying as a root invocation using QueryTestVerdicts unless legacy is true.
func QueryVerdictResultsAndExonerations(ctx context.Context, client rdbpb.ResultDBClient, invID, testID, variantHash string, legacy bool, maxFetch int) ([]*rdbpb.TestResult, []*rdbpb.TestExoneration, bool, error) {
	if legacy {
		return queryLegacyInvocationVerdicts(ctx, client, invID, testID, variantHash, maxFetch)
	}
	return queryRootInvocationVerdicts(ctx, client, invID, testID, variantHash, maxFetch)
}

func queryRootInvocationVerdicts(ctx context.Context, client rdbpb.ResultDBClient, invID, testID, variantHash string, maxFetch int) ([]*rdbpb.TestResult, []*rdbpb.TestExoneration, bool, error) {
	rootInvName := "rootInvocations/" + NormalizeInvocation(invID)

	// When both a structured testID and variantHash are known, query directly by
	// TestPrefixFilter (AggregationLevel_CASE) and omit OrderBy: "ui_priority" so
	// ResultDB uses fetchUsingIteratorQuery (primary key prefix lookup) in 1 RPC.
	var testPrefixFilter *rdbpb.TestIdentifierPrefix
	if testID != "" && variantHash != "" {
		if parsed, err := pbutil.ParseAndValidateTestID(testID, pbutil.QuerySideTestIDLimitCallback); err == nil {
			testPrefixFilter = &rdbpb.TestIdentifierPrefix{
				Level: rdbpb.AggregationLevel_CASE,
				Id: &rdbpb.TestIdentifier{
					ModuleName:        parsed.ModuleName,
					ModuleScheme:      parsed.ModuleScheme,
					ModuleVariantHash: variantHash,
					CoarseName:        parsed.CoarseName,
					FineName:          parsed.FineName,
					CaseName:          parsed.CaseName,
				},
			}
		}
	}

	var verdictResults []*rdbpb.TestResult
	var exList []*rdbpb.TestExoneration
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(1000)
		if maxFetch > 0 && int32(maxFetch) < pageSize {
			pageSize = int32(maxFetch)
		}
		req := &rdbpb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			View:      rdbpb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  pageSize,
			PageToken: pageToken,
		}
		if testPrefixFilter != nil {
			req.Predicate = &rdbpb.TestVerdictPredicate{
				TestPrefixFilter: testPrefixFilter,
			}
		} else {
			req.OrderBy = "ui_priority"
			if testID != "" {
				req.Predicate = &rdbpb.TestVerdictPredicate{
					ContainsTestResultFilter: fmt.Sprintf("test_id = %q", testID),
				}
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			if IsPermissionDenied(err) {
				return nil, nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s (%s): %w", rootInvName, ResultDBPermissionReason(rootInvName, err), err)
			}
			return nil, nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s: %w", rootInvName, err)
		}
		for _, tv := range res.TestVerdicts {
			for _, tr := range tv.Results {
				if tr.TestIdStructured == nil && tv.TestIdStructured != nil {
					tr.TestIdStructured = tv.TestIdStructured
				}
				if tr.Variant == nil && tv.TestIdStructured != nil {
					tr.Variant = tv.TestIdStructured.ModuleVariant
				}
				if tr.VariantHash == "" && tv.TestIdStructured != nil {
					tr.VariantHash = tv.TestIdStructured.ModuleVariantHash
				}
				if tr.TestId == "" {
					tr.TestId = tv.TestId
				}
				if tr.TestMetadata == nil {
					tr.TestMetadata = tv.TestMetadata
				}
				if testID != "" && tr.TestId != testID {
					continue
				}
				if variantHash != "" && tr.VariantHash != variantHash {
					continue
				}
				verdictResults = append(verdictResults, tr)
			}
			for _, ex := range tv.Exonerations {
				if ex.VariantHash == "" && tv.TestIdStructured != nil {
					ex.VariantHash = tv.TestIdStructured.ModuleVariantHash
				}
				if ex.TestId == "" {
					ex.TestId = tv.TestId
				}
				if testID != "" && ex.TestId != testID {
					continue
				}
				if variantHash != "" && ex.VariantHash != variantHash {
					continue
				}
				exList = append(exList, ex)
			}
		}
		if maxFetch > 0 && len(verdictResults) >= maxFetch {
			if res.NextPageToken != "" || len(verdictResults) > maxFetch {
				hasMore = true
				verdictResults = verdictResults[:maxFetch]
			}
			break
		}
		// When querying a specific testID with OrderBy: "ui_priority", RPC #1
		// queries UnsuccessfulTestResultsV2 and unconditionally returns a synthetic
		// NextPageToken (MaxUIPriority) to scan PASSED/SKIPPED verdicts across the
		// entire table. If RPC #1 already found the verdict, stop immediately
		// rather than triggering a 15-20s full-table scan on RPC #2.
		if (testID != "" && len(verdictResults) > 0) || res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return verdictResults, exList, hasMore, nil
}

func queryLegacyInvocationVerdicts(ctx context.Context, client rdbpb.ResultDBClient, invID, testID, variantHash string, maxFetch int) ([]*rdbpb.TestResult, []*rdbpb.TestExoneration, bool, error) {
	legacyInvName := "invocations/" + NormalizeInvocation(invID)

	var verdictResults []*rdbpb.TestResult
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(1000)
		if maxFetch > 0 && int32(maxFetch) < pageSize {
			pageSize = int32(maxFetch)
		}
		req := &rdbpb.QueryTestResultsRequest{
			Invocations: []string{legacyInvName},
			PageSize:    pageSize,
			PageToken:   pageToken,
		}
		if testID != "" {
			req.Predicate = &rdbpb.TestResultPredicate{
				TestIdRegexp: regexp.QuoteMeta(testID),
			}
			if variantHash != "" {
				req.Predicate.Variant = &rdbpb.VariantPredicate{
					Predicate: &rdbpb.VariantPredicate_Contains{
						Contains: &rdbpb.Variant{
							Def: map[string]string{},
						},
					},
				}
			}
		}
		res, err := client.QueryTestResults(ctx, req)
		if err != nil {
			if IsPermissionDenied(err) {
				return nil, nil, false, errors.Fmt("QueryTestResults RPC failed for %s (%s): %w", legacyInvName, ResultDBPermissionReason(legacyInvName, err), err)
			}
			return nil, nil, false, errors.Fmt("QueryTestResults RPC failed for %s: %w", legacyInvName, err)
		}
		for _, tr := range res.TestResults {
			if testID != "" && tr.TestId != testID {
				continue
			}
			if variantHash != "" && tr.VariantHash != variantHash {
				continue
			}
			verdictResults = append(verdictResults, tr)
		}
		if maxFetch > 0 && len(verdictResults) >= maxFetch {
			if res.NextPageToken != "" || len(verdictResults) > maxFetch {
				hasMore = true
				verdictResults = verdictResults[:maxFetch]
			}
			break
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	var exList []*rdbpb.TestExoneration
	reqEx := &rdbpb.QueryTestExonerationsRequest{
		Invocations: []string{legacyInvName},
		PageSize:    1000,
	}
	if testID != "" {
		reqEx.Predicate = &rdbpb.TestExonerationPredicate{
			TestIdRegexp: regexp.QuoteMeta(testID),
		}
	}
	resEx, err := client.QueryTestExonerations(ctx, reqEx)
	if err == nil && resEx != nil {
		for _, ex := range resEx.TestExonerations {
			if testID != "" && ex.TestId != testID {
				continue
			}
			if variantHash != "" && ex.VariantHash != variantHash {
				continue
			}
			exList = append(exList, ex)
		}
	}

	return verdictResults, exList, hasMore, nil
}

// ExtractModuleNameFromTestID parses the module name from a structured test ID
// (e.g. ":ModuleName!junit:pkg:Class#Method"), returning "" if not structured.
func ExtractModuleNameFromTestID(testID string) string {
	if strings.HasPrefix(testID, ":") {
		if bangIdx := strings.Index(testID, "!"); bangIdx > 1 {
			return strings.ReplaceAll(testID[1:bangIdx], `\:`, ":")
		}
	}
	return ""
}
