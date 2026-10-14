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
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func ListCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "list -invocationid <invocation_id> -testid <test_id> -resultid <result_id>"
		desc = "List artifacts for a test result"
		longDesc = desc + " in ResultDB by explicit ID flags.\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	case ParentTypeWorkUnit:
		usage = "list -invocationid <invocation_id> -workunitid <work_unit_id>"
		desc = "List artifacts for a work unit"
		longDesc = desc + " in ResultDB by explicit ID flags.\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	default:
		usage = "list -invocationid <invocation_id> [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>]"
		desc = "List artifacts for a work unit or test result"
		longDesc = desc + " in ResultDB by explicit ID flags.\n\n" +
			"Specify -invocationid to list invocation-level artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"Ancestor work units or parent invocations are also checked and a notice is displayed if artifacts exist."
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
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
			r.Flags.IntVar(&r.maxArtifacts, "max-artifacts", DefaultMaxArtifacts, "Maximum number of artifacts to display (default: 100, 0 for all)")
			r.Flags.BoolVar(&r.allArtifacts, "all", false, "Show all artifacts without truncation")
			return r
		},
	}
}

type artifactListRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       rdbpb.ResultDBClient
	parentType   ParentType
	host         string
	invocationID string
	testID       string
	resultID     string
	workUnitID   string
	maxArtifacts int
	allArtifacts bool
	legacy       bool // Unused; will be removed in the future.
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
	if r.maxArtifacts < 0 {
		fmt.Fprintf(os.Stderr, "flag -max-artifacts must be non-negative\n")
		return 1
	}

	effectiveParentType := r.parentType
	if effectiveParentType == ParentTypeUnknown {
		var err error
		effectiveParentType, err = ResolveParentType(r.invocationID, r.workUnitID, r.testID, r.resultID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
	} else if effectiveParentType == ParentTypeTestResult {
		if r.invocationID == "" || r.testID == "" || r.resultID == "" {
			fmt.Fprintf(os.Stderr, "flags -invocationid, -testid, and -resultid are required (run 'luci ids <url>' to extract ids)\n")
			return 1
		}
	} else if effectiveParentType == ParentTypeWorkUnit {
		if r.invocationID == "" || r.workUnitID == "" {
			fmt.Fprintf(os.Stderr, "flags -invocationid and -workunitid are required (run 'luci ids <url>' to extract ids)\n")
			return 1
		}
	} else if effectiveParentType == ParentTypeInvocation {
		if r.invocationID == "" {
			fmt.Fprintf(os.Stderr, "flag -invocationid is required (run 'luci ids <url>' to extract ids)\n")
			return 1
		}
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	if client == nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, _, _, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if resolved.Legacy && effectiveParentType == ParentTypeWorkUnit {
		fmt.Fprintf(os.Stderr, "work unit artifacts are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	cleanTarget, err := ResolveTargetResourceName(ctx, client, effectiveParentType, resolved.ID, r.workUnitID, r.testID, r.resultID, resolved.Legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	maxArtifacts := r.maxArtifacts
	if r.allArtifacts {
		maxArtifacts = 0
	}

	if effectiveParentType == ParentTypeTestResult {
		trArtifacts, err := QueryAllArtifacts(ctx, client, cleanTarget)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for test result %q: %s\n", cleanTarget, err)
			return 1
		}

		displayedCount := printArtifactHeader("Test Result", format.FormatTestResultBreadcrumb(cleanTarget), len(trArtifacts), maxArtifacts)
		format.PrintArtifactList(trArtifacts[:displayedCount])

		if resolved.Legacy {
			PrintImmediateParentInvocationNotice(ctx, client, cleanTarget, os.Stdout)
		} else {
			PrintAncestorWorkUnitNotices(ctx, client, cleanTarget, effectiveParentType, os.Stdout)
		}
		return 0
	}

	if effectiveParentType == ParentTypeInvocation {
		if resolved.Legacy {
			invArtifacts, err := QueryAllArtifacts(ctx, client, cleanTarget)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list artifacts for invocation %q: %s\n", cleanTarget, err)
				return 1
			}

			displayedCount := printArtifactHeader("Invocation", format.FormatInvocationBreadcrumb(cleanTarget), len(invArtifacts), maxArtifacts)
			format.PrintArtifactList(invArtifacts[:displayedCount])
			return 0
		}

		invArtifacts, hasMore, err := QueryInvocationArtifacts(ctx, client, cleanTarget, maxArtifacts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for invocation %q: %s\n", cleanTarget, err)
			return 1
		}

		printInvocationArtifactHeader(os.Stdout, format.FormatInvocationBreadcrumb(cleanTarget), len(invArtifacts), hasMore)
		format.PrintInvocationArtifactList(invArtifacts)
		return 0
	}

	// Work unit
	wuArtifacts, err := QueryAllArtifacts(ctx, client, cleanTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts for work unit %q: %s\n", cleanTarget, err)
		return 1
	}

	displayedCount := printArtifactHeader("Work Unit", format.FormatWorkUnitBreadcrumb(cleanTarget), len(wuArtifacts), maxArtifacts)
	format.PrintArtifactList(wuArtifacts[:displayedCount])

	PrintAncestorWorkUnitNotices(ctx, client, cleanTarget, effectiveParentType, os.Stdout)
	return 0
}

func printInvocationArtifactHeader(out io.Writer, breadcrumb string, count int, hasMore bool) {
	if hasMore {
		fmt.Fprintf(out, "Invocation Artifacts (%s) (showing %d, use -max-artifacts or -all to see more):\n", breadcrumb, count)
	} else {
		fmt.Fprintf(out, "Invocation Artifacts (%s) (%d):\n", breadcrumb, count)
	}
}

func printArtifactHeader(category, breadcrumb string, totalCount, maxArtifacts int) int {
	return printArtifactHeaderTo(os.Stdout, category, breadcrumb, totalCount, maxArtifacts)
}

func printArtifactHeaderTo(out io.Writer, category, breadcrumb string, totalCount, maxArtifacts int) int {
	displayedCount := totalCount
	if maxArtifacts > 0 && totalCount > maxArtifacts {
		displayedCount = maxArtifacts
		fmt.Fprintf(out, "%s Artifacts (%s) (showing %d of %d, use -all to see all):\n", category, breadcrumb, displayedCount, totalCount)
	} else {
		fmt.Fprintf(out, "%s Artifacts (%s) (%d):\n", category, breadcrumb, totalCount)
	}
	return displayedCount
}

func PrintImmediateParentInvocationNotice(ctx context.Context, client rdbpb.ResultDBClient, target string, out io.Writer) {
	idx := strings.Index(target, "/tests/")
	if idx == -1 {
		return
	}
	parentInv := target[:idx]
	leafInvID := strings.TrimPrefix(parentInv, "invocations/")

	res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
		Parent:   parentInv,
		PageSize: 1000,
	})
	if err == nil && len(res.Artifacts) > 0 {
		artLabel := "artifacts"
		countStr := fmt.Sprintf("%d", len(res.Artifacts))
		if res.NextPageToken != "" {
			countStr = fmt.Sprintf("%d+", len(res.Artifacts))
		} else if len(res.Artifacts) == 1 {
			artLabel = "artifact"
		}
		fmt.Fprintln(out, "Invocation Artifacts:")
		fmt.Fprintf(out, "  - Invocation %s contains %s %s. Run 'luci artifact list -invocationid %s' to view.\n", leafInvID, countStr, artLabel, leafInvID)
		fmt.Fprintln(out)
	}
}

func PrintAncestorWorkUnitNotices(ctx context.Context, client rdbpb.ResultDBClient, target string, parentType ParentType, out io.Writer) {
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
		res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
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
		fmt.Fprintln(out, "Work Unit Artifacts:")
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
			fmt.Fprintf(out, "  - %s contains %s %s. Run 'luci work-unit artifact list -invocationid %s -workunitid %s' to view.\n", wuLabel, countStr, artLabel, invID, wuID)
		}
		fmt.Fprintln(out)
	}
}

func QueryAllArtifacts(ctx context.Context, client rdbpb.ResultDBClient, parent string) ([]*rdbpb.Artifact, error) {
	var all []*rdbpb.Artifact
	pageToken := ""
	for {
		res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
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

// QueryInvocationArtifacts queries work-unit-level and test-result-level
// artifacts across a root invocation using ResultDB.QueryArtifacts, up to
// maxArtifacts (or all artifacts if maxArtifacts <= 0).
// It returns the artifacts and whether more artifacts exist beyond maxArtifacts.
func QueryInvocationArtifacts(ctx context.Context, client rdbpb.ResultDBClient, cleanTarget string, maxArtifacts int) ([]*rdbpb.Artifact, bool, error) {
	return queryInvocationArtifacts(ctx, client, cleanTarget, nil, &fieldmaskpb.FieldMask{
		Paths: []string{"name", "artifact_id", "content_type", "artifact_type", "size_bytes", "has_lines"},
	}, maxArtifacts)
}
