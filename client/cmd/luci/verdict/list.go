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

package verdict

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	defaultMaxVerdicts = 100
	maxLegacyScanPages = 10
)

func ListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-invocationid <invocation_id>] [<target>]",
		ShortDesc: "List test verdicts in an invocation",
		LongDesc: "List test verdicts in an invocation in UI priority order.\n\n" +
			"By default, only failed or execution errored verdicts are printed (up to 100 verdicts, capped within 9.5 KB).\n" +
			"Flags can be used to include other verdict statuses (e.g. -flaky, -exonerated, -passed, -all-statuses),\n" +
			"filter by module or test substring (-modulename, -filter), and bypass the default output cap (-max-verdicts 200, -all).\n\n" +
			"Works with both root invocations and legacy invocations.\n\n" +
			"Examples:\n" +
			"  $ luci verdict list https://ci.chromium.org/ui/b/8765432109876543210\n" +
			"  $ luci verdict list -invocationid build-123",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictListRun{af: af, maxVerdicts: defaultMaxVerdicts}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.moduleName, "modulename", "", "Filter verdicts to a specific module name")
			r.Flags.StringVar(&r.moduleName, "module-name", "", "Alias for -modulename")
			r.Flags.StringVar(&r.moduleName, "module", "", "Alias for -modulename")
			r.Flags.StringVar(&r.filter, "filter", "", "Filter verdicts by test result filter expression or substring (AIP-160)")
			r.Flags.StringVar(&r.filter, "testfilter", "", "Alias for -filter")
			r.Flags.StringVar(&r.filter, "test-filter", "", "Alias for -filter")
			r.Flags.IntVar(&r.maxVerdicts, "max-verdicts", defaultMaxVerdicts, "Maximum number of verdicts to display (default: 100, 0 for all; setting explicitly bypasses 9.5 KB cap)")
			r.Flags.IntVar(&r.maxVerdicts, "limit", defaultMaxVerdicts, "Alias for -max-verdicts")
			r.Flags.IntVar(&r.maxVerdicts, "n", defaultMaxVerdicts, "Alias for -max-verdicts")
			r.Flags.BoolVar(&r.allVerdicts, "all", false, "Show all verdicts without truncation")
			r.Flags.StringVar(&r.rawStatus, "status", "", "Comma-separated list of verdict statuses to include (e.g. failed,flaky,exonerated,passed,skipped,precluded,all)")
			r.Flags.BoolVar(&r.includeExonerated, "exonerated", false, "Include exonerated verdicts")
			r.Flags.BoolVar(&r.includeExonerated, "include-exonerated", false, "Include exonerated verdicts (alias)")
			r.Flags.BoolVar(&r.includeFlaky, "flaky", false, "Include flaky verdicts")
			r.Flags.BoolVar(&r.includeFlaky, "include-flaky", false, "Include flaky verdicts (alias)")
			r.Flags.BoolVar(&r.includePassed, "passed", false, "Include passed verdicts")
			r.Flags.BoolVar(&r.includePassed, "include-passed", false, "Include passed verdicts (alias)")
			r.Flags.BoolVar(&r.includeSkipped, "skipped", false, "Include skipped verdicts")
			r.Flags.BoolVar(&r.includeSkipped, "include-skipped", false, "Include skipped verdicts (alias)")
			r.Flags.BoolVar(&r.includePrecluded, "precluded", false, "Include precluded verdicts")
			r.Flags.BoolVar(&r.includePrecluded, "include-precluded", false, "Include precluded verdicts (alias)")
			r.Flags.BoolVar(&r.allStatuses, "all-statuses", false, "Include all verdict statuses")
			// -legacy is now unused and will be removed in the future.
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type verdictListRun struct {
	subcommands.CommandRunBase
	af                *base.AuthFlags
	client            pb.ResultDBClient
	out               io.Writer
	host              string
	invocationID      string
	moduleName        string
	filter            string
	maxVerdicts       int
	allVerdicts       bool
	rawStatus         string
	includeExonerated bool
	includeFlaky      bool
	includePassed     bool
	includeSkipped    bool
	includePrecluded  bool
	allStatuses       bool
	legacy            bool // Unused; will be removed in the future.
}

type statusFilter struct {
	failed           bool
	executionErrored bool
	precluded        bool
	flaky            bool
	exonerated       bool
	skipped          bool
	passed           bool
	moduleName       string
	containsFilter   string
}

func (f statusFilter) All() bool {
	return f.failed && f.executionErrored && f.precluded && f.flaky && f.exonerated && f.skipped && f.passed
}

func (f statusFilter) hasDefaultStatuses() bool {
	return f.failed && f.executionErrored && !f.precluded && !f.flaky && !f.exonerated && !f.skipped && !f.passed
}

func (f statusFilter) hasScopeFilter() bool {
	return f.moduleName != "" || f.containsFilter != ""
}

func (f statusFilter) IsDefault() bool {
	return f.hasDefaultStatuses() && !f.hasScopeFilter()
}

func (f statusFilter) formatCLIFlags() string {
	var parts []string
	if f.All() {
		parts = append(parts, "-all-statuses")
	} else if !f.hasDefaultStatuses() {
		if f.failed && f.executionErrored {
			if f.precluded {
				parts = append(parts, "-precluded")
			}
			if f.flaky {
				parts = append(parts, "-flaky")
			}
			if f.exonerated {
				parts = append(parts, "-exonerated")
			}
			if f.skipped {
				parts = append(parts, "-skipped")
			}
			if f.passed {
				parts = append(parts, "-passed")
			}
		} else {
			var statuses []string
			if f.failed {
				statuses = append(statuses, "failed")
			}
			if f.executionErrored {
				statuses = append(statuses, "execution_errored")
			}
			if f.precluded {
				statuses = append(statuses, "precluded")
			}
			if f.flaky {
				statuses = append(statuses, "flaky")
			}
			if f.exonerated {
				statuses = append(statuses, "exonerated")
			}
			if f.skipped {
				statuses = append(statuses, "skipped")
			}
			if f.passed {
				statuses = append(statuses, "passed")
			}
			if len(statuses) > 0 {
				parts = append(parts, "-status="+strings.Join(statuses, ","))
			}
		}
	}
	if f.moduleName != "" {
		parts = append(parts, "-modulename="+base.ShellQuoteIfNeeded(f.moduleName))
	}
	if f.containsFilter != "" {
		parts = append(parts, "-filter="+base.ShellQuoteIfNeeded(f.containsFilter))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func (f statusFilter) buildContainsTestResultFilter() string {
	var parts []string
	if f.moduleName != "" {
		parts = append(parts, fmt.Sprintf("test_id_structured.module_name = %q", f.moduleName))
	}
	if f.containsFilter != "" {
		if len(parts) > 0 {
			parts = append(parts, fmt.Sprintf("(%s)", f.containsFilter))
		} else {
			parts = append(parts, f.containsFilter)
		}
	}
	return strings.Join(parts, " AND ")
}

func (f statusFilter) toEffectiveStatuses() []pb.VerdictEffectiveStatus {
	if f.All() {
		return nil
	}
	var res []pb.VerdictEffectiveStatus
	if f.failed {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED)
	}
	if f.executionErrored {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED)
	}
	if f.precluded {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PRECLUDED)
	}
	if f.flaky {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY)
	}
	if f.exonerated {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED)
	}
	if f.skipped {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_SKIPPED)
	}
	if f.passed {
		res = append(res, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED)
	}
	return res
}

func (f statusFilter) Matches(status pb.TestVerdict_Status, statusOverride pb.TestVerdict_StatusOverride, hasExonerations bool) bool {
	if f.All() {
		return true
	}
	if statusOverride == pb.TestVerdict_EXONERATED || hasExonerations {
		return f.exonerated
	}
	switch status {
	case pb.TestVerdict_FAILED:
		return f.failed
	case pb.TestVerdict_EXECUTION_ERRORED:
		return f.executionErrored
	case pb.TestVerdict_PRECLUDED:
		return f.precluded
	case pb.TestVerdict_FLAKY:
		return f.flaky
	case pb.TestVerdict_SKIPPED:
		return f.skipped
	case pb.TestVerdict_PASSED:
		return f.passed
	default:
		return false
	}
}

func parseStatusFilter(rawStatus string, exonerated, flaky, passed, skipped, precluded, allStatuses bool) (statusFilter, error) {
	if allStatuses || strings.EqualFold(rawStatus, "all") {
		return statusFilter{
			failed:           true,
			executionErrored: true,
			precluded:        true,
			flaky:            true,
			exonerated:       true,
			skipped:          true,
			passed:           true,
		}, nil
	}

	if rawStatus != "" {
		var f statusFilter
		parts := strings.Split(rawStatus, ",")
		for _, p := range parts {
			p = strings.ToLower(strings.TrimSpace(p))
			p = strings.ReplaceAll(p, "-", "_")
			switch p {
			case "":
				continue
			case "failed", "fail":
				f.failed = true
			case "execution_errored", "execution_error", "errored", "error":
				f.executionErrored = true
			case "precluded":
				f.precluded = true
			case "flaky":
				f.flaky = true
			case "exonerated":
				f.exonerated = true
			case "skipped", "skip":
				f.skipped = true
			case "passed", "pass":
				f.passed = true
			default:
				return statusFilter{}, errors.Fmt("invalid status %q; valid values are: failed, execution_errored, precluded, flaky, exonerated, skipped, passed, all", p)
			}
		}
		if exonerated {
			f.exonerated = true
		}
		if flaky {
			f.flaky = true
		}
		if passed {
			f.passed = true
		}
		if skipped {
			f.skipped = true
		}
		if precluded {
			f.precluded = true
		}
		return f, nil
	}

	f := statusFilter{
		failed:           true,
		executionErrored: true,
	}
	if exonerated {
		f.exonerated = true
	}
	if flaky {
		f.flaky = true
	}
	if passed {
		f.passed = true
	}
	if skipped {
		f.skipped = true
	}
	if precluded {
		f.precluded = true
	}
	return f, nil
}

// ListedVerdict represents a test verdict formatted for listing.
type ListedVerdict struct {
	TestID                 string
	Status                 string
	Variant                *pb.Variant
	VariantHash            string
	FailureReason          string
	FailureReasonTruncated bool
	Exonerations           []*pb.TestExoneration
}

func rootVerdictToListed(tv *pb.TestVerdict) *ListedVerdict {
	statusStr := tv.Status.String()
	if tv.StatusOverride == pb.TestVerdict_EXONERATED {
		if tv.Status == pb.TestVerdict_FAILED || tv.Status == pb.TestVerdict_EXECUTION_ERRORED || tv.Status == pb.TestVerdict_PRECLUDED || tv.Status == pb.TestVerdict_FLAKY {
			statusStr = tv.Status.String() + " (EXONERATED)"
		} else {
			statusStr = "EXONERATED"
		}
	}
	var variant *pb.Variant
	varHash := ""
	if tv.TestIdStructured != nil {
		variant = tv.TestIdStructured.ModuleVariant
		varHash = tv.TestIdStructured.ModuleVariantHash
	}
	var failureReason string
	var failureTruncated bool
	for _, tr := range tv.Results {
		if tr.FailureReason != nil {
			firstLine, truncated := format.FormatFailureReasonFirstLine(tr.FailureReason, 120)
			if firstLine != "" {
				failureReason = firstLine
				failureTruncated = truncated
				break
			}
		}
	}
	return &ListedVerdict{
		TestID:                 tv.TestId,
		Status:                 statusStr,
		Variant:                variant,
		VariantHash:            varHash,
		FailureReason:          failureReason,
		FailureReasonTruncated: failureTruncated,
		Exonerations:           tv.Exonerations,
	}
}

func isLegacyVariantExonerated(tv *pb.TestVariant) bool {
	return tv.Status == pb.TestVariantStatus_EXONERATED || tv.StatusOverride == pb.TestVerdict_EXONERATED || len(tv.Exonerations) > 0
}

func legacyVariantToListed(tv *pb.TestVariant) *ListedVerdict {
	isExonerated := isLegacyVariantExonerated(tv)
	statusStr := legacyVariantStatusV2(tv).String()
	if isExonerated {
		if statusStr == "FAILED" || statusStr == "EXECUTION_ERRORED" || statusStr == "PRECLUDED" || statusStr == "FLAKY" {
			statusStr = statusStr + " (EXONERATED)"
		} else {
			statusStr = "EXONERATED"
		}
	}
	var failureReason string
	var failureTruncated bool
	for _, rb := range tv.Results {
		if tr := rb.GetResult(); tr != nil && tr.FailureReason != nil {
			firstLine, truncated := format.FormatFailureReasonFirstLine(tr.FailureReason, 120)
			if firstLine != "" {
				failureReason = firstLine
				failureTruncated = truncated
				break
			}
		}
	}
	return &ListedVerdict{
		TestID:                 tv.TestId,
		Status:                 statusStr,
		Variant:                tv.Variant,
		VariantHash:            tv.VariantHash,
		FailureReason:          failureReason,
		FailureReasonTruncated: failureTruncated,
		Exonerations:           tv.Exonerations,
	}
}

// QueryRootInvocationListedVerdicts queries test verdicts for a root invocation in UI priority order.
func QueryRootInvocationListedVerdicts(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(invID)
	effectiveStatuses := filter.toEffectiveStatuses()
	containsFilter := filter.buildContainsTestResultFilter()

	var listed []*ListedVerdict
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(1000)
		if maxVerdicts > 0 {
			remaining := maxVerdicts - len(listed)
			if remaining <= 0 {
				break
			}
			if int32(remaining) < pageSize {
				pageSize = int32(remaining)
			}
		}
		req := &pb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			OrderBy:   "ui_priority, test_id_structured",
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  pageSize,
			PageToken: pageToken,
		}
		if len(effectiveStatuses) > 0 || containsFilter != "" {
			req.Predicate = &pb.TestVerdictPredicate{
				EffectiveVerdictStatus:   effectiveStatuses,
				ContainsTestResultFilter: containsFilter,
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s (%s): %w", rootInvName, base.ResultDBPermissionReason(rootInvName, err), err)
			}
			return nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s: %w", rootInvName, err)
		}
		for i, tv := range res.TestVerdicts {
			listed = append(listed, rootVerdictToListed(tv))
			if maxVerdicts > 0 && len(listed) >= maxVerdicts {
				if i < len(res.TestVerdicts)-1 || res.NextPageToken != "" {
					hasMore = true
				}
				break
			}
		}
		if maxVerdicts > 0 && len(listed) >= maxVerdicts {
			break
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return listed, hasMore, nil
}

func formatInvocationInProgressNotice(invID string, resolved *base.ResolvedInvocation) string {
	if resolved == nil {
		return ""
	}
	if resolved.Legacy {
		if resolved.Invocation == nil {
			return ""
		}
		switch resolved.Invocation.State {
		case pb.Invocation_ACTIVE:
			return fmt.Sprintf("Note: Invocation %q is still ACTIVE (results may be incomplete; test execution is still in progress).\n", invID)
		case pb.Invocation_FINALIZING:
			return fmt.Sprintf("Note: Invocation %q is FINALIZING (results may be incomplete until finalized).\n", invID)
		}
		return ""
	}
	if resolved.RootInvocation == nil {
		return ""
	}
	switch resolved.RootInvocation.FinalizationState {
	case pb.RootInvocation_ACTIVE:
		return fmt.Sprintf("Note: Invocation %q is still ACTIVE (results may be incomplete; test execution is still in progress).\n", invID)
	case pb.RootInvocation_FINALIZING:
		return fmt.Sprintf("Note: Invocation %q is FINALIZING (results may be incomplete until finalized).\n", invID)
	}
	return ""
}

type hiddenVerdictCounts struct {
	failed           int
	executionErrored int
	precluded        int
	flaky            int
	exonerated       int
	truncated        bool
	scanCapped       bool
	scanned          int
}

func (h *hiddenVerdictCounts) add(status pb.TestVerdict_Status, statusOverride pb.TestVerdict_StatusOverride, hasExonerations bool) {
	if statusOverride == pb.TestVerdict_EXONERATED || hasExonerations {
		h.exonerated++
		return
	}
	switch status {
	case pb.TestVerdict_FAILED:
		h.failed++
	case pb.TestVerdict_EXECUTION_ERRORED:
		h.executionErrored++
	case pb.TestVerdict_PRECLUDED:
		h.precluded++
	case pb.TestVerdict_FLAKY:
		h.flaky++
	}
}

func hiddenVerdictCountsFromAgg(vc *pb.TestAggregation_VerdictCounts, filter statusFilter) hiddenVerdictCounts {
	if vc == nil || filter.All() || filter.hasScopeFilter() {
		return hiddenVerdictCounts{}
	}
	var h hiddenVerdictCounts
	if !filter.failed && vc.Failed > 0 {
		h.failed = int(vc.Failed)
	}
	if !filter.executionErrored && vc.ExecutionErrored > 0 {
		h.executionErrored = int(vc.ExecutionErrored)
	}
	if !filter.precluded && vc.Precluded > 0 {
		h.precluded = int(vc.Precluded)
	}
	if !filter.flaky && vc.Flaky > 0 {
		h.flaky = int(vc.Flaky)
	}
	if !filter.exonerated && vc.Exonerated > 0 {
		h.exonerated = int(vc.Exonerated)
	}
	return h
}

func (h hiddenVerdictCounts) total() int {
	return h.failed + h.executionErrored + h.precluded + h.flaky + h.exonerated
}

func (h hiddenVerdictCounts) formatNotice() string {
	suffix := ""
	if h.truncated {
		suffix = "+"
	}
	var counts []string
	var statusHints []string
	var flagHints []string
	if h.failed > 0 {
		counts = append(counts, fmt.Sprintf("%d%s failed", h.failed, suffix))
		statusHints = append(statusHints, "failed")
	}
	if h.executionErrored > 0 {
		counts = append(counts, fmt.Sprintf("%d%s execution-errored", h.executionErrored, suffix))
		statusHints = append(statusHints, "execution_errored")
	}
	if len(statusHints) > 0 {
		flagHints = append(flagHints, "-status="+strings.Join(statusHints, ","))
	}
	if h.precluded > 0 {
		counts = append(counts, fmt.Sprintf("%d%s precluded", h.precluded, suffix))
		flagHints = append(flagHints, "-precluded")
	}
	if h.flaky > 0 {
		counts = append(counts, fmt.Sprintf("%d%s flaky", h.flaky, suffix))
		flagHints = append(flagHints, "-flaky")
	}
	if h.exonerated > 0 {
		counts = append(counts, fmt.Sprintf("%d%s exonerated", h.exonerated, suffix))
		flagHints = append(flagHints, "-exonerated")
	}
	if len(counts) == 0 {
		if h.scanCapped {
			return fmt.Sprintf("Note: stopped after scanning %d variants; pass -all to scan all.\n", h.scanned)
		}
		return ""
	}
	noun := "verdicts"
	if h.total() == 1 && !h.truncated {
		noun = "verdict"
	}
	hintClause := fmt.Sprintf("use %s or -all-statuses to include", strings.Join(flagHints, ", "))
	if h.scanCapped {
		return fmt.Sprintf("Note: %s %s not shown (%s; stopped after scanning %d variants, pass -all to scan all).\n",
			strings.Join(counts, ", "), noun, hintClause, h.scanned)
	}
	return fmt.Sprintf("Note: %s %s not shown (%s).\n", strings.Join(counts, ", "), noun, hintClause)
}

func legacyVariantStatusV2(tv *pb.TestVariant) pb.TestVerdict_Status {
	if tv.StatusV2 != pb.TestVerdict_STATUS_UNSPECIFIED {
		return tv.StatusV2
	}
	switch tv.Status {
	case pb.TestVariantStatus_UNEXPECTED:
		return pb.TestVerdict_FAILED
	case pb.TestVariantStatus_FLAKY:
		return pb.TestVerdict_FLAKY
	case pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED:
		return pb.TestVerdict_EXECUTION_ERRORED
	case pb.TestVariantStatus_EXPECTED:
		return pb.TestVerdict_PASSED
	default:
		return pb.TestVerdict_STATUS_UNSPECIFIED
	}
}

func matchesLegacyScope(tv *pb.TestVariant, filter statusFilter) bool {
	if filter.moduleName != "" {
		modMatch := (tv.TestIdStructured != nil && tv.TestIdStructured.ModuleName == filter.moduleName) ||
			strings.Contains(tv.TestId, filter.moduleName)
		if !modMatch {
			return false
		}
	}
	if filter.containsFilter != "" {
		sub := strings.ToLower(filter.containsFilter)
		textMatch := strings.Contains(strings.ToLower(tv.TestId), sub) ||
			(tv.TestMetadata != nil && strings.Contains(strings.ToLower(tv.TestMetadata.Name), sub))
		if !textMatch {
			return false
		}
	}
	return true
}

// QueryLegacyInvocationListedVerdicts queries test variants for a legacy invocation in UI priority order.
func QueryLegacyInvocationListedVerdicts(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	listed, hasMore, _, err := queryLegacyInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
	return listed, hasMore, err
}

func queryLegacyInvocationListedVerdicts(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, maxVerdicts int) ([]*ListedVerdict, bool, hiddenVerdictCounts, error) {
	legacyInvName := "invocations/" + base.NormalizeInvocation(invID)

	var legacyPredicateStatus pb.TestVariantStatus
	if filter.All() || filter.passed || filter.skipped {
		legacyPredicateStatus = pb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED
	} else {
		// Always use UNEXPECTED_MASK rather than UNEXPECTED because ResultDB v1
		// maps StatusV2=FAILED verdicts with a skipped retry to v1 FLAKY and
		// StatusV2=EXECUTION_ERRORED verdicts to v1 UNEXPECTEDLY_SKIPPED.
		legacyPredicateStatus = pb.TestVariantStatus_UNEXPECTED_MASK
	}

	clientSideFiltering := !filter.All() || filter.hasScopeFilter()

	var listed []*ListedVerdict
	var hidden hiddenVerdictCounts
	pageToken := ""
	hasMore := false
	pages := 0

	for {
		pageSize := int32(1000)
		if !clientSideFiltering && maxVerdicts > 0 {
			remaining := maxVerdicts - len(listed)
			if remaining <= 0 {
				break
			}
			if int32(remaining) < pageSize {
				pageSize = int32(remaining)
			}
		}
		// Note: While resultdb.proto marks 'parent' as REQUIRED on QueryTestVariantsRequest,
		// its documentation specifies it "Should be empty if invocations field is specified."
		// For legacy invocations, 'Invocations' must be used because 'Parent' only accepts
		// 'rootInvocations/...' resource names and rejects legacy 'invocations/...'.
		req := &pb.QueryTestVariantsRequest{
			Invocations: []string{legacyInvName},
			PageSize:    pageSize,
			PageToken:   pageToken,
		}
		if legacyPredicateStatus != pb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED {
			req.Predicate = &pb.TestVariantPredicate{
				Status: legacyPredicateStatus,
			}
		}
		res, err := client.QueryTestVariants(ctx, req)
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, false, hiddenVerdictCounts{}, errors.Fmt("QueryTestVariants RPC failed for %s (%s): %w", legacyInvName, base.ResultDBPermissionReason(legacyInvName, err), err)
			}
			return nil, false, hiddenVerdictCounts{}, errors.Fmt("QueryTestVariants RPC failed for %s: %w", legacyInvName, err)
		}
		pages++
		hidden.scanned += len(res.TestVariants)
		for _, tv := range res.TestVariants {
			if !matchesLegacyScope(tv, filter) {
				continue
			}
			hasEx := isLegacyVariantExonerated(tv)
			st := legacyVariantStatusV2(tv)
			if !filter.Matches(st, tv.StatusOverride, hasEx) {
				hidden.add(st, tv.StatusOverride, hasEx)
				continue
			}
			if maxVerdicts > 0 && len(listed) >= maxVerdicts {
				hasMore = true
				continue
			}
			listed = append(listed, legacyVariantToListed(tv))
		}
		if maxVerdicts > 0 && len(listed) >= maxVerdicts {
			if res.NextPageToken != "" {
				hasMore = true
				hidden.truncated = true
			}
			break
		}
		if res.NextPageToken == "" {
			break
		}
		if clientSideFiltering && maxVerdicts > 0 && pages >= maxLegacyScanPages {
			hasMore = true
			hidden.truncated = true
			hidden.scanCapped = true
			break
		}
		pageToken = res.NextPageToken
	}
	return listed, hasMore, hidden, nil
}

// QueryVerdictList queries test verdicts for an invocation.
func QueryVerdictList(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, legacy bool, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	if legacy {
		return QueryLegacyInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
	}
	return QueryRootInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
}

func printEmptyRootInvocationVerdicts(out io.Writer, invID string, rootInv *pb.RootInvocation, rootAgg *pb.TestAggregation, filter statusFilter) {
	vc := rootAgg.GetTotalVerdictCounts()
	total := format.VerdictCountsTotal(vc)

	if vc != nil && total == 0 {
		format.FprintRootInvocationAggregationSummary(out, rootAgg)
		format.FprintRootInvocationSummary(out, rootInv.GetSummaryMarkdown(), rootInv)
		format.FprintZeroVerdictDiagnostics(out, invID, rootInv, rootAgg, format.ZeroVerdictDiagnosticOptions{
			ModuleName:         filter.moduleName,
			IncludeEmptyBanner: true,
			IncludeWorkUnitGet: true,
		})
		return
	}

	if filter.hasScopeFilter() {
		if !filter.All() {
			fmt.Fprintf(out, "No matching verdicts found in invocation %q (use -all-statuses or status flags to see other verdicts)\n", invID)
		} else {
			fmt.Fprintf(out, "No matching verdicts found in invocation %q\n", invID)
		}
		if filter.moduleName != "" {
			fmt.Fprintf(out, "  Hint: inspect module state with: luci module get -invocationid=%s -modulename=%s\n",
				base.ShellQuoteIfNeeded(invID), base.ShellQuoteIfNeeded(filter.moduleName))
		}
		return
	}

	format.FprintRootInvocationAggregationSummary(out, rootAgg)
	if !format.IsGenericHarnessSummary(rootInv.GetSummaryMarkdown()) {
		format.FprintRootInvocationSummary(out, rootInv.GetSummaryMarkdown(), rootInv)
	}

	var countsClause string
	if inline := format.FormatVerdictCountsInline(vc); inline != "" {
		countsClause = " " + inline
	}
	if filter.IsDefault() {
		fmt.Fprintf(out, "No failed or execution errored verdicts found in invocation %q%s (use -all-statuses or status flags to see other verdicts)\n", invID, countsClause)
		return
	}
	if !filter.All() {
		fmt.Fprintf(out, "No matching verdicts found in invocation %q%s (use -all-statuses or status flags to see other verdicts)\n", invID, countsClause)
		return
	}

	printListedVerdicts(out, invID, nil, false, filter)
}

func formatListedVerdictEntry(v *ListedVerdict) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "- %s %s\n", v.Status, v.TestID)
	if len(v.Variant.GetDef()) > 0 {
		varStr := format.FormatVariant(v.Variant)
		if v.VariantHash != "" {
			fmt.Fprintf(&b, "    Variant: %s (hash: %s)\n", varStr, v.VariantHash)
		} else {
			fmt.Fprintf(&b, "    Variant: %s\n", varStr)
		}
	}
	if v.FailureReason != "" {
		label := "Error:"
		if v.FailureReasonTruncated {
			label = "Error (truncated):"
		}
		fmt.Fprintf(&b, "    %s %s\n", label, v.FailureReason)
	}
	for _, ex := range v.Exonerations {
		reasonStr := format.StripHTML(ex.ExplanationHtml)
		if reasonStr != "" {
			reasonStr, _ = format.TruncateFirstLine(reasonStr, 120)
			fmt.Fprintf(&b, "    Exoneration: %s [%s]\n", reasonStr, ex.Reason)
		} else if ex.Reason != pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
			fmt.Fprintf(&b, "    Exoneration: [%s]\n", ex.Reason)
		}
	}
	return b.String()
}

func printListedVerdicts(out io.Writer, invID string, verdicts []*ListedVerdict, hasMore bool, filter statusFilter) {
	printListedVerdictsWithNoticeAndBudget(out, invID, verdicts, hasMore, false, filter, "", 0)
}

func printListedVerdictsWithBudget(out io.Writer, invID string, verdicts []*ListedVerdict, hasMore bool, filter statusFilter, maxBytes int) {
	printListedVerdictsWithNoticeAndBudget(out, invID, verdicts, hasMore, false, filter, "", maxBytes)
}

func printListedVerdictsWithNoticeAndBudget(out io.Writer, invID string, verdicts []*ListedVerdict, hasMore, scanCapped bool, filter statusFilter, hiddenNotice string, maxBytes int) {
	if len(verdicts) == 0 {
		var msg string
		if filter.IsDefault() {
			msg = fmt.Sprintf("No failed or execution errored verdicts found in invocation %q (use -all-statuses or status flags to see other verdicts)\n", invID)
		} else {
			msg = fmt.Sprintf("No matching verdicts found in invocation %q\n", invID)
		}
		if hiddenNotice != "" {
			msg += hiddenNotice
		}
		if maxBytes > 0 && len(msg) > maxBytes {
			msg = truncateVerdictListOutput(msg, maxBytes)
		}
		fmt.Fprint(out, msg)
		return
	}

	entries := make([]string, len(verdicts))
	prefixLen := make([]int, len(verdicts)+1)
	for i, v := range verdicts {
		entries[i] = formatListedVerdictEntry(v)
		prefixLen[i+1] = prefixLen[i] + len(entries[i])
	}

	var fullHeader string
	if scanCapped {
		fullHeader = fmt.Sprintf("Verdicts (showing %d, use -all to scan all):\n", len(verdicts))
	} else if hasMore {
		fullHeader = fmt.Sprintf("Verdicts (showing %d, use -max-verdicts or -all to see more):\n", len(verdicts))
	} else {
		fullHeader = fmt.Sprintf("Verdicts (%d):\n", len(verdicts))
	}

	if maxBytes <= 0 || len(fullHeader)+prefixLen[len(verdicts)]+len(hiddenNotice) <= maxBytes {
		fmt.Fprint(out, fullHeader)
		for _, entry := range entries {
			fmt.Fprint(out, entry)
		}
		if hiddenNotice != "" {
			fmt.Fprint(out, hiddenNotice)
		}
		return
	}

	totalStr := fmt.Sprintf("%d", len(verdicts))
	if hasMore {
		totalStr += "+"
	}
	quotedInv := ""
	if invID != "" {
		quotedInv = base.ShellQuoteIfNeeded(invID)
	}
	extraFlags := filter.formatCLIFlags()

	buildCapped := func(k int) (string, string) {
		hdr := fmt.Sprintf("Verdicts (showing %d of %s within 9.5 KB output limit; pass -all, -max-verdicts=<N>, or -filter=<SUBSTR> to see more):\n", k, totalStr)
		omitted := len(verdicts) - k
		omittedStr := fmt.Sprintf("%d", omitted)
		verdictNoun := "verdicts"
		if hasMore {
			omittedStr += "+"
		} else if omitted == 1 {
			verdictNoun = "verdict"
		}
		var ftr string
		if k == 0 {
			ftr = fmt.Sprintf("... %s %s omitted to stay within 9.5 KB limit; run 'luci verdict list -invocationid=%s%s -all' (or pass -modulename / -filter) to view more\n",
				omittedStr, verdictNoun, quotedInv, extraFlags)
		} else {
			ftr = fmt.Sprintf("... and %s more %s omitted to stay within 9.5 KB limit; run 'luci verdict list -invocationid=%s%s -all' (or pass -modulename / -filter) to view more\n",
				omittedStr, verdictNoun, quotedInv, extraFlags)
		}
		if hiddenNotice != "" {
			ftr += hiddenNotice
		}
		return hdr, ftr
	}

	for k := len(verdicts) - 1; k >= 0; k-- {
		hdr, ftr := buildCapped(k)
		if len(hdr)+prefixLen[k]+len(ftr) <= maxBytes {
			fmt.Fprint(out, hdr)
			for i := range k {
				fmt.Fprint(out, entries[i])
			}
			fmt.Fprint(out, ftr)
			return
		}
	}

	fallback := fmt.Sprintf("Verdicts (%s omitted to stay within output limit; pass -all to view)\n", totalStr)
	if hiddenNotice != "" {
		fallback += hiddenNotice
	}
	fmt.Fprint(out, format.TruncateUTF8Prefix(fallback, maxBytes))
}

func truncateVerdictListOutput(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	capped := format.TruncateOutputBytes(s, maxBytes-1, "pass -all to view full output")
	if capped != "" && !strings.HasSuffix(capped, "\n") {
		capped += "\n"
	}
	return capped
}

func (r *verdictListRun) hasExplicitLimitFlag() bool {
	if r.maxVerdicts > 0 && r.maxVerdicts != defaultMaxVerdicts {
		return true
	}
	explicit := false
	r.Flags.Visit(func(f *flag.Flag) {
		if f.Name == "max-verdicts" || f.Name == "limit" || f.Name == "n" {
			explicit = true
		}
	})
	return explicit
}

func (r *verdictListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && r.invocationID == "" {
		fmt.Fprintf(os.Stderr, "flag -invocationid (or a positional URL/ID) is required\n")
		return 1
	}
	if r.maxVerdicts < 0 {
		fmt.Fprintf(os.Stderr, "flag -max-verdicts must be non-negative\n")
		return 1
	}

	filter, err := parseStatusFilter(r.rawStatus, r.includeExonerated, r.includeFlaky, r.includePassed, r.includeSkipped, r.includePrecluded, r.allStatuses)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	if client == nil && r.af != nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		client, _, _, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
		ModuleName:   &r.moduleName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flag -invocationid (or a positional URL/ID containing an invocation ID) is required")
		return 1
	}
	filter.moduleName = strings.TrimSpace(r.moduleName)
	filter.containsFilter = strings.TrimSpace(r.filter)

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list verdicts: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy
	if extracted != nil {
		extracted.Legacy = legacy
	}

	maxVerdicts := r.maxVerdicts
	if r.allVerdicts {
		maxVerdicts = 0
	}

	var (
		verdicts   []*ListedVerdict
		hasMore    bool
		hidden     hiddenVerdictCounts
		rootAgg    *pb.TestAggregation
		aggFetched bool
	)
	if legacy {
		verdicts, hasMore, hidden, err = queryLegacyInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list verdicts: %s\n", err)
			return 1
		}
	} else {
		var wg sync.WaitGroup
		aggFetched = !filter.All() && !filter.hasScopeFilter()
		if aggFetched {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rootAgg = format.FetchRootInvocationAggregation(ctx, client, invID)
			}()
		}
		verdicts, hasMore, err = QueryRootInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
		wg.Wait()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list verdicts: %s\n", err)
			return 1
		}
		if rootAgg != nil {
			hidden = hiddenVerdictCountsFromAgg(rootAgg.GetTotalVerdictCounts(), filter)
		}
	}

	out := r.out
	if out == nil && a != nil {
		out = a.GetOut()
	}
	if out == nil {
		out = os.Stdout
	}
	capOutput := !r.allVerdicts && !r.hasExplicitLimitFlag()
	var headerBuf bytes.Buffer
	ids.PrintDerivedIDs(&headerBuf, extracted)
	if !legacy {
		format.FprintRootInvocationMetadata(&headerBuf, resolved.RootInvocation)
	}
	if notice := formatInvocationInProgressNotice(invID, resolved); notice != "" {
		headerBuf.WriteString(notice)
	}
	if !legacy && len(verdicts) == 0 {
		if !aggFetched {
			rootAgg = format.FetchRootInvocationAggregation(ctx, client, invID)
		}
		printEmptyRootInvocationVerdicts(&headerBuf, invID, resolved.RootInvocation, rootAgg, filter)
		headerStr := headerBuf.String()
		if capOutput && len(headerStr) > format.DefaultMaxOutputBytes {
			headerStr = truncateVerdictListOutput(headerStr, format.DefaultMaxOutputBytes)
		}
		fmt.Fprint(out, headerStr)
		return 0
	}
	hiddenNotice := hidden.formatNotice()
	const minVerdictSectionBytes = 2500
	headerStr := headerBuf.String()
	if !capOutput {
		fmt.Fprint(out, headerStr)
		printListedVerdictsWithNoticeAndBudget(out, invID, verdicts, hasMore, hidden.scanCapped, filter, hiddenNotice, 0)
		return 0
	}

	var uncappedVerdicts bytes.Buffer
	printListedVerdictsWithNoticeAndBudget(&uncappedVerdicts, invID, verdicts, hasMore, hidden.scanCapped, filter, hiddenNotice, 0)
	if len(headerStr)+uncappedVerdicts.Len() <= format.DefaultMaxOutputBytes {
		fmt.Fprint(out, headerStr)
		fmt.Fprint(out, uncappedVerdicts.String())
		return 0
	}

	verdictReserve := min(uncappedVerdicts.Len(), minVerdictSectionBytes)
	if len(headerStr) > format.DefaultMaxOutputBytes-verdictReserve {
		headerStr = truncateVerdictListOutput(headerStr, format.DefaultMaxOutputBytes-verdictReserve)
	}
	fmt.Fprint(out, headerStr)
	budget := format.DefaultMaxOutputBytes - len(headerStr)
	if uncappedVerdicts.Len() <= budget {
		fmt.Fprint(out, uncappedVerdicts.String())
	} else {
		printListedVerdictsWithNoticeAndBudget(out, invID, verdicts, hasMore, hidden.scanCapped, filter, hiddenNotice, budget)
	}
	return 0
}
