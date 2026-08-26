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
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func ListCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	usage := "list -invocationid <invocation_id> -testid <test_id> -resultid <result_id>"
	desc := "List artifacts for a test result"
	if parentType == ParentTypeWorkUnit {
		usage = "list -invocationid <invocation_id> -workunitid <work_unit_id>"
		desc = "List artifacts for a work unit"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc: desc + " in ResultDB by explicit ID flags.\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist.",
		CommandRun: func() subcommands.CommandRun {
			r := &artifactListRun{af: af, parentType: parentType}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			if parentType == ParentTypeTestResult {
				r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
				r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
				r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			} else {
				r.Flags.StringVar(&r.workUnitID, "workunitid", "", "Work unit ID (e.g. run-tests or ants-wu...)")
			}
			r.Flags.IntVar(&r.maxArtifacts, "max-artifacts", 100, "Maximum number of artifacts to display (default: 100, 0 for all)")
			r.Flags.IntVar(&r.maxArtifacts, "max-results", 100, "Alias for -max-artifacts")
			r.Flags.BoolVar(&r.allArtifacts, "all", false, "Show all artifacts without truncation")
			return r
		},
	}
}

type artifactListRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	parentType   ParentType
	host         string
	invocationID string
	testID       string
	resultID     string
	workUnitID   string
	legacy       bool
	maxArtifacts int
	allArtifacts bool
}

func (r *artifactListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	var cleanTarget string
	if r.parentType == ParentTypeTestResult {
		if r.invocationID == "" || r.testID == "" || r.resultID == "" {
			fmt.Fprintf(os.Stderr, "flags -invocationid, -testid, and -resultid are required (run 'luci ids <url>' to extract ids)\n")
			return 1
		}
		cleanTarget = base.FormatTestResultResourceName(r.invocationID, r.testID, r.resultID)
	} else {
		if r.invocationID == "" || r.workUnitID == "" {
			fmt.Fprintf(os.Stderr, "flags -invocationid and -workunitid are required (run 'luci ids <url>' to extract ids)\n")
			return 1
		}
		cleanTarget = base.FormatWorkUnitResourceName(r.invocationID, r.workUnitID)
	}

	ctx := cli.GetContext(a, r, env)
	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	ctx = format.WithDiscoveryCache(ctx)
	client, _, _, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	maxArtifacts := r.maxArtifacts
	if r.allArtifacts {
		maxArtifacts = 0
	}

	if r.parentType == ParentTypeTestResult {
		trArtifacts, err := QueryAllArtifacts(ctx, client, cleanTarget)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for test result %q: %s\n", cleanTarget, err)
			return 1
		}

		displayedCount := len(trArtifacts)
		if maxArtifacts > 0 && displayedCount > maxArtifacts {
			displayedCount = maxArtifacts
			fmt.Printf("Test Result Artifacts (%s) (showing %d of %d, use -all to see all):\n", format.FormatTestResultBreadcrumb(cleanTarget), displayedCount, len(trArtifacts))
		} else if len(trArtifacts) > 0 {
			fmt.Printf("Test Result Artifacts (%s) (%d):\n", format.FormatTestResultBreadcrumb(cleanTarget), len(trArtifacts))
		} else {
			fmt.Printf("Test Result Artifacts (%s):\n", format.FormatTestResultBreadcrumb(cleanTarget))
		}
		format.PrintArtifactList(trArtifacts[:displayedCount])

		if !r.legacy {
			printAncestorWorkUnitNotices(ctx, client, cleanTarget, r.parentType)
		}
		return 0
	}

	// Work unit
	wuArtifacts, err := QueryAllArtifacts(ctx, client, cleanTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts for work unit %q: %s\n", cleanTarget, err)
		return 1
	}

	displayedCount := len(wuArtifacts)
	if maxArtifacts > 0 && displayedCount > maxArtifacts {
		displayedCount = maxArtifacts
		fmt.Printf("Work Unit Artifacts (%s) (showing %d of %d, use -all to see all):\n", format.FormatWorkUnitBreadcrumb(cleanTarget), displayedCount, len(wuArtifacts))
	} else if len(wuArtifacts) > 0 {
		fmt.Printf("Work Unit Artifacts (%s) (%d):\n", format.FormatWorkUnitBreadcrumb(cleanTarget), len(wuArtifacts))
	} else {
		fmt.Printf("Work Unit Artifacts (%s):\n", format.FormatWorkUnitBreadcrumb(cleanTarget))
	}
	format.PrintArtifactList(wuArtifacts[:displayedCount])

	printAncestorWorkUnitNotices(ctx, client, cleanTarget, r.parentType)
	return 0
}

func printAncestorWorkUnitNotices(ctx context.Context, client pb.ResultDBClient, target string, parentType ParentType) {
	var wuToCheck []string
	visited := make(map[string]bool)

	if parentType == ParentTypeTestResult {
		idx := strings.Index(target, "/tests/")
		if idx == -1 {
			return
		}
		parentWU := target[:idx]
		visited[parentWU] = true
		wuToCheck = append(wuToCheck, parentWU)

		for _, anc := range format.QueryAncestorWorkUnits(ctx, client, parentWU) {
			if !visited[anc.Name] {
				visited[anc.Name] = true
				wuToCheck = append(wuToCheck, anc.Name)
			}
		}
	} else if parentType == ParentTypeWorkUnit {
		visited[target] = true
		for _, anc := range format.QueryAncestorWorkUnits(ctx, client, target) {
			if !visited[anc.Name] {
				visited[anc.Name] = true
				wuToCheck = append(wuToCheck, anc.Name)
			}
		}
	}

	if len(wuToCheck) == 0 {
		return
	}

	type wuInfo struct {
		name  string
		count int
		more  bool
	}
	var withArtifacts []wuInfo

	for _, wuName := range wuToCheck {
		res, err := client.ListArtifacts(ctx, &pb.ListArtifactsRequest{
			Parent:   wuName,
			PageSize: 1000,
		})
		if err == nil && len(res.Artifacts) > 0 {
			withArtifacts = append(withArtifacts, wuInfo{
				name:  wuName,
				count: len(res.Artifacts),
				more:  res.NextPageToken != "",
			})
		}
	}

	if len(withArtifacts) > 0 {
		fmt.Println("Work Unit Artifacts:")
		for _, info := range withArtifacts {
			artLabel := "artifacts"
			countStr := fmt.Sprintf("%d", info.count)
			if info.more {
				countStr = fmt.Sprintf("%d+", info.count)
			} else if info.count == 1 {
				artLabel = "artifact"
			}
			invID, wuID := base.ExtractWorkUnitComponents(info.name)
			wuLabel := "Root work unit"
			if wuID != "" && wuID != "root" {
				wuLabel = fmt.Sprintf("Work unit %s", wuID)
			}
			fmt.Printf("  - %s contains %s %s. Run 'luci work-unit artifact list -invocationid %s -workunitid %s' to view.\n", wuLabel, countStr, artLabel, invID, wuID)
		}
		fmt.Println()
	}
}

func QueryAllArtifacts(ctx context.Context, client pb.ResultDBClient, parent string) ([]*pb.Artifact, error) {
	var all []*pb.Artifact
	pageToken := ""
	for {
		res, err := client.ListArtifacts(ctx, &pb.ListArtifactsRequest{
			Parent:    parent,
			PageSize:  1000,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, res.Artifacts...)
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return all, nil
}
