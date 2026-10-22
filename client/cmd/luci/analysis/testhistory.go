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
	"os"
	"sort"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	defaultTestHistoryDays  = 7
	maxTestHistoryDays      = 90
	defaultTestHistoryLimit = 3
	maxVariantRowsShown     = 12
	testHistoryPageSize     = 1000
	maxTestHistoryPages     = 10
)

// TestHistoryCmd returns the subcommand for `luci analysis test-history`.
func TestHistoryCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "test-history -project <project> (-testid <test_id> | -modulename <mod> -scheme <scheme> -case <case> [-coarse <coarse>] [-fine <fine>]) [-variant <k:v>] [-days <n>] [-limit <n>] [-subrealm <realm>]",
		ShortDesc: "Summarize cross-build pass/flaky/failed stats and recent verdicts for a test ID",
		LongDesc: "Query LUCI Analysis TestHistory for a test ID across recent builds to inspect per-variant\n" +
			"pass, flaky, failed, and exonerated counts (TestHistory.QueryStats) alongside recent verdicts\n" +
			"(TestHistory.Query) with -invocationid and -varianthash flags ready for 'luci verdict get'.\n\n" +
			"Accepts either a flat/encoded -testid or structured test ID flags (-modulename, -scheme, -coarse, -fine, -case).\n\n" +
			"Examples:\n" +
			"  luci analysis test-history -project turquoise -testid \"fuchsia-pkg://fuchsia.com/netstack-fidl-test#meta/netstack-fidl-test.cm\"\n" +
			"  luci analysis test-history -project chromium -modulename \"//chrome/test:browser_tests\" -scheme gtest -fine WebAppBrowserTest -case \"BrowserDisplayNotInstallable/All.0\"",
		CommandRun: func() subcommands.CommandRun {
			r := &testHistoryRun{
				af:    af,
				days:  defaultTestHistoryDays,
				limit: defaultTestHistoryLimit,
			}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.AnalysisHost, "LUCI Analysis host")
			r.Flags.StringVar(&r.project, "project", "", "LUCI project (required, e.g. chromium, turquoise)")
			r.Flags.StringVar(&r.testID, "testid", "", "Flat or structured test ID to query history for")
			r.Flags.StringVar(&r.moduleName, "modulename", "", "Structured test ID module name (e.g. //chrome/test:browser_tests)")
			r.Flags.StringVar(&r.moduleScheme, "scheme", "", "Structured test ID scheme (e.g. gtest, junit, webtest)")
			r.Flags.StringVar(&r.coarseName, "coarse", "", "Structured test ID coarse name (e.g. package name)")
			r.Flags.StringVar(&r.fineName, "fine", "", "Structured test ID fine name (e.g. class or suite name)")
			r.Flags.StringVar(&r.caseName, "case", "", "Structured test ID case/method name")
			r.Flags.Var(&r.variants, "variant", "Filter test history by key:value variant pair (repeatable, e.g. -variant board:vim3)")
			r.Flags.IntVar(&r.days, "days", defaultTestHistoryDays, "Lookback window in days (1..90)")
			r.Flags.IntVar(&r.limit, "limit", defaultTestHistoryLimit, "Maximum number of recent verdicts to display (0 for all fetched)")
			r.Flags.StringVar(&r.subRealm, "subrealm", "", "Optional project-scoped subrealm filter (e.g. ci or try)")
			return r
		},
	}
}

type testHistoryRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	host         string
	project      string
	testID       string
	moduleName   string
	moduleScheme string
	coarseName   string
	fineName     string
	caseName     string
	variants     stringlistflag.Flag
	days         int
	limit        int
	subRealm     string
}

type variantStatsSummary struct {
	VariantHash          string
	Variant              map[string]string
	Counts               *rdbpb.TestAggregation_VerdictCounts
	Total                int32
	LatestNonPassingDate string
}

func (s *variantStatsSummary) nonPassingCount() int32 {
	return s.Counts.Failed + s.Counts.ExecutionErrored + s.Counts.Precluded + s.Counts.Flaky + s.Counts.Exonerated
}

type testHistoryVerdictEntry struct {
	InvocationID  string
	VariantHash   string
	Variant       map[string]string
	Status        string
	Exonerated    bool
	PartitionTime string
}

type testHistoryReport struct {
	Project           string
	TestID            string
	TestIDStructured  *rdbpb.TestIdentifier
	SubRealm          string
	Days              int
	StatsTruncated    bool
	AllVerdictsPassed bool
	NewerPassingCount int
	TotalCounts       *rdbpb.TestAggregation_VerdictCounts
	VariantStats      []variantStatsSummary
	Verdicts          []testHistoryVerdictEntry
}

func (r *testHistoryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; pass -project <p> -testid <t> (or -modulename/-scheme/-case)\n")
		return 1
	}
	limitExplicit := false
	for _, setFlag := range flagSetNames(&r.Flags) {
		if setFlag == "limit" {
			limitExplicit = true
		}
	}

	resolvedTestID, err := resolveTestHistoryTestID(r.testID, r.moduleName, r.moduleScheme, r.coarseName, r.fineName, r.caseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}
	_, _, thClient, err := r.af.NewAnalysisClients(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	_, schemasClient, _, _ := r.af.NewResultDBClient(ctx, chromeinfra.ResultDBHost)

	report, err := fetchTestHistory(
		ctx,
		thClient,
		r.project,
		resolvedTestID,
		r.subRealm,
		r.variants,
		r.days,
		r.limit,
		!limitExplicit,
		time.Now().UTC(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if err := printTestHistoryReport(ctx, os.Stdout, schemasClient, report); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}

// resolveTestHistoryTestID resolves either a flat/encoded -testid or structured test identifier
// component flags (-modulename, -scheme, -coarse, -fine, -case) into a canonical test_id string.
func resolveTestHistoryTestID(testIDFlag, moduleName, scheme, coarse, fine, caseName string) (string, error) {
	flat := strings.TrimSpace(testIDFlag)
	mod := strings.TrimSpace(moduleName)
	sch := strings.TrimSpace(scheme)
	crs := strings.TrimSpace(coarse)
	fn := strings.TrimSpace(fine)
	cs := strings.TrimSpace(caseName)

	hasStructuredFlags := mod != "" || sch != "" || crs != "" || fn != "" || cs != ""
	if !hasStructuredFlags {
		return flat, nil
	}
	if flat != "" {
		return "", errors.New("specify either -testid or structured test ID flags (-modulename, -scheme, -case), not both")
	}
	if mod == "" || sch == "" || cs == "" {
		return "", errors.New("structured test ID requires at least -modulename, -scheme, and -case")
	}
	baseID := pbutil.BaseTestIdentifier{
		ModuleName:   mod,
		ModuleScheme: sch,
		CoarseName:   crs,
		FineName:     fn,
		CaseName:     cs,
	}
	if err := pbutil.ValidateBaseTestIdentifier(baseID, pbutil.QuerySideTestIDLimitCallback); err != nil {
		return "", errors.Fmt("invalid structured test ID: %w", err)
	}
	return pbutil.EncodeTestID(baseID), nil
}

// fetchTestHistory validates flags and queries LUCI Analysis TestHistory (QueryVariants + QueryStats + Query).
// Unlike `luci analysis failures` (which queries 7d clustered failures), `test-history` queries
// LUCI Analysis's 90-day verdict history table, allowing triage of older/verified bugs (e.g. b/561874224).
func fetchTestHistory(
	ctx context.Context,
	thClient analysispb.TestHistoryClient,
	projectFlag, testIDFlag, subRealmFlag string,
	rawVariants []string,
	daysFlag, limitFlag int,
	adaptive bool,
	now time.Time,
) (*testHistoryReport, error) {
	project := strings.TrimSpace(projectFlag)
	testID := strings.TrimSpace(testIDFlag)
	if project == "" {
		return nil, errors.New("missing required -project flag (e.g. -project chromium)")
	}
	if err := validateBareID("-project", project); err != nil {
		return nil, err
	}
	if testID == "" {
		return nil, errors.New("missing required -testid flag (or -modulename, -scheme, -case)")
	}
	if daysFlag <= 0 {
		return nil, errors.Fmt("-days must be positive, got %d", daysFlag)
	}
	days := daysFlag
	if days > maxTestHistoryDays {
		days = maxTestHistoryDays
	}

	variantFilters, err := parseVariantFilters(rawVariants)
	if err != nil {
		return nil, err
	}

	// LUCI Analysis TestVerdictPredicate.sub_realm expects the bare subrealm without "<project>:" prefix.
	subRealm := strings.TrimSpace(subRealmFlag)
	if idx := strings.IndexByte(subRealm, ':'); idx >= 0 && subRealm[:idx] == project {
		subRealm = subRealm[idx+1:]
	}

	now = now.UTC()
	earliest := now.Add(-time.Duration(days) * 24 * time.Hour)

	predicate := &analysispb.TestVerdictPredicate{
		SubRealm: subRealm,
		PartitionTimeRange: &analysispb.TimeRange{
			Earliest: timestamppb.New(earliest),
			Latest:   timestamppb.New(now),
		},
	}
	if len(variantFilters) > 0 {
		predicate.VariantPredicate = &analysispb.VariantPredicate{
			Predicate: &analysispb.VariantPredicate_Contains{
				Contains: &analysispb.Variant{Def: variantFilters},
			},
		}
	}

	// QueryStats and Query only populate `variant_hash` on returned groups/verdicts; best-effort
	// call QueryVariants first so hashes can be rendered alongside readable builder/board dimensions.
	variantDefs := fetchVariantDefinitions(ctx, thClient, project, testID, subRealm, predicate.VariantPredicate)

	stats, totalCounts, statsTruncated, err := fetchVariantStats(ctx, thClient, project, testID, predicate, variantDefs)
	if err != nil {
		return nil, err
	}

	effectiveLimit := limitFlag
	if adaptive && effectiveLimit > 0 {
		effectiveLimit = adaptiveExampleLimit(effectiveLimit, len(stats))
	}

	verdicts, allPassed, newerPassingCount, err := fetchVerdictTimeline(ctx, thClient, project, testID, predicate, variantDefs, effectiveLimit)
	if err != nil {
		return nil, err
	}

	parsedStructured, _ := pbutil.ParseStructuredTestIdentifierForInput(testID, nil)

	report := &testHistoryReport{
		Project:           project,
		TestID:            testID,
		TestIDStructured:  parsedStructured,
		SubRealm:          subRealm,
		Days:              days,
		StatsTruncated:    statsTruncated,
		AllVerdictsPassed: allPassed,
		NewerPassingCount: newerPassingCount,
		TotalCounts:       totalCounts,
		VariantStats:      stats,
		Verdicts:          verdicts,
	}

	return report, nil
}

// fetchVariantDefinitions best-effort resolves variant hashes to their key:value definitions
// via TestHistory.QueryVariants so callers immediately see hardware/builder metadata alongside hashes.
func fetchVariantDefinitions(ctx context.Context, client analysispb.TestHistoryClient, project, testID, subRealm string, vp *analysispb.VariantPredicate) map[string]map[string]string {
	defs := make(map[string]map[string]string)
	pageToken := ""
	for page := 0; page < maxTestHistoryPages; page++ {
		resp, err := client.QueryVariants(ctx, &analysispb.QueryVariantsRequest{
			Project:          project,
			TestId:           testID,
			SubRealm:         subRealm,
			VariantPredicate: vp,
			PageSize:         testHistoryPageSize,
			PageToken:        pageToken,
		})
		if err != nil || resp == nil {
			break
		}
		for _, v := range resp.Variants {
			if v.GetVariantHash() != "" && len(v.GetVariant().GetDef()) > 0 {
				defs[v.GetVariantHash()] = v.GetVariant().GetDef()
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return defs
}

// fetchVariantStats calls TestHistory.QueryStats across pages and aggregates mutually exclusive
// effective verdict status counts per variant across 24h partition buckets within the time window.
func fetchVariantStats(ctx context.Context, client analysispb.TestHistoryClient, project, testID string, predicate *analysispb.TestVerdictPredicate, variantDefs map[string]map[string]string) ([]variantStatsSummary, *rdbpb.TestAggregation_VerdictCounts, bool, error) {
	byHash := make(map[string]*variantStatsSummary)
	totalCounts := &rdbpb.TestAggregation_VerdictCounts{}
	truncated := false
	pageToken := ""
	for page := 0; page < maxTestHistoryPages; page++ {
		resp, err := client.QueryStats(ctx, &analysispb.QueryTestHistoryStatsRequest{
			Project:   project,
			TestId:    testID,
			Predicate: predicate,
			PageSize:  testHistoryPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, totalCounts, false, errors.Fmt("TestHistory.QueryStats RPC failed for %s / %s: %w", project, testID, err)
		}
		for _, g := range resp.GetGroups() {
			hash := g.GetVariantHash()
			if hash == "" {
				continue
			}
			sum, ok := byHash[hash]
			if !ok {
				sum = &variantStatsSummary{
					VariantHash: hash,
					Variant:     variantDefs[hash],
					Counts:      &rdbpb.TestAggregation_VerdictCounts{},
				}
				byHash[hash] = sum
			}
			var failed, flaky, passed, skipped, execErr, precluded, exonerated int32
			if vc := g.GetVerdictCounts(); vc != nil {
				// In LUCI Analysis VerdictCounts (v2), `failed`, `execution_errored`, and `precluded`
				// include exonerated verdicts; subtract `*_exonerated` so counts represent mutually
				// exclusive effective statuses matching ResultDB's TestAggregation.VerdictCounts.
				failed = max(0, vc.GetFailed()-vc.GetFailedExonerated())
				execErr = max(0, vc.GetExecutionErrored()-vc.GetExecutionErroredExonerated())
				precluded = max(0, vc.GetPrecluded()-vc.GetPrecludedExonerated())
				flaky = vc.GetFlaky()
				passed = vc.GetPassed()
				skipped = vc.GetSkipped()
				exonerated = vc.GetFailedExonerated() + vc.GetExecutionErroredExonerated() + vc.GetPrecludedExonerated()
			} else {
				failed = g.GetUnexpectedCount()
				flaky = g.GetFlakyCount()
				passed = g.GetExpectedCount()
				skipped = g.GetUnexpectedlySkippedCount()
				exonerated = g.GetExoneratedCount()
			}

			sum.Counts.Failed += failed
			sum.Counts.Flaky += flaky
			sum.Counts.Passed += passed
			sum.Counts.Skipped += skipped
			sum.Counts.ExecutionErrored += execErr
			sum.Counts.Precluded += precluded
			sum.Counts.Exonerated += exonerated

			totalCounts.Failed += failed
			totalCounts.Flaky += flaky
			totalCounts.Passed += passed
			totalCounts.Skipped += skipped
			totalCounts.ExecutionErrored += execErr
			totalCounts.Precluded += precluded
			totalCounts.Exonerated += exonerated

			if (failed+flaky+execErr+precluded+exonerated) > 0 && g.GetPartitionTime() != nil {
				partDate := g.GetPartitionTime().AsTime().UTC().Format("2006-01-02")
				if partDate > sum.LatestNonPassingDate {
					sum.LatestNonPassingDate = partDate
				}
			}
		}
		if resp.GetNextPageToken() == "" {
			break
		}
		if page == maxTestHistoryPages-1 {
			truncated = true
		}
		pageToken = resp.GetNextPageToken()
	}

	result := make([]variantStatsSummary, 0, len(byHash))
	for _, s := range byHash {
		c := s.Counts
		s.Total = c.Passed + c.Flaky + c.Failed + c.ExecutionErrored + c.Precluded + c.Skipped + c.Exonerated
		result = append(result, *s)
	}
	// Order variants with the highest failure/flake volume first so problematic variants
	// stand out immediately when a test runs across dozens of builders/boards.
	sort.Slice(result, func(i, j int) bool {
		if result[i].nonPassingCount() != result[j].nonPassingCount() {
			return result[i].nonPassingCount() > result[j].nonPassingCount()
		}
		if result[i].Total != result[j].Total {
			return result[i].Total > result[j].Total
		}
		return result[i].VariantHash < result[j].VariantHash
	})
	return result, totalCounts, truncated, nil
}

// fetchVerdictTimeline calls TestHistory.Query ordered by partition_time DESC and returns
// recent non-passing verdicts (falling back to recent passing verdicts when all runs passed),
// along with the number of newer passing verdicts observed ahead of the newest non-passing verdict.
func fetchVerdictTimeline(ctx context.Context, client analysispb.TestHistoryClient, project, testID string, predicate *analysispb.TestVerdictPredicate, variantDefs map[string]map[string]string, limit int) ([]testHistoryVerdictEntry, bool, int, error) {
	var nonPassing []testHistoryVerdictEntry
	var fallbackPassing []testHistoryVerdictEntry
	newerPassing := 0
	pageToken := ""
	for page := 0; page < maxTestHistoryPages; page++ {
		resp, err := client.Query(ctx, &analysispb.QueryTestHistoryRequest{
			Project:   project,
			TestId:    testID,
			Predicate: predicate,
			PageSize:  testHistoryPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, false, 0, errors.Fmt("TestHistory.Query RPC failed for %s / %s: %w", project, testID, err)
		}
		for _, tv := range resp.GetVerdicts() {
			statusLabel, isNonPassing, isExonerated := classifyVerdictStatus(tv)
			partTime := ""
			if tv.GetPartitionTime() != nil {
				partTime = tv.GetPartitionTime().AsTime().UTC().Format(time.RFC3339)
			}
			entry := testHistoryVerdictEntry{
				InvocationID:  base.NormalizeInvocation(tv.GetInvocationId()),
				VariantHash:   tv.GetVariantHash(),
				Variant:       variantDefs[tv.GetVariantHash()],
				Status:        statusLabel,
				Exonerated:    isExonerated,
				PartitionTime: partTime,
			}
			if isNonPassing {
				nonPassing = append(nonPassing, entry)
			} else {
				if len(nonPassing) == 0 {
					newerPassing++
				}
				if limit <= 0 || len(fallbackPassing) < limit {
					fallbackPassing = append(fallbackPassing, entry)
				}
			}
		}
		// Keep paginating if high-volume PASSED verdicts pushed earlier failures onto subsequent pages.
		if (limit > 0 && len(nonPassing) >= limit) || resp.GetNextPageToken() == "" {
			break
		}
		pageToken = resp.GetNextPageToken()
	}

	if len(nonPassing) > 0 {
		if limit > 0 && len(nonPassing) > limit {
			nonPassing = nonPassing[:limit]
		}
		return nonPassing, false, newerPassing, nil
	}
	return fallbackPassing, len(fallbackPassing) > 0, 0, nil
}

// classifyVerdictStatus maps LUCI Analysis TestVerdict v2 status (+ fallback v1 status)
// to a concise display label, whether it is non-passing (failed/flaky/errored/precluded),
// and whether it was exonerated.
func classifyVerdictStatus(tv *analysispb.TestVerdict) (label string, nonPassing bool, exonerated bool) {
	exonerated = tv.GetStatusOverride() == analysispb.TestVerdict_EXONERATED ||
		tv.GetStatus() == analysispb.TestVerdictStatus_EXONERATED

	switch tv.GetStatusV2() {
	case analysispb.TestVerdict_FAILED:
		return "FAILED", true, exonerated
	case analysispb.TestVerdict_FLAKY:
		return "FLAKY", true, exonerated
	case analysispb.TestVerdict_EXECUTION_ERRORED:
		return "EXECUTION_ERRORED", true, exonerated
	case analysispb.TestVerdict_PRECLUDED:
		return "PRECLUDED", true, exonerated
	case analysispb.TestVerdict_SKIPPED:
		return "SKIPPED", false, exonerated
	case analysispb.TestVerdict_PASSED:
		return "PASSED", false, exonerated
	}

	// Fallback when only deprecated v1 TestVerdictStatus is populated.
	switch tv.GetStatus() {
	case analysispb.TestVerdictStatus_UNEXPECTED:
		return "FAILED", true, exonerated
	case analysispb.TestVerdictStatus_FLAKY:
		return "FLAKY", true, exonerated
	case analysispb.TestVerdictStatus_EXONERATED:
		return "EXONERATED", true, true
	case analysispb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED:
		return "SKIPPED", true, exonerated
	case analysispb.TestVerdictStatus_EXPECTED:
		return "PASSED", false, exonerated
	default:
		return "UNKNOWN", false, exonerated
	}
}

func printTestHistoryReport(ctx context.Context, out io.Writer, schemasClient rdbpb.SchemasClient, report *testHistoryReport) error {
	fmt.Fprintf(out, "Project:   %s\n", report.Project)
	if report.SubRealm != "" {
		fmt.Fprintf(out, "SubRealm:  %s\n", report.SubRealm)
	}
	format.FprintTestID(out, ctx, schemasClient, &rdbpb.TestResult{
		TestId:           report.TestID,
		TestIdStructured: report.TestIDStructured,
	})
	fmt.Fprintf(out, "Window:    last %dd\n", report.Days)

	if len(report.VariantStats) == 0 && len(report.Verdicts) == 0 {
		fmt.Fprintf(out, "\nNo test history found in the last %d days.\n", report.Days)
		return nil
	}

	if len(report.VariantStats) > 0 {
		nonPassingVariants := 0
		for _, s := range report.VariantStats {
			if s.nonPassingCount() > 0 {
				nonPassingVariants++
			}
		}
		overallInline := format.FormatVerdictCountsInline(report.TotalCounts)
		truncNote := ""
		if report.StatsTruncated {
			truncNote = ", partial stats (page cap reached)"
		}
		fmt.Fprintf(out, "\nVariants (%d total, %d with non-passing verdicts%s) %s:\n",
			len(report.VariantStats), nonPassingVariants, truncNote, overallInline)

		shownStats := report.VariantStats
		extraStats := 0
		if len(shownStats) > maxVariantRowsShown {
			extraStats = len(shownStats) - maxVariantRowsShown
			shownStats = shownStats[:maxVariantRowsShown]
		}
		for _, s := range shownStats {
			countsStr := format.FormatVerdictCountsInline(s.Counts)
			var metaParts []string
			metaParts = append(metaParts, "hash: "+s.VariantHash)
			if report.Days > 1 && s.LatestNonPassingDate != "" {
				metaParts = append(metaParts, "last non-passing: "+s.LatestNonPassingDate)
			}
			varStr := ""
			if len(s.Variant) > 0 {
				varStr = " " + format.FormatVariant(&rdbpb.Variant{Def: s.Variant})
			}
			fmt.Fprintf(out, "- %s%s (%s)\n", countsStr, varStr, strings.Join(metaParts, ", "))
		}
		if extraStats > 0 {
			fmt.Fprintf(out, "  ... +%d more variants (pass -variant <k:v> to narrow)\n", extraStats)
		}
	}

	if len(report.Verdicts) == 0 {
		fmt.Fprintf(out, "\nNo matching test verdicts found.\n")
		return nil
	}

	if report.AllVerdictsPassed {
		fmt.Fprintf(out, "\nRecent Passing Verdicts (no failing/flaky verdicts in window; showing %d):\n", len(report.Verdicts))
	} else if report.NewerPassingCount > 0 {
		fmt.Fprintf(out, "\nRecent Non-Passing Verdicts (showing %d; %d newer verdicts passed since latest non-passing):\n",
			len(report.Verdicts), report.NewerPassingCount)
	} else {
		fmt.Fprintf(out, "\nRecent Non-Passing Verdicts (showing %d):\n", len(report.Verdicts))
	}
	for _, v := range report.Verdicts {
		statusDisplay := v.Status
		if v.Exonerated && statusDisplay != "EXONERATED" {
			statusDisplay += " (EXONERATED)"
		}
		timeSuffix := ""
		if v.PartitionTime != "" {
			timeSuffix = fmt.Sprintf(" [%s]", v.PartitionTime)
		}
		fmt.Fprintf(out, "- %s %s%s\n", statusDisplay, v.InvocationID, timeSuffix)
		if len(v.Variant) > 0 {
			varStr := format.FormatVariant(&rdbpb.Variant{Def: v.Variant})
			fmt.Fprintf(out, "    Variant: %s (hash: %s)\n", varStr, v.VariantHash)
		} else if v.VariantHash != "" {
			fmt.Fprintf(out, "    Variant Hash: %s\n", v.VariantHash)
		}
	}

	first := report.Verdicts[0]
	fmt.Fprintf(out, "\nNext step:\n  luci verdict get -invocationid %s -testid %q -varianthash %s\n",
		first.InvocationID, report.TestID, first.VariantHash)
	return nil
}
