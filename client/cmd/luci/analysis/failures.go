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

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const (
	defaultFailuresLimit  = 3
	maxVariantValuesShown = 6
)

var priorityVariantKeys = []string{
	"builder",
	"board",
	"test_environment",
	"test_suite",
	"os",
}

// FailuresCmd returns the subcommand for `luci analysis failures`.
func FailuresCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "failures [-bugid <bug_id> | -project <project> -ruleid <rule_id> | -project <project> -clusteralgorithm <alg> -clusterid <id>] [-variant <k:v>] [-limit <n>]",
		ShortDesc: "Summarize variant breakdown and recent failing invocations for a bug, rule, or cluster",
		LongDesc: "Query cluster failure examples from LUCI Analysis, summarize failure distribution across\n" +
			"variant dimensions (builder, board, test_environment, test_suite, os), and list recent\n" +
			"failing ResultDB -invocationid and -testid flags ready for 'luci verdict get'.\n\n" +
			"Examples:\n" +
			"  luci analysis failures -bugid 553974454\n" +
			"  luci analysis failures -bugid 553974454 -variant board:vim3\n" +
			"  luci analysis failures -project turquoise -ruleid 32ae75255a4aa807897d8fa20137e7d5\n" +
			"  luci analysis failures -project chromium -clusteralgorithm reason-v6 -clusterid 0123456789abcdef",
		CommandRun: func() subcommands.CommandRun {
			r := &failuresRun{af: af, limit: defaultFailuresLimit}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.AnalysisHost, "LUCI Analysis host")
			r.Flags.StringVar(&r.bugID, "bugid", "", "Buganizer bug ID (e.g. 553974454)")
			r.Flags.StringVar(&r.project, "project", "", "LUCI project (e.g. chromium, turquoise)")
			r.Flags.StringVar(&r.ruleID, "ruleid", "", "LUCI Analysis rule ID (e.g. 32ae75255a4aa807897d8fa20137e7d5)")
			r.Flags.StringVar(&r.clusterAlg, "clusteralgorithm", "", "LUCI Analysis cluster algorithm (e.g. rules or reason-v6)")
			r.Flags.StringVar(&r.clusterID, "clusterid", "", "LUCI Analysis cluster ID (e.g. 32ae75255a4aa807897d8fa20137e7d5)")
			r.Flags.Var(&r.variants, "variant", "Filter failures by key:value variant pair (repeatable, e.g. -variant board:vim3)")
			r.Flags.IntVar(&r.limit, "limit", defaultFailuresLimit, "Maximum number of recent distinct failure groups to list (0 for all)")
			r.Flags.StringVar(&r.metric, "metric", "", "Optional metric ID to filter cluster failures (e.g. human-cls-failed-presubmit)")
			return r
		},
	}
}

type failuresRun struct {
	subcommands.CommandRunBase
	af         *base.AuthFlags
	host       string
	bugID      string
	project    string
	ruleID     string
	clusterAlg string
	clusterID  string
	variants   stringlistflag.Flag
	limit      int
	metric     string
}

type variantValueBucket struct {
	Value   string
	Count   int64
	Percent float64
}

type variantDimensionSummary struct {
	Key    string
	Values []variantValueBucket
}

type failureExampleOutput struct {
	InvocationID        string
	TestID              string
	PartitionTime       string
	Count               int32
	BuildStatus         string
	Exonerations        []string
	Variant             map[string]string
	FailureReasonPrefix string
}

type clusterFailuresReport struct {
	Project               string
	ClusterAlgorithm      string
	ClusterID             string
	AdditionalRules       []string
	Legacy                bool
	TotalFailures         int64
	DistinctGroupsMatched int
	VariantDistribution   []variantDimensionSummary
	Examples              []failureExampleOutput
}

func (r *failuresRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; pass -bugid <id>, -project <p> -ruleid <r>, or -project <p> -clusteralgorithm <a> -clusterid <c>\n")
		return 1
	}

	variantFilters, err := parseVariantFilters(r.variants)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}
	rulesClient, clustersClient, _, err := r.af.NewAnalysisClients(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	report, err := fetchClusterFailures(ctx, rulesClient, clustersClient, r.bugID, r.project, r.ruleID, r.clusterAlg, r.clusterID, r.metric, variantFilters, r.limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if len(report.Examples) > 0 {
		if rdbClient, _, _, rdbErr := r.af.NewResultDBClient(ctx, chromeinfra.ResultDBHost); rdbErr == nil {
			report.Legacy = base.IsLegacyInvocation(ctx, rdbClient, report.Examples[0].InvocationID)
		}
	}

	if err := printClusterFailuresReport(os.Stdout, report); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}

func parseVariantFilters(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	filters := make(map[string]string, len(raw))
	for _, entry := range raw {
		sep := strings.IndexAny(entry, ":=")
		if sep <= 0 {
			return nil, errors.Fmt("invalid -variant filter %q: expected key:value", entry)
		}
		filters[strings.TrimSpace(entry[:sep])] = strings.TrimSpace(entry[sep+1:])
	}
	return filters, nil
}

// resolveClusterTarget resolves either:
//  1. An explicit cluster (-project <p> -clusteralgorithm <a> -clusterid <c>), or
//  2. A rule target (-project <p> -ruleid <r>), or
//  3. A bug target (-bugid <id>) into its rule cluster (projects/<project>/clusters/rules/<rule_id>).
func resolveClusterTarget(ctx context.Context, rulesClient analysispb.RulesClient, bugIDFlag, projectFlag, ruleIDFlag, clusterAlgFlag, clusterIDFlag string) (project, alg, clusterID string, extraRules []string, err error) {
	projectFlag = strings.TrimSpace(projectFlag)
	ruleIDFlag = strings.TrimSpace(ruleIDFlag)
	clusterAlgFlag = strings.TrimSpace(clusterAlgFlag)
	clusterIDFlag = strings.TrimSpace(clusterIDFlag)
	bugIDFlag = strings.TrimSpace(bugIDFlag)

	hasBug := bugIDFlag != ""
	hasRule := ruleIDFlag != ""
	hasCluster := clusterAlgFlag != "" || clusterIDFlag != ""

	modesCount := 0
	if hasBug {
		modesCount++
	}
	if hasRule {
		modesCount++
	}
	if hasCluster {
		modesCount++
	}

	if modesCount > 1 {
		return "", "", "", nil, errors.New("-bugid, -ruleid, and (-clusteralgorithm, -clusterid) are mutually exclusive; specify only one target")
	}
	if modesCount == 0 {
		return "", "", "", nil, errors.New("must specify one of -bugid, (-project and -ruleid), or (-project, -clusteralgorithm, and -clusterid)")
	}

	if hasCluster {
		if projectFlag == "" {
			return "", "", "", nil, errors.New("flag -project is required when -clusteralgorithm and -clusterid are specified")
		}
		if clusterAlgFlag == "" {
			return "", "", "", nil, errors.New("flag -clusteralgorithm is required when -clusterid is specified")
		}
		if clusterIDFlag == "" {
			return "", "", "", nil, errors.New("flag -clusterid is required when -clusteralgorithm is specified")
		}
		if err := validateBareID("-project", projectFlag); err != nil {
			return "", "", "", nil, err
		}
		if err := validateBareID("-clusteralgorithm", clusterAlgFlag); err != nil {
			return "", "", "", nil, err
		}
		if err := validateBareID("-clusterid", clusterIDFlag); err != nil {
			return "", "", "", nil, err
		}
		return projectFlag, clusterAlgFlag, clusterIDFlag, nil, nil
	}

	targets, err := resolveRuleTargets(ctx, rulesClient, bugIDFlag, projectFlag, ruleIDFlag)
	if err != nil {
		return "", "", "", nil, err
	}

	chosen := targets[0]
	if len(targets) > 1 {
		// Prefer the first active rule when a bug maps to multiple rules (across or within projects).
		for _, t := range targets {
			r, getErr := rulesClient.Get(ctx, &analysispb.GetRuleRequest{Name: t.ResourceName})
			if getErr == nil && r.GetIsActive() {
				chosen = t
				break
			}
		}
		for _, t := range targets {
			if t.ResourceName != chosen.ResourceName {
				extraRules = append(extraRules, t.ResourceName)
			}
		}
	}
	return chosen.Project, "rules", chosen.RuleID, extraRules, nil
}

// fetchClusterFailures queries Clusters.QueryClusterFailures (which returns up to 2,000 recent
// distinct failure groups from BigQuery), aggregates variant counts across ALL matched groups so
// low-volume physical hardware dimensions are never obscured by high-volume emulator flakes, and
// returns the most recent distinct groups up to limit.
func fetchClusterFailures(
	ctx context.Context,
	rulesClient analysispb.RulesClient,
	clustersClient analysispb.ClustersClient,
	bugIDFlag, projectFlag, ruleIDFlag, clusterAlgFlag, clusterIDFlag, metricFlag string,
	variantFilters map[string]string,
	limit int,
) (*clusterFailuresReport, error) {
	project, alg, clusterID, extraRules, err := resolveClusterTarget(ctx, rulesClient, bugIDFlag, projectFlag, ruleIDFlag, clusterAlgFlag, clusterIDFlag)
	if err != nil {
		return nil, err
	}

	req := &analysispb.QueryClusterFailuresRequest{
		Parent: fmt.Sprintf("projects/%s/clusters/%s/%s/failures", project, alg, clusterID),
	}
	if m := strings.TrimSpace(metricFlag); m != "" {
		if err := validateBareID("-metric", m); err != nil {
			return nil, err
		}
		req.MetricFilter = fmt.Sprintf("projects/%s/metrics/%s", project, m)
	}

	resp, err := clustersClient.QueryClusterFailures(ctx, req)
	if err != nil {
		return nil, errors.Fmt("QueryClusterFailures(%s) RPC failed: %w", req.Parent, err)
	}

	var matched []*analysispb.DistinctClusterFailure
	for _, f := range resp.GetFailures() {
		if matchesVariantFilters(f.GetVariant().GetDef(), variantFilters) {
			matched = append(matched, f)
		}
	}

	var totalFailures int64
	for _, f := range matched {
		c := int64(f.GetCount())
		if c <= 0 {
			c = 1
		}
		totalFailures += c
	}

	return &clusterFailuresReport{
		Project:               project,
		ClusterAlgorithm:      alg,
		ClusterID:             clusterID,
		AdditionalRules:       extraRules,
		TotalFailures:         totalFailures,
		DistinctGroupsMatched: len(matched),
		VariantDistribution:   buildVariantDistribution(matched, totalFailures),
		Examples:              formatFailureExamples(matched, limit),
	}, nil
}

// formatFailureExamples sorts distinct failure groups by partition_time DESC and formats up to
// limit records (or all records when limit <= 0) into normalized failureExampleOutput items.
func formatFailureExamples(failures []*analysispb.DistinctClusterFailure, limit int) []failureExampleOutput {
	sorted := make([]*analysispb.DistinctClusterFailure, len(failures))
	copy(sorted, failures)

	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].GetPartitionTime().AsTime()
		tj := sorted[j].GetPartitionTime().AsTime()
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		if sorted[i].GetIngestedInvocationId() != sorted[j].GetIngestedInvocationId() {
			return sorted[i].GetIngestedInvocationId() < sorted[j].GetIngestedInvocationId()
		}
		return sorted[i].GetTestId() < sorted[j].GetTestId()
	})

	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}

	examples := make([]failureExampleOutput, 0, len(sorted))
	for _, f := range sorted {
		c := f.GetCount()
		if c <= 0 {
			c = 1
		}
		var exReasons []string
		for _, ex := range f.GetExonerations() {
			if ex.GetReason() != analysispb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
				exReasons = append(exReasons, ex.GetReason().String())
			}
		}
		partTime := ""
		if f.GetPartitionTime() != nil && f.GetPartitionTime().IsValid() {
			partTime = f.GetPartitionTime().AsTime().UTC().Format(time.RFC3339)
		}
		buildStatus := ""
		if f.GetBuildStatus() != analysispb.BuildStatus_BUILD_STATUS_UNSPECIFIED {
			buildStatus = strings.TrimPrefix(f.GetBuildStatus().String(), "BUILD_STATUS_")
		}
		examples = append(examples, failureExampleOutput{
			InvocationID:        base.NormalizeInvocation(f.GetIngestedInvocationId()),
			TestID:              f.GetTestId(),
			PartitionTime:       partTime,
			Count:               c,
			BuildStatus:         buildStatus,
			Exonerations:        exReasons,
			Variant:             f.GetVariant().GetDef(),
			FailureReasonPrefix: f.GetFailureReasonPrefix(),
		})
	}
	return examples
}

func matchesVariantFilters(def, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for k, want := range filters {
		if def[k] != want {
			return false
		}
	}
	return true
}

// buildVariantDistribution aggregates total failure counts per variant key/value across all
// matched failure groups. Priority triage dimensions (builder, board, test_environment,
// test_suite, os) are listed first, followed by any other discriminating keys (with >=2 distinct
// values) so single-value boilerplate tags do not clutter the summary header.
func buildVariantDistribution(failures []*analysispb.DistinctClusterFailure, totalFailures int64) []variantDimensionSummary {
	if len(failures) == 0 || totalFailures <= 0 {
		return nil
	}

	keyCounts := make(map[string]map[string]int64)
	for _, f := range failures {
		c := int64(f.GetCount())
		if c <= 0 {
			c = 1
		}
		for k, v := range f.GetVariant().GetDef() {
			if v == "" {
				continue
			}
			m, ok := keyCounts[k]
			if !ok {
				m = make(map[string]int64)
				keyCounts[k] = m
			}
			m[v] += c
		}
	}

	var orderedKeys []string
	seen := make(map[string]bool)
	for _, pk := range priorityVariantKeys {
		if _, ok := keyCounts[pk]; ok {
			orderedKeys = append(orderedKeys, pk)
			seen[pk] = true
		}
	}
	if len(orderedKeys) == 0 {
		var fallbackKeys []string
		for k := range keyCounts {
			fallbackKeys = append(fallbackKeys, k)
		}
		sort.Strings(fallbackKeys)
		orderedKeys = fallbackKeys
	} else {
		var extraDiscriminating []string
		for k, m := range keyCounts {
			if !seen[k] && len(m) > 1 {
				extraDiscriminating = append(extraDiscriminating, k)
			}
		}
		sort.Strings(extraDiscriminating)
		orderedKeys = append(orderedKeys, extraDiscriminating...)
	}

	summaries := make([]variantDimensionSummary, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		valMap := keyCounts[k]
		buckets := make([]variantValueBucket, 0, len(valMap))
		for val, cnt := range valMap {
			buckets = append(buckets, variantValueBucket{
				Value:   val,
				Count:   cnt,
				Percent: float64(cnt) * 100.0 / float64(totalFailures),
			})
		}
		sort.Slice(buckets, func(i, j int) bool {
			if buckets[i].Count != buckets[j].Count {
				return buckets[i].Count > buckets[j].Count
			}
			return buckets[i].Value < buckets[j].Value
		})
		summaries = append(summaries, variantDimensionSummary{
			Key:    k,
			Values: buckets,
		})
	}
	return summaries
}

func printClusterFailuresReport(out io.Writer, report *clusterFailuresReport) error {
	if report == nil {
		return errors.New("nil cluster failures report")
	}

	fmt.Fprintf(out, "Project: %s | Cluster: %s/%s\n", report.Project, report.ClusterAlgorithm, report.ClusterID)
	if report.Legacy {
		fmt.Fprintf(out, "Legacy:  true (downstream luci verdict / test-result / artifact commands require -legacy)\n")
	}
	if len(report.AdditionalRules) > 0 {
		fmt.Fprintf(out, "Note: Bug also matches additional rules (%s); pass -project <p> to inspect another project.\n",
			strings.Join(report.AdditionalRules, ", "))
	}

	if report.TotalFailures == 0 {
		fmt.Fprintf(out, "\nNo matching cluster failures found.\n")
		return nil
	}

	if len(report.VariantDistribution) > 0 {
		fmt.Fprintf(out, "\nVariant Distribution (%d failures across %d distinct groups):\n",
			report.TotalFailures, report.DistinctGroupsMatched)
		for _, dim := range report.VariantDistribution {
			var parts []string
			shown := dim.Values
			extra := 0
			if len(shown) > maxVariantValuesShown {
				extra = len(shown) - maxVariantValuesShown
				shown = shown[:maxVariantValuesShown]
			}
			for _, b := range shown {
				parts = append(parts, fmt.Sprintf("%s=%d (%.1f%%)", b.Value, b.Count, b.Percent))
			}
			if extra > 0 {
				parts = append(parts, fmt.Sprintf("+%d more", extra))
			}
			fmt.Fprintf(out, "  %-18s %s\n", dim.Key+":", strings.Join(parts, ", "))
		}
	}

	fmt.Fprintf(out, "\nRecent Failing Invocations (showing %d of %d distinct groups):\n",
		len(report.Examples), report.DistinctGroupsMatched)
	printFailureExamples(out, report.Examples)

	if len(report.Examples) > 0 {
		ex := report.Examples[0]
		legacyFlag := ""
		if report.Legacy {
			legacyFlag = "-legacy "
		}
		fmt.Fprintf(out, "\nNext step:\n  luci verdict get %s-invocationid %s -testid %q\n", legacyFlag, ex.InvocationID, ex.TestID)
	}
	return nil
}

// printFailureExamples writes formatted failure example entries.
func printFailureExamples(out io.Writer, examples []failureExampleOutput) {
	for i, ex := range examples {
		var meta []string
		if ex.PartitionTime != "" {
			meta = append(meta, ex.PartitionTime)
		}
		if ex.Count > 1 {
			meta = append(meta, fmt.Sprintf("count=%d", ex.Count))
		}
		if ex.BuildStatus != "" {
			meta = append(meta, fmt.Sprintf("build=%s", ex.BuildStatus))
		}
		if len(ex.Exonerations) > 0 {
			meta = append(meta, fmt.Sprintf("exonerated=%s", strings.Join(ex.Exonerations, ",")))
		}
		metaStr := ""
		if len(meta) > 0 {
			metaStr = " [" + strings.Join(meta, ", ") + "]"
		}
		fmt.Fprintf(out, "  %d. -invocationid %s -testid %q%s\n", i+1, ex.InvocationID, ex.TestID, metaStr)
		if variantLine := formatCompactVariant(ex.Variant); variantLine != "" {
			fmt.Fprintf(out, "     variant: %s\n", variantLine)
		}
		if ex.FailureReasonPrefix != "" {
			firstLine := strings.SplitN(strings.TrimSpace(ex.FailureReasonPrefix), "\n", 2)[0]
			fmt.Fprintf(out, "     reason:  %s\n", firstLine)
		}
	}
}

func formatCompactVariant(def map[string]string) string {
	if len(def) == 0 {
		return ""
	}
	var pairs []string
	seen := make(map[string]bool, len(def))
	for _, k := range priorityVariantKeys {
		if v, ok := def[k]; ok && v != "" {
			pairs = append(pairs, fmt.Sprintf("%s:%s", k, v))
			seen[k] = true
		}
	}
	var rest []string
	for k, v := range def {
		if !seen[k] && v != "" {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		pairs = append(pairs, fmt.Sprintf("%s:%s", k, def[k]))
	}
	return strings.Join(pairs, " ")
}
