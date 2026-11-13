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

package analysis

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
)

const (
	maxTestSubstringLength     = 512
	maxMatchingTestsShown      = 20
	maxDidYouMeanSuggestions   = 10
	maxOtherMatchesWithHistory = 3
	minLeafSuggestionTokenByte = 4
)

type testSubstringSearchResult struct {
	TestIDs         []string
	Truncated       bool
	CaseInsensitive bool
}

// normalizeSubRealm strips an optional "<project>:" prefix from subRealmFlag so it
// matches LUCI Analysis's bare sub_realm expectation across QueryTests, QueryVariants,
// QueryStats, and Query.
func normalizeSubRealm(project, subRealmFlag string) string {
	subRealm := strings.TrimSpace(subRealmFlag)
	if idx := strings.IndexByte(subRealm, ':'); idx >= 0 && subRealm[:idx] == project {
		subRealm = subRealm[idx+1:]
	}
	return subRealm
}

// validateTestIDSubstring checks that val is a non-empty, <= 512-byte printable UTF-8 NFC
// string accepted by LUCI Analysis's TestHistory.QueryTests RPC.
func validateTestIDSubstring(flagName, val string) error {
	if val == "" {
		return errors.Fmt("%s cannot be empty", flagName)
	}
	if err := pbutil.ValidateUTF8Printable(val, maxTestSubstringLength, pbutil.ValidationModeLoose); err != nil {
		return errors.Fmt("invalid %s: %w", flagName, err)
	}
	return nil
}

// isCanonicalTestIDInput reports whether testID came from an unambiguous canonical source
// (structured flags, a positional URL/resource target, a syntactically valid ':module!scheme:...'
// structured ID, or a '<scheme>://...' legacy test URI such as 'ninja://' or 'fuchsia-pkg://')
// as opposed to a short/partial '-testid' input (e.g. 'Suite#Case' or ':Suite#Case').
func isCanonicalTestIDInput(testID string, fromStructuredFlags, fromPositionalTarget bool) bool {
	if fromStructuredFlags || fromPositionalTarget {
		return true
	}
	if pbutil.IsStructuredTestID(testID) {
		return pbutil.ValidateTestID(testID, pbutil.QuerySideTestIDLimitCallback) == nil
	}
	return strings.Contains(testID, "://")
}

// validateMalformedStructuredTestID returns an error when testID appears to be a hand-constructed
// ':module!scheme:...' structured test ID (starts with ':' and has '!' before any '#') that fails
// ResultDB structured test ID syntax validation, rather than a short ':Suite#Case' partial query.
func validateMalformedStructuredTestID(testID string) error {
	if !pbutil.IsStructuredTestID(testID) {
		return nil
	}
	bang := strings.IndexByte(testID, '!')
	hash := strings.IndexByte(testID, '#')
	if bang < 0 || (hash >= 0 && bang > hash) {
		return nil
	}
	err := pbutil.ValidateTestID(testID, pbutil.QuerySideTestIDLimitCallback)
	if err == nil {
		return nil
	}
	strippedMod := strings.ReplaceAll(strings.ReplaceAll(testID[1:bang], `\\`, ""), `\:`, "")
	if strings.Contains(strippedMod, ":") {
		return errors.Fmt("invalid structured -testid: %w (hint: escape ':' in module names as '\\:', or pass -modulename, -scheme, -case)", err)
	}
	return errors.Fmt("invalid structured -testid: %w", err)
}

// queryMatchingTests searches LUCI Analysis's 90-day test_realms index via TestHistory.QueryTests,
// retrying once with CaseInsensitive: true if an exact-case query returns zero matches, and
// falling back to a structured-delimiter rewrite (e.g. 'pkg:Class#method' for 'pkg.Class#method',
// 'Suite#Case' for 'Suite.Case', or '\:\:' for C++ '::') when test_metadata.name is unpopulated
// or uses a different separator than test_id.
func queryMatchingTests(ctx context.Context, client analysispb.TestHistoryClient, project, subRealm, substring string, caseInsensitive bool) (*testSubstringSearchResult, error) {
	res, err := queryTestsWithOptionalCaseRetry(ctx, client, project, subRealm, substring, caseInsensitive)
	if err != nil || len(res.TestIDs) > 0 {
		return res, err
	}
	if alt := alternateStructuredSubstring(substring); alt != "" && validateTestIDSubstring("-test-substring", alt) == nil {
		altRes, altErr := queryTestsWithOptionalCaseRetry(ctx, client, project, subRealm, alt, caseInsensitive)
		if altErr != nil || len(altRes.TestIDs) > 0 {
			return altRes, altErr
		}
	}
	return res, nil
}

func queryTestsWithOptionalCaseRetry(ctx context.Context, client analysispb.TestHistoryClient, project, subRealm, substring string, caseInsensitive bool) (*testSubstringSearchResult, error) {
	req := &analysispb.QueryTestsRequest{
		Project:         project,
		TestIdSubstring: substring,
		SubRealm:        subRealm,
		PageSize:        testHistoryPageSize,
		CaseInsensitive: caseInsensitive,
	}
	resp, err := client.QueryTests(ctx, req)
	if err != nil {
		return nil, errors.Fmt("TestHistory.QueryTests RPC failed for %s / %q: %w", project, substring, err)
	}
	if len(resp.GetTestIds()) == 0 && !caseInsensitive {
		req.CaseInsensitive = true
		retryResp, retryErr := client.QueryTests(ctx, req)
		if retryErr != nil {
			return nil, errors.Fmt("TestHistory.QueryTests RPC failed for %s / %q: %w", project, substring, retryErr)
		}
		if len(retryResp.GetTestIds()) > 0 {
			resp = retryResp
			caseInsensitive = true
		}
	}
	return &testSubstringSearchResult{
		TestIDs:         resp.GetTestIds(),
		Truncated:       resp.GetNextPageToken() != "",
		CaseInsensitive: caseInsensitive,
	}, nil
}

// alternateStructuredSubstring rewrites common display test substrings ('pkg.Class#method',
// 'Class.method', 'pkg.Class.method', or 'ns::Suite#Case') into their encoded structured
// test_id substring equivalents ('pkg:Class#method', 'Class#method', 'ns\:\:Suite#Case')
// for harnesses that omit test_metadata.name or use '#' instead of '.' in test_metadata.name.
func alternateStructuredSubstring(s string) string {
	if s == "" || strings.HasPrefix(s, ":") || strings.HasPrefix(s, "ninja:") || strings.Contains(s, "!") || strings.Contains(s, "://") {
		return ""
	}
	if strings.Contains(s, "::") && !strings.Contains(s, `\:`) {
		escaped := strings.ReplaceAll(s, "::", `\:\:`)
		if !strings.Contains(escaped, "#") && !strings.Contains(escaped, "/") {
			if dot := strings.LastIndexByte(escaped, '.'); dot > 0 && dot < len(escaped)-1 && !isCommonTestFileExtension(escaped[dot+1:]) {
				escaped = escaped[:dot] + "#" + escaped[dot+1:]
			}
		}
		return escaped
	}
	if hashIdx := strings.IndexByte(s, '#'); hashIdx >= 0 {
		if hashIdx > 0 {
			beforeHash := s[:hashIdx]
			if !strings.ContainsAny(beforeHash, ":/") {
				if pkgSuite, ok := replaceLastDotWithColon(beforeHash); ok {
					return pkgSuite + s[hashIdx:]
				}
			}
		}
		return ""
	}
	if strings.ContainsAny(s, ":/") || strings.HasPrefix(strings.ToLower(s), "tast.") {
		return ""
	}
	lastDot := strings.LastIndexByte(s, '.')
	if lastDot <= 0 || lastDot >= len(s)-1 {
		return ""
	}
	method := s[lastDot+1:]
	if isCommonTestFileExtension(method) {
		return ""
	}
	beforeMethod, _ := replaceLastDotWithColon(s[:lastDot])
	return beforeMethod + "#" + method
}

func replaceLastDotWithColon(s string) (string, bool) {
	if dot := strings.LastIndexByte(s, '.'); dot > 0 && dot < len(s)-1 {
		return s[:dot] + ":" + s[dot+1:], true
	}
	return s, false
}

func isCommonTestFileExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case "html", "htm", "xhtml", "js", "mjs", "cjs", "ts", "tsx", "py", "cm", "json", "xml", "java", "cc", "cpp", "mm", "rs", "go", "sh":
		return true
	default:
		return false
	}
}

// rankAndResolveSubstringCandidates deduplicates and ranks candidate test IDs (bounded-suffix
// matches first, then structured ':' IDs before legacy IDs, then alphabetical) and returns
// an auto-resolved canonical test ID when the match is unambiguous and not page-truncated.
func rankAndResolveSubstringCandidates(candidates []string, query string, caseInsensitive, truncated bool) (resolvedID string, ranked []string) {
	deduped := dedupeNonEmptyStrings(candidates)
	if len(deduped) == 0 {
		return "", nil
	}

	boundedCache := make(map[string]bool, len(deduped))
	for _, id := range deduped {
		boundedCache[id] = isBoundedSuffixMatch(id, query, caseInsensitive)
	}

	sort.Slice(deduped, func(i, j int) bool {
		bi, bj := boundedCache[deduped[i]], boundedCache[deduped[j]]
		if bi != bj {
			return bi
		}
		si, sj := pbutil.IsStructuredTestID(deduped[i]), pbutil.IsStructuredTestID(deduped[j])
		if si != sj {
			return si
		}
		return deduped[i] < deduped[j]
	})

	if truncated {
		return "", deduped
	}
	return resolveUnambiguousCandidate(deduped, boundedCache), deduped
}

func dedupeNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		deduped = append(deduped, v)
	}
	return deduped
}

func resolveUnambiguousCandidate(ranked []string, boundedCache map[string]bool) string {
	if len(ranked) == 1 {
		return ranked[0]
	}
	var bounded []string
	for _, id := range ranked {
		if boundedCache[id] {
			bounded = append(bounded, id)
		}
	}
	if len(bounded) == 1 {
		return bounded[0]
	}
	if len(bounded) < 2 {
		return ""
	}
	var structuredBounded []string
	var legacyBounded []string
	for _, id := range bounded {
		if pbutil.IsStructuredTestID(id) {
			structuredBounded = append(structuredBounded, id)
		} else {
			legacyBounded = append(legacyBounded, id)
		}
	}
	if len(structuredBounded) != 1 || len(legacyBounded) == 0 {
		return ""
	}
	for _, leg := range legacyBounded {
		if !isLegacyCounterpartOfStructured(leg, structuredBounded[0]) {
			return ""
		}
	}
	return structuredBounded[0]
}

// isBoundedSuffixMatch reports whether testID (or its unescaped structured suite/case display
// forms such as '<FineName>.<CaseName>' for GTest or '<CoarseName>.<FineName>#<CaseName>' for JUnit)
// ends with query on a '#./:!$' token boundary.
func isBoundedSuffixMatch(testID, query string, caseInsensitive bool) bool {
	if query == "" {
		return false
	}
	if hasRawBoundedSuffix(testID, query, caseInsensitive) {
		return true
	}
	if !pbutil.IsStructuredTestID(testID) {
		return isLegacyNinjaParameterizedGTestMatch(testID, query, caseInsensitive)
	}
	parsed, err := pbutil.ParseAndValidateTestID(testID, pbutil.QuerySideTestIDLimitCallback)
	if err != nil {
		return false
	}
	for _, c := range structuredDisplayCandidates(parsed) {
		if hasDisplayBoundedSuffix(c, query, caseInsensitive, 0) {
			return true
		}
	}
	// Match raw GTest parameterized display names (e.g. 'All/Suite.Case/0' or 'All/Suite/0.Case')
	// rewritten by result_adapter into FineName='Suite', CaseName='Case/All.0' (or 'Case/0').
	if parsed.ModuleScheme == "gtest" && parsed.FineName != "" && parsed.FineName != "-" {
		unescapedCase := unescapeStructuredCase(parsed.CaseName)
		return isParameterizedGTestDisplayMatch(parsed.FineName, unescapedCase, query, caseInsensitive)
	}
	return false
}

// structuredDisplayCandidates builds the unescaped display-form strings for a structured
// test ID so callers can check suffix boundaries uniformly across schemes and field depths.
func structuredDisplayCandidates(parsed pbutil.BaseTestIdentifier) []string {
	caseParts := splitStructuredCaseComponents(parsed.CaseName)
	unescapedCase := strings.Join(caseParts, ":")
	caseForms := []string{unescapedCase}

	candidates := []string{unescapedCase, "#" + unescapedCase}
	if len(caseParts) > 1 {
		dotCase := strings.Join(caseParts, ".")
		caseForms = append(caseForms, dotCase)
		candidates = append(candidates, dotCase, "#"+dotCase)
		for k := 1; k < len(caseParts); k++ {
			candidates = append(candidates, ":"+strings.Join(caseParts[k:], ":"))
		}
	}

	hasFine := parsed.FineName != "" && parsed.FineName != "-"
	if hasFine {
		for _, c := range caseForms {
			candidates = append(candidates,
				parsed.FineName+"#"+c,
				parsed.FineName+"."+c,
				":"+parsed.FineName+"#"+c,
				":"+parsed.FineName+"."+c,
			)
		}
		candidates = append(candidates,
			parsed.FineName+"/"+unescapedCase,
			parsed.FineName+":"+unescapedCase,
		)
		if parsed.ModuleScheme == "tast" {
			candidates = append(candidates, "tast."+parsed.FineName+"."+strings.Join(caseParts, "."))
		}
	}

	if parsed.CoarseName == "" {
		return candidates
	}
	coarseTrimmed := strings.TrimSuffix(parsed.CoarseName, "/")
	if !hasFine {
		for _, c := range caseForms {
			candidates = append(candidates,
				parsed.CoarseName+"."+c,
				coarseTrimmed+"/"+c,
			)
		}
		return candidates
	}

	coarseSlashFine := coarseTrimmed + "/" + parsed.FineName
	for _, c := range caseForms {
		candidates = append(candidates,
			parsed.CoarseName+"."+parsed.FineName+"#"+c,
			coarseSlashFine+"#"+c,
		)
	}
	candidates = append(candidates,
		parsed.CoarseName+":"+parsed.FineName+"#"+unescapedCase,
		parsed.CoarseName+":"+parsed.FineName+":"+unescapedCase,
		parsed.CoarseName+"."+parsed.FineName+"."+unescapedCase,
		coarseSlashFine+"/"+unescapedCase,
		coarseSlashFine+"."+unescapedCase,
		":"+parsed.CoarseName+":"+parsed.FineName+"#"+unescapedCase,
		":"+parsed.CoarseName+"."+parsed.FineName+"#"+unescapedCase,
	)
	if len(caseParts) > 1 {
		dotCase := caseForms[1]
		candidates = append(candidates,
			parsed.CoarseName+"."+parsed.FineName+"."+dotCase,
			coarseSlashFine+"/"+dotCase,
		)
	}
	return candidates
}

type parameterizedGTestForm struct {
	display       string
	trailingToken string
}

// parameterizedGTestDisplayForms returns raw GTest launcher display names ('Inst/Suite.Case/Param',
// 'Inst/Suite/Param.Case', and 'Suite/Param.Case') for result_adapter-rewritten gtest cases.
func parameterizedGTestDisplayForms(suite, unescapedCase string) []parameterizedGTestForm {
	baseCase, paramPart, ok := strings.Cut(unescapedCase, "/")
	if !ok || baseCase == "" || paramPart == "" || strings.Contains(paramPart, "/") {
		return nil
	}
	if inst, param, hasInst := strings.Cut(paramPart, "."); hasInst && inst != "" && param != "" {
		return []parameterizedGTestForm{
			{display: inst + "/" + suite + "." + baseCase + "/" + param, trailingToken: param},
			{display: inst + "/" + suite + "/" + param + "." + baseCase, trailingToken: baseCase},
		}
	}
	return []parameterizedGTestForm{
		{display: suite + "/" + paramPart + "." + baseCase, trailingToken: baseCase},
	}
}

// isParameterizedGTestDisplayMatch checks raw GTest launcher display forms while requiring query
// to extend across the rewritten '/' or '.' parameter boundary so bare 'Case', '.Case', or '/Param'
// queries do not falsely treat a single parameter instantiation as an exact leaf match.
func isParameterizedGTestDisplayMatch(suite, unescapedCase, query string, caseInsensitive bool) bool {
	for _, form := range parameterizedGTestDisplayForms(suite, unescapedCase) {
		token := form.trailingToken
		if caseInsensitive {
			token = strings.ToLower(token)
		}
		// Require query to extend past "<sep><trailingToken>" (len(token)+2 bytes).
		if hasDisplayBoundedSuffix(form.display, query, caseInsensitive, len(token)+2) {
			return true
		}
	}
	return false
}

func isLegacyNinjaParameterizedGTestMatch(testID, query string, caseInsensitive bool) bool {
	if !strings.HasPrefix(testID, "ninja:") {
		return false
	}
	afterNinja := strings.TrimPrefix(testID, "ninja:")
	paramSlash := strings.LastIndexByte(afterNinja, '/')
	if paramSlash <= 0 {
		return false
	}
	beforeParam := afterNinja[:paramSlash]
	paramPart := afterNinja[paramSlash+1:]
	if paramPart == "" {
		return false
	}
	modSlash := strings.LastIndexByte(beforeParam, '/')
	if modSlash <= 0 {
		return false
	}
	suiteAndCase := beforeParam[modSlash+1:]
	dotIdx := strings.LastIndexByte(suiteAndCase, '.')
	if dotIdx <= 0 || dotIdx >= len(suiteAndCase)-1 {
		return false
	}
	suite := suiteAndCase[:dotIdx]
	baseCase := suiteAndCase[dotIdx+1:]
	return isParameterizedGTestDisplayMatch(suite, baseCase+"/"+paramPart, query, caseInsensitive)
}

func unescapeStructuredCase(caseName string) string {
	return strings.Join(splitStructuredCaseComponents(caseName), ":")
}

// splitStructuredCaseComponents splits a parsed structured CaseName (where pbutil
// leaves '\\' and '\:' escaped and uses unescaped ':' as the multi-component separator)
// into its unescaped component strings.
func splitStructuredCaseComponents(caseName string) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(caseName); i++ {
		if caseName[i] == '\\' && i+1 < len(caseName) {
			cur.WriteByte(caseName[i+1])
			i++
			continue
		}
		if caseName[i] == ':' {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(caseName[i])
	}
	parts = append(parts, cur.String())
	return parts
}

// hasRawBoundedSuffix reports whether raw encoded testID ends with suffix on a
// '#./:!$' token boundary, respecting ResultDB '\' escapes before '#', '!', and ':'.
func hasRawBoundedSuffix(target, suffix string, caseInsensitive bool) bool {
	if suffix == "" {
		return false
	}
	if caseInsensitive {
		target = strings.ToLower(target)
		suffix = strings.ToLower(suffix)
	}
	if !strings.HasSuffix(target, suffix) {
		return false
	}
	if len(target) == len(suffix) {
		return true
	}
	prefix := target[:len(target)-len(suffix)]
	if len(suffix) > 1 && isTestIDBoundaryByte(suffix[0]) {
		if (suffix[0] == '#' || suffix[0] == '!' || suffix[0] == ':') && hasOddTrailingBackslashes(prefix) {
			return false
		}
		return true
	}
	return isTestIDBoundaryByte(prefix[len(prefix)-1])
}

// hasDisplayBoundedSuffix reports whether an already-unescaped display string ends
// with suffix on a '#./:!$' token boundary and has length >= minSuffixBytes.
// Leading structural delimiters ('!', '#', ':') are never matched mid-string inside
// unescaped component values (where '\!', '\#', '\:' have already been decoded).
func hasDisplayBoundedSuffix(display, suffix string, caseInsensitive bool, minSuffixBytes int) bool {
	if suffix == "" {
		return false
	}
	if caseInsensitive {
		display = strings.ToLower(display)
		suffix = strings.ToLower(suffix)
	}
	if len(suffix) < minSuffixBytes {
		return false
	}
	if !strings.HasSuffix(display, suffix) {
		return false
	}
	switch suffix[0] {
	case '!':
		return false
	case '#', ':':
		return len(display) == len(suffix)
	}
	if len(display) == len(suffix) {
		return true
	}
	prefix := display[:len(display)-len(suffix)]
	return hasBoundaryBeforeSuffix(prefix, suffix)
}

func hasBoundaryBeforeSuffix(prefix, suffix string) bool {
	if len(suffix) > 1 && isTestIDBoundaryByte(suffix[0]) {
		return true
	}
	return isTestIDBoundaryByte(prefix[len(prefix)-1])
}

func hasOddTrailingBackslashes(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

func isTestIDBoundaryByte(b byte) bool {
	switch b {
	case '#', '.', '/', ':', '!', '$':
		return true
	default:
		return false
	}
}

// isLegacyCounterpartOfStructured reports whether legacyID is the pre-structured 'ninja:<module>/...'
// counterpart of structuredID for the same module, coarse package/dir, fine suite/class, and case.
func isLegacyCounterpartOfStructured(legacyID, structuredID string) bool {
	if !strings.HasPrefix(legacyID, "ninja:") {
		return false
	}
	parsed, err := pbutil.ParseAndValidateTestID(structuredID, pbutil.QuerySideTestIDLimitCallback)
	if err != nil || parsed.ModuleName == "" || parsed.ModuleName == pbutil.LegacyModuleName {
		return false
	}
	if parsed.CaseName == "" || parsed.CaseName == pbutil.FixtureCaseName {
		return false
	}
	afterNinja := strings.TrimPrefix(legacyID, "ninja:")
	prefix := parsed.ModuleName + "/"
	if !strings.HasPrefix(afterNinja, prefix) {
		return false
	}
	rest := afterNinja[len(prefix):]
	unescapedCase := unescapeStructuredCase(parsed.CaseName)
	return matchesLegacyNinjaRest(rest, parsed, unescapedCase)
}

func matchesLegacyNinjaRest(rest string, parsed pbutil.BaseTestIdentifier, unescapedCase string) bool {
	if parsed.CoarseName == "" && (parsed.FineName == "" || parsed.FineName == "-") {
		return rest == unescapedCase
	}
	coarseTrimmed := strings.TrimSuffix(parsed.CoarseName, "/")
	if parsed.FineName == "" || parsed.FineName == "-" {
		return rest == parsed.CoarseName+"."+unescapedCase || rest == coarseTrimmed+"/"+unescapedCase
	}
	if parsed.CoarseName == "" {
		forms := []string{
			parsed.FineName + "." + unescapedCase,
			parsed.FineName + "/" + unescapedCase,
			parsed.FineName + "#" + unescapedCase,
			parsed.FineName + ":" + unescapedCase,
		}
		if slices.Contains(forms, rest) {
			return true
		}
		if parsed.ModuleScheme == "gtest" {
			for _, form := range parameterizedGTestDisplayForms(parsed.FineName, unescapedCase) {
				if rest == form.display {
					return true
				}
			}
		}
		return false
	}
	coarseSlashFine := coarseTrimmed + "/" + parsed.FineName
	forms := []string{
		parsed.CoarseName + "." + parsed.FineName + "#" + unescapedCase,
		parsed.CoarseName + "." + parsed.FineName + "." + unescapedCase,
		coarseSlashFine + "/" + unescapedCase,
		coarseSlashFine + "." + unescapedCase,
		coarseSlashFine + "#" + unescapedCase,
		parsed.CoarseName + ":" + parsed.FineName + "#" + unescapedCase,
		parsed.CoarseName + ":" + parsed.FineName + ":" + unescapedCase,
	}
	return slices.Contains(forms, rest)
}

// fetchLeafSuggestions runs a best-effort QueryTests search using the trailing case/leaf token
// of query (e.g. 'MyCase' from 'WrongSuite#MyCase' or ':mod!gtest::Suite#MyCase') to populate
// 'Did you mean' suggestions without ever auto-resolving.
func fetchLeafSuggestions(ctx context.Context, client analysispb.TestHistoryClient, project, subRealm, query string, caseInsensitive bool) []string {
	leaf := extractLeafSuggestionToken(query)
	if leaf == "" || validateTestIDSubstring("-testid", leaf) != nil {
		return nil
	}
	res, err := queryMatchingTests(ctx, client, project, subRealm, leaf, caseInsensitive)
	if err != nil || res == nil || len(res.TestIDs) == 0 {
		return nil
	}
	_, ranked := rankAndResolveSubstringCandidates(res.TestIDs, leaf, res.CaseInsensitive, res.Truncated)
	filtered := make([]string, 0, len(ranked))
	for _, id := range ranked {
		if id != query {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func extractLeafSuggestionToken(query string) string {
	if pbutil.IsStructuredTestID(query) {
		if parsed, err := pbutil.ParseAndValidateTestID(query, pbutil.QuerySideTestIDLimitCallback); err == nil {
			if parsed.CaseName == pbutil.FixtureCaseName {
				if parsed.FineName != "" && parsed.FineName != "-" && len(parsed.FineName) >= minLeafSuggestionTokenByte {
					return parsed.FineName
				}
				return ""
			}
			parts := splitStructuredCaseComponents(parsed.CaseName)
			leaf := strings.Join(parts, ".")
			if len(parts) > 1 && parsed.ModuleScheme != "tast" {
				leaf = parts[len(parts)-1]
			}
			if leaf != "" && leaf != query && len(leaf) >= minLeafSuggestionTokenByte {
				return leaf
			}
			return ""
		}
	}
	var leaf string
	if idx := strings.LastIndexByte(query, '#'); idx >= 0 {
		leaf = query[idx+1:]
	} else if idx := strings.LastIndexAny(query, "/:"); idx >= 0 {
		leaf = query[idx+1:]
		if dot := strings.LastIndexByte(leaf, '.'); dot >= 0 {
			if cand := leaf[dot+1:]; !isCommonTestFileExtension(cand) && len(cand) >= minLeafSuggestionTokenByte {
				leaf = cand
			}
		}
	} else if idx := strings.LastIndexByte(query, '.'); idx >= 0 {
		cand := query[idx+1:]
		if !isCommonTestFileExtension(cand) {
			leaf = cand
		}
	}
	if leaf == "" || leaf == query || len(leaf) < minLeafSuggestionTokenByte {
		return ""
	}
	return leaf
}

func populateAlternateMatches(
	report *testHistoryReport,
	ranked []string,
	project, subRealm string,
	variants []string,
	daysExplicit bool,
	days int,
	limitExplicit bool,
	limit int,
) {
	if report == nil || len(ranked) <= 1 {
		return
	}
	var others []string
	var legacyCounterparts []string
	for _, id := range ranked {
		if id == report.TestID {
			continue
		}
		if isLegacyCounterpartOfStructured(id, report.TestID) {
			legacyCounterparts = append(legacyCounterparts, id)
		} else {
			others = append(others, id)
		}
	}
	others = append(others, legacyCounterparts...)
	if len(others) > 0 {
		report.DidYouMeanTestIDs = others
		report.DidYouMeanNextStep = formatTestHistoryReRunCommand(project, others[0], subRealm, variants, daysExplicit, days, limitExplicit, limit)
	}
}

func formatTestHistoryReRunCommand(
	project, testID, subRealm string,
	variants []string,
	daysExplicit bool,
	days int,
	limitExplicit bool,
	limit int,
) string {
	parts := []string{"luci", "analysis", "test-history", "-project", base.ShellQuoteIfNeeded(project)}
	if subRealm != "" {
		parts = append(parts, "-subrealm", base.ShellQuoteIfNeeded(subRealm))
	}
	for _, v := range variants {
		parts = append(parts, "-variant", base.ShellQuoteIfNeeded(v))
	}
	if daysExplicit {
		parts = append(parts, "-days", fmt.Sprintf("%d", days))
	}
	if limitExplicit {
		parts = append(parts, "-limit", fmt.Sprintf("%d", limit))
	}
	parts = append(parts, "-testid", base.ShellQuoteIfNeeded(testID))
	return strings.Join(parts, " ")
}

func printSubstringSearchHeader(out io.Writer, project, subRealm, query string) {
	fmt.Fprintf(out, "Project:   %s\n", project)
	if subRealm != "" {
		fmt.Fprintf(out, "SubRealm:  %s\n", subRealm)
	}
	fmt.Fprintf(out, "Query:     %q\n", query)
}

func printNoSubstringMatches(out io.Writer, project, subRealm, query string, suggestions []string, nextStep string) {
	printSubstringSearchHeader(out, project, subRealm, query)
	fmt.Fprintf(out, "\nNo matching test IDs found in the last 90 days.\n")
	printDidYouMeanSuggestions(out, "Did you mean one of these test IDs?", suggestions, maxDidYouMeanSuggestions, nextStep)
}

func printSubstringDisambiguation(
	out io.Writer,
	project, subRealm, query string,
	ranked []string,
	truncated bool,
	variants []string,
	daysExplicit bool,
	days int,
	limitExplicit bool,
	limit int,
) {
	if len(ranked) == 0 {
		return
	}
	printSubstringSearchHeader(out, project, subRealm, query)

	countDesc := fmt.Sprintf("%d", len(ranked))
	if truncated {
		countDesc = fmt.Sprintf("%d+, page cap reached", len(ranked))
	}
	heading := fmt.Sprintf("Multiple matching test IDs found (%s); specify exact -testid to view history:", countDesc)
	printCappedTestIDList(out, heading, ranked, maxMatchingTestsShown, " (pass a more specific -test-substring to narrow)")

	nextCmd := formatTestHistoryReRunCommand(project, ranked[0], subRealm, variants, daysExplicit, days, limitExplicit, limit)
	fmt.Fprintf(out, "\nNext step:\n  %s\n", nextCmd)
}

func printDidYouMeanSuggestions(out io.Writer, heading string, suggestions []string, maxShown int, nextStep string) {
	if len(suggestions) == 0 {
		return
	}
	printCappedTestIDList(out, heading, suggestions, maxShown, "")
	if nextStep != "" {
		fmt.Fprintf(out, "\nNext step:\n  %s\n", nextStep)
	}
}

func printCappedTestIDList(out io.Writer, heading string, testIDs []string, maxShown int, overflowSuffix string) {
	if len(testIDs) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", heading)
	shown := testIDs
	extra := 0
	if maxShown > 0 && len(shown) > maxShown {
		extra = len(shown) - maxShown
		shown = shown[:maxShown]
	}
	for _, id := range shown {
		fmt.Fprintf(out, "- %s\n", id)
	}
	if extra > 0 {
		fmt.Fprintf(out, "  ... +%d more matching test IDs%s\n", extra, overflowSuffix)
	}
}
