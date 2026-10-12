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

	"github.com/maruel/subcommands"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// RuleCmd returns the subcommand group for `luci analysis rule`.
func RuleCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "rule <subcommand>",
		ShortDesc: "Inspect LUCI Analysis failure association rules",
		LongDesc: "Inspect LUCI Analysis failure association rules and cluster impact metrics.\n\n" +
			"Available subcommands:\n" +
			"  get  Get rule definition, bug management state, and impact counts by -bugid or (-project and -ruleid)",
		CommandRun: func() subcommands.CommandRun {
			return &ruleRun{af: af}
		},
	}
}

type ruleRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *ruleRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci analysis rule", "LUCI Analysis rule inspection", []*subcommands.Command{
		RuleGetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// RuleGetCmd returns the subcommand for `luci analysis rule get`.
func RuleGetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-bugid <bug_id> | -project <project> -ruleid <rule_id>]",
		ShortDesc: "Get a LUCI Analysis rule and its cluster impact",
		LongDesc: "Retrieve a LUCI Analysis failure association rule and its cluster impact metrics.\n\n" +
			"Examples:\n" +
			"  luci analysis rule get -bugid 553974454\n" +
			"  luci analysis rule get -project turquoise -ruleid 32ae75255a4aa807897d8fa20137e7d5",
		CommandRun: func() subcommands.CommandRun {
			r := &ruleGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.AnalysisHost, "LUCI Analysis host")
			r.Flags.StringVar(&r.bugID, "bugid", "", "Buganizer bug ID (e.g. 553974454)")
			r.Flags.StringVar(&r.project, "project", "", "LUCI project (e.g. chromium, turquoise)")
			r.Flags.StringVar(&r.ruleID, "ruleid", "", "LUCI Analysis rule ID (e.g. 32ae75255a4aa807897d8fa20137e7d5)")
			return r
		},
	}
}

type ruleGetRun struct {
	subcommands.CommandRunBase
	af      *base.AuthFlags
	host    string
	bugID   string
	project string
	ruleID  string
}

type ruleDetail struct {
	Rule    *analysispb.Rule
	Cluster *analysispb.Cluster
}

func (r *ruleGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; pass -bugid <id> or -project <p> -ruleid <r>\n")
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

	details, err := fetchRuleDetails(ctx, rulesClient, clustersClient, r.bugID, r.project, r.ruleID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	if err := printRuleDetails(os.Stdout, details); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}

// fetchRuleDetails resolves each target rule via Rules.Get (for SQL predicate, associated bug,
// and bug management policies) paired with Clusters.Get on projects/<project>/clusters/rules/<rule_id>
// (for 1d/3d/7d impact counts).
func fetchRuleDetails(ctx context.Context, rulesClient analysispb.RulesClient, clustersClient analysispb.ClustersClient, bugIDFlag, projectFlag, ruleIDFlag string) ([]ruleDetail, error) {
	targets, err := resolveRuleTargets(ctx, rulesClient, bugIDFlag, projectFlag, ruleIDFlag)
	if err != nil {
		return nil, err
	}

	details := make([]ruleDetail, 0, len(targets))
	for _, t := range targets {
		rule, err := rulesClient.Get(ctx, &analysispb.GetRuleRequest{
			Name: t.ResourceName,
		})
		if err != nil {
			return nil, errors.Fmt("GetRule(%s) RPC failed: %w", t.ResourceName, err)
		}
		clusterName := fmt.Sprintf("projects/%s/clusters/rules/%s", t.Project, t.RuleID)
		cluster, err := clustersClient.Get(ctx, &analysispb.GetClusterRequest{
			Name: clusterName,
		})
		if err != nil {
			return nil, errors.Fmt("GetCluster(%s) RPC failed: %w", clusterName, err)
		}
		details = append(details, ruleDetail{
			Rule:    rule,
			Cluster: cluster,
		})
	}
	return details, nil
}

func printRuleDetails(out io.Writer, details []ruleDetail) error {
	for i, d := range details {
		if i > 0 {
			fmt.Fprintln(out)
		}
		r := d.Rule
		fmt.Fprintf(out, "Project:         %s\n", r.GetProject())
		fmt.Fprintf(out, "Rule ID:         %s\n", r.GetRuleId())
		fmt.Fprintf(out, "Resource Name:   %s\n", r.GetName())
		fmt.Fprintf(out, "Cluster Name:    projects/%s/clusters/rules/%s\n", r.GetProject(), r.GetRuleId())
		if bug := r.GetBug(); bug != nil {
			label := bug.GetLinkText()
			if label == "" {
				label = fmt.Sprintf("%s:%s", bug.GetSystem(), bug.GetId())
			}
			if bug.GetUrl() != "" {
				fmt.Fprintf(out, "Bug:             %s (%s)\n", label, bug.GetUrl())
			} else {
				fmt.Fprintf(out, "Bug:             %s\n", label)
			}
		}
		fmt.Fprintf(out, "Active:          %v (managing bug: %v, priority: %v)\n",
			r.GetIsActive(), r.GetIsManagingBug(), r.GetIsManagingBugPriority())

		var activePolicies []string
		for _, ps := range r.GetBugManagementState().GetPolicyState() {
			if ps.GetIsActive() {
				activePolicies = append(activePolicies, ps.GetPolicyId())
			}
		}
		sort.Strings(activePolicies)
		if len(activePolicies) > 0 {
			fmt.Fprintf(out, "Active Policies: %s\n", strings.Join(activePolicies, ", "))
		}
		if sc := r.GetSourceCluster(); sc.GetAlgorithm() != "" && sc.GetId() != "" {
			fmt.Fprintf(out, "Source Cluster:  %s/%s\n", sc.GetAlgorithm(), sc.GetId())
		}
		// LUCI Analysis omits rule_definition if the caller lacks analysis.rules.getDefinition.
		if def := strings.TrimSpace(r.GetRuleDefinition()); def != "" {
			fmt.Fprintf(out, "Rule Definition: %s\n", def)
		} else {
			fmt.Fprintf(out, "Rule Definition: <redacted or unset>\n")
		}

		if metrics := d.Cluster.GetMetrics(); len(metrics) > 0 {
			keys := displayMetricKeys(metrics)
			maxWidth := 0
			for _, k := range keys {
				if len(k)+1 > maxWidth {
					maxWidth = len(k) + 1
				}
			}
			fmt.Fprintf(out, "\nImpact Metrics (1d / 3d / 7d):\n")
			for _, k := range keys {
				tc := metrics[k]
				fmt.Fprintf(out, "  %-*s %d / %d / %d\n",
					maxWidth,
					k+":",
					tc.GetOneDay().GetNominal(),
					tc.GetThreeDay().GetNominal(),
					tc.GetSevenDay().GetNominal())
			}
		}

		fmt.Fprintf(out, "\nNext step:\n  luci analysis failures -project %s -ruleid %s\n", r.GetProject(), r.GetRuleId())
	}
	return nil
}

// displayMetricKeys orders core LUCI Analysis metrics first followed by any additional
// project-configured metrics that have non-zero counts in at least one window.
func displayMetricKeys(metrics map[string]*analysispb.Cluster_TimewiseCounts) []string {
	core := []string{
		"failures",
		"human-cls-failed-presubmit",
		"critical-failures-exonerated",
	}
	seen := make(map[string]bool, len(metrics))
	var out []string
	for _, p := range core {
		if _, ok := metrics[p]; ok {
			out = append(out, p)
			seen[p] = true
		}
	}
	var rest []string
	for k, tc := range metrics {
		if !seen[k] {
			if tc.GetOneDay().GetNominal() > 0 || tc.GetThreeDay().GetNominal() > 0 || tc.GetSevenDay().GetNominal() > 0 {
				rest = append(rest, k)
			}
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
