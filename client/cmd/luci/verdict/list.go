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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func ListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list -invocationid <invocation_id>",
		ShortDesc: "List test verdicts in an invocation",
		LongDesc: "List test verdicts in an invocation in UI priority order.\n\n" +
			"By default, only failed or execution errored verdicts are printed, up to 100.\n" +
			"Flags can be used to include other verdict statuses (e.g. -flaky, -exonerated, -passed, -all-statuses)\n" +
			"and to list more than 100 results (e.g. -max-verdicts 200, -all).\n\n" +
			"Works with root invocations and legacy invocations (using -legacy).",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictListRun{af: af, maxVerdicts: 100}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.IntVar(&r.maxVerdicts, "max-verdicts", 100, "Maximum number of verdicts to display (default: 100, 0 for all)")
			r.Flags.IntVar(&r.maxVerdicts, "limit", 100, "Alias for -max-verdicts")
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
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type verdictListRun struct {
	subcommands.CommandRunBase
	af                *base.AuthFlags
	host              string
	invocationID      string
	maxVerdicts       int
	allVerdicts       bool
	rawStatus         string
	includeExonerated bool
	includeFlaky      bool
	includePassed     bool
	includeSkipped    bool
	includePrecluded  bool
	allStatuses       bool
	legacy            bool
}

type statusFilter struct {
	failed           bool
	executionErrored bool
	precluded        bool
	flaky            bool
	exonerated       bool
	skipped          bool
	passed           bool
}

func (f *statusFilter) All() bool {
	return f.failed && f.executionErrored && f.precluded && f.flaky && f.exonerated && f.skipped && f.passed
}

func (f *statusFilter) IsDefault() bool {
	return f.failed && f.executionErrored && !f.precluded && !f.flaky && !f.exonerated && !f.skipped && !f.passed
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
	TestID       string
	Status       string
	Variant      *pb.Variant
	VariantHash  string
	Exonerations []*pb.TestExoneration
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
	return &ListedVerdict{
		TestID:       tv.TestId,
		Status:       statusStr,
		Variant:      variant,
		VariantHash:  varHash,
		Exonerations: tv.Exonerations,
	}
}

func legacyVariantToListed(tv *pb.TestVariant) *ListedVerdict {
	statusStr := ""
	isExonerated := tv.StatusOverride == pb.TestVerdict_EXONERATED || len(tv.Exonerations) > 0
	if tv.StatusV2 != pb.TestVerdict_STATUS_UNSPECIFIED {
		statusStr = tv.StatusV2.String()
	} else {
		switch tv.Status {
		case pb.TestVariantStatus_UNEXPECTED:
			statusStr = "FAILED"
		case pb.TestVariantStatus_FLAKY:
			statusStr = "FLAKY"
		case pb.TestVariantStatus_EXONERATED:
			statusStr = "EXONERATED"
		case pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED:
			statusStr = "SKIPPED"
		case pb.TestVariantStatus_EXPECTED:
			statusStr = "PASSED"
		default:
			statusStr = "STATUS_UNSPECIFIED"
		}
	}
	if isExonerated {
		if statusStr == "FAILED" || statusStr == "EXECUTION_ERRORED" || statusStr == "PRECLUDED" || statusStr == "FLAKY" {
			statusStr = statusStr + " (EXONERATED)"
		} else {
			statusStr = "EXONERATED"
		}
	}
	return &ListedVerdict{
		TestID:       tv.TestId,
		Status:       statusStr,
		Variant:      tv.Variant,
		VariantHash:  tv.VariantHash,
		Exonerations: tv.Exonerations,
	}
}

// QueryRootInvocationListedVerdicts queries test verdicts for a root invocation in UI priority order.
func QueryRootInvocationListedVerdicts(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(invID)
	effectiveStatuses := filter.toEffectiveStatuses()

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
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC,
			PageSize:  pageSize,
			PageToken: pageToken,
		}
		if len(effectiveStatuses) > 0 {
			req.Predicate = &pb.TestVerdictPredicate{
				EffectiveVerdictStatus: effectiveStatuses,
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			return nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s (if this is a legacy invocation, try with -legacy): %w", rootInvName, err)
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

// QueryLegacyInvocationListedVerdicts queries test variants for a legacy invocation in UI priority order.
func QueryLegacyInvocationListedVerdicts(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	legacyInvName := "invocations/" + base.NormalizeInvocation(invID)

	var legacyPredicateStatus pb.TestVariantStatus
	if filter.All() || filter.passed || filter.skipped {
		legacyPredicateStatus = pb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED
	} else if filter.exonerated || filter.flaky || filter.precluded {
		legacyPredicateStatus = pb.TestVariantStatus_UNEXPECTED_MASK
	} else {
		legacyPredicateStatus = pb.TestVariantStatus_UNEXPECTED
	}

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
			return nil, false, errors.Fmt("QueryTestVariants RPC failed for %s (if this is a root invocation, omit -legacy): %w", legacyInvName, err)
		}
		for i, tv := range res.TestVariants {
			hasEx := tv.StatusOverride == pb.TestVerdict_EXONERATED || len(tv.Exonerations) > 0
			if !filter.Matches(tv.StatusV2, tv.StatusOverride, hasEx) {
				continue
			}
			listed = append(listed, legacyVariantToListed(tv))
			if maxVerdicts > 0 && len(listed) >= maxVerdicts {
				if i < len(res.TestVariants)-1 || res.NextPageToken != "" {
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

// QueryVerdictList queries test verdicts for an invocation.
func QueryVerdictList(ctx context.Context, client pb.ResultDBClient, invID string, filter statusFilter, legacy bool, maxVerdicts int) ([]*ListedVerdict, bool, error) {
	if legacy {
		return QueryLegacyInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
	}
	return QueryRootInvocationListedVerdicts(ctx, client, invID, filter, maxVerdicts)
}

func printListedVerdicts(invID string, verdicts []*ListedVerdict, hasMore bool, isDefaultFilter bool) {
	if len(verdicts) == 0 {
		if isDefaultFilter {
			fmt.Printf("No failed or execution errored verdicts found in invocation %q (use -all-statuses or status flags to see other verdicts)\n", invID)
		} else {
			fmt.Printf("No matching verdicts found in invocation %q\n", invID)
		}
		return
	}

	if hasMore {
		fmt.Printf("Verdicts (showing %d, use -max-verdicts or -all to see more):\n", len(verdicts))
	} else {
		fmt.Printf("Verdicts (%d):\n", len(verdicts))
	}

	for _, v := range verdicts {
		fmt.Printf("- %s %s\n", v.Status, v.TestID)
		if v.Variant != nil && len(v.Variant.GetDef()) > 0 {
			varStr := format.FormatVariant(v.Variant)
			if v.VariantHash != "" {
				fmt.Printf("    Variant: %s (hash: %s)\n", varStr, v.VariantHash)
			} else {
				fmt.Printf("    Variant: %s\n", varStr)
			}
		}
		if len(v.Exonerations) > 0 {
			for _, ex := range v.Exonerations {
				reasonStr := format.StripHTML(ex.ExplanationHtml)
				if reasonStr != "" {
					reasonStr, _ = format.TruncateFirstLine(reasonStr, 120)
					fmt.Printf("    Exoneration: %s [%s]\n", reasonStr, ex.Reason)
				} else if ex.Reason != pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
					fmt.Printf("    Exoneration: [%s]\n", ex.Reason)
				}
			}
		}
	}
}

func (r *verdictListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flag -invocationid (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	if r.invocationID == "" {
		fmt.Fprintf(os.Stderr, "flag -invocationid is required (run 'luci ids <url>' to extract ids)\n")
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

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client, _, _, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	invID := base.NormalizeInvocation(r.invocationID)

	maxVerdicts := r.maxVerdicts
	if r.allVerdicts {
		maxVerdicts = 0
	}

	verdicts, hasMore, err := QueryVerdictList(ctx, client, invID, filter, r.legacy, maxVerdicts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list verdicts: %s\n", err)
		return 1
	}

	printListedVerdicts(invID, verdicts, hasMore, filter.IsDefault())
	return 0
}
