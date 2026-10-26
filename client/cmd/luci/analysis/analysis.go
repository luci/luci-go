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

// Package analysis implements the `luci analysis` CLI subcommands.
package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/maruel/subcommands"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/errors"
)

// Cmd returns the top-level subcommand for `luci analysis`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "analysis <subcommand>",
		ShortDesc: "Query LUCI Analysis rules, cluster failures, and test history",
		LongDesc: "Query LUCI Analysis (analysis.api.luci.app) rules, failure clusters, and cross-build test history.\n\n" +
			"Available subcommands:\n" +
			"  rule          Inspect failure association rules (e.g. 'luci analysis rule get -bugid 553974454')\n" +
			"  failures      Summarize variant distribution and recent failing invocations for a bug, rule, or cluster\n" +
			"  test-history  Query cross-build pass/flaky/failed stats and recent verdicts for a test ID",
		CommandRun: func() subcommands.CommandRun {
			return &analysisRun{af: af}
		},
	}
}

type analysisRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *analysisRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci analysis", "LUCI Analysis inspection", []*subcommands.Command{
		RuleCmd(r.af),
		FailuresCmd(r.af),
		TestHistoryCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

type ruleTarget struct {
	Project      string
	RuleID       string
	ResourceName string
}

// parseRuleResourceName extracts project and ruleID from "projects/<project>/rules/<rule_id>".
func parseRuleResourceName(name string) (project, ruleID string, err error) {
	trimmed := strings.TrimSpace(name)
	parts := strings.Split(trimmed, "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[1] == "" || parts[2] != "rules" || parts[3] == "" {
		return "", "", errors.Fmt("invalid rule resource name %q: expected projects/<project>/rules/<rule_id>", name)
	}
	return parts[1], parts[3], nil
}

// validateBareID validates that val is a non-empty bare ID without URLs, paths, or separators.
func validateBareID(flagName, val string) error {
	if val == "" {
		return errors.Fmt("flag %s cannot be empty", flagName)
	}
	if strings.ContainsAny(val, "/:") {
		return errors.Fmt("invalid %s %q: expected a bare ID, not a resource name or URL (use 'luci ids' to extract IDs)", flagName, val)
	}
	return nil
}

// validateBugID validates that val is a numeric Buganizer bug ID.
func validateBugID(val string) error {
	if val == "" {
		return errors.New("flag -bugid cannot be empty")
	}
	if !isDigitsOnly(val) {
		return errors.Fmt("invalid -bugid %q: expected a numeric Buganizer bug ID (e.g. 553974454); use 'luci ids' to extract IDs from URLs", val)
	}
	return nil
}

func isDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// resolveRuleTargets resolves a bug identifier (-bugid) or explicit rule target (-ruleid with
// -project) into one or more canonical LUCI Analysis rule resource targets.
func resolveRuleTargets(ctx context.Context, rulesClient analysispb.RulesClient, bugIDFlag, projectFlag, ruleIDFlag string) ([]ruleTarget, error) {
	projectFlag = strings.TrimSpace(projectFlag)
	ruleIDFlag = strings.TrimSpace(ruleIDFlag)
	bugIDFlag = strings.TrimSpace(bugIDFlag)

	if bugIDFlag != "" && ruleIDFlag != "" {
		return nil, errors.New("cannot specify both -bugid and -ruleid")
	}
	if bugIDFlag == "" && ruleIDFlag == "" {
		return nil, errors.New("either -bugid or (-project and -ruleid) is required")
	}

	if ruleIDFlag != "" {
		if projectFlag == "" {
			return nil, errors.New("flag -project is required when -ruleid is specified")
		}
		if err := validateBareID("-project", projectFlag); err != nil {
			return nil, err
		}
		if err := validateBareID("-ruleid", ruleIDFlag); err != nil {
			return nil, err
		}
		return []ruleTarget{{
			Project:      projectFlag,
			RuleID:       ruleIDFlag,
			ResourceName: fmt.Sprintf("projects/%s/rules/%s", projectFlag, ruleIDFlag),
		}}, nil
	}

	if err := validateBugID(bugIDFlag); err != nil {
		return nil, err
	}
	if projectFlag != "" {
		if err := validateBareID("-project", projectFlag); err != nil {
			return nil, err
		}
	}

	resp, err := rulesClient.LookupBug(ctx, &analysispb.LookupBugRequest{
		System: "buganizer",
		Id:     bugIDFlag,
	})
	if err != nil {
		return nil, errors.Fmt("LookupBug(buganizer:%s) RPC failed: %w", bugIDFlag, err)
	}
	if len(resp.GetRules()) == 0 {
		return nil, errors.Fmt("no LUCI Analysis rules found for bug buganizer:%s", bugIDFlag)
	}

	var matched []ruleTarget
	for _, rn := range resp.GetRules() {
		proj, rID, parseErr := parseRuleResourceName(rn)
		if parseErr != nil {
			continue
		}
		if projectFlag != "" && proj != projectFlag {
			continue
		}
		matched = append(matched, ruleTarget{
			Project:      proj,
			RuleID:       rID,
			ResourceName: rn,
		})
	}
	if len(matched) == 0 {
		return nil, errors.Fmt("no LUCI Analysis rules found for bug buganizer:%s in project %q (available rules: %s)", bugIDFlag, projectFlag, strings.Join(resp.GetRules(), ", "))
	}
	return matched, nil
}
