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

package ids

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ExtractedIDs holds all extracted resource identifiers.
type ExtractedIDs struct {
	BugID              string `json:"bug_id,omitempty"`
	Project            string `json:"project,omitempty"`
	RuleID             string `json:"rule_id,omitempty"`
	ClusterAlgorithm   string `json:"cluster_algorithm,omitempty"`
	ClusterID          string `json:"cluster_id,omitempty"`
	BuildID            string `json:"build_id,omitempty"`
	InvocationID       string `json:"invocation_id,omitempty"`
	StepName           string `json:"step_name,omitempty"`
	LogName            string `json:"log_name,omitempty"`
	ModuleName         string `json:"module_name,omitempty"`
	WorkUnitID         string `json:"work_unit_id,omitempty"`
	TestID             string `json:"test_id,omitempty"`
	ResultID           string `json:"result_id,omitempty"`
	ArtifactID         string `json:"artifact_id,omitempty"`
	VariantHash        string `json:"variant_hash,omitempty"`
	SummaryPlaceholder bool   `json:"summary_placeholder,omitempty"`
	Legacy             bool   `json:"legacy,omitempty"`

	// builder, buildNumber, and logdogStreamPath are unexported internal fields used during ID extraction
	// to query Buildbucket for the build ID or canonical step/log names. They are not output.
	builder           string
	buildNumber       int
	logdogStreamPath  string
	lookupMiloVerdict bool
	miloMatchFunc     func(*pb.TestResult) bool
}

// IsEmpty returns true if no identifiers were extracted.
func (e *ExtractedIDs) IsEmpty() bool {
	return e.BugID == "" &&
		e.Project == "" &&
		e.RuleID == "" &&
		e.ClusterAlgorithm == "" &&
		e.ClusterID == "" &&
		e.BuildID == "" &&
		e.InvocationID == "" &&
		e.StepName == "" &&
		e.LogName == "" &&
		e.ModuleName == "" &&
		e.WorkUnitID == "" &&
		e.TestID == "" &&
		e.ResultID == "" &&
		e.ArtifactID == "" &&
		e.VariantHash == ""
}

// Cmd returns the subcommand for `luci ids`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ids [-json] <target>",
		ShortDesc: "Extract resource IDs from a URL or resource name",
		LongDesc: "Parse a URL or resource name (including Milo / Buildbucket / LogDog / LUCI Analysis URLs, ResultDB / LUCI Analysis resource names, bug URLs / shorthands, and AnTS / ATI URLs)\n" +
			"and extract the canonical IDs (-bugid, -project, -ruleid, -clusteralgorithm, -clusterid, -buildid, -invocationid, -step, -log, -workunitid, -testid, -resultid, -artifactid, -varianthash)\n" +
			"for use with other commands.",
		CommandRun: func() subcommands.CommandRun {
			r := &idsRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.bbHost, "bb-host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output extracted IDs in JSON format")
			// -legacy is now unused and will be removed in the future.
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type idsRun struct {
	subcommands.CommandRunBase
	af        *base.AuthFlags
	rdbClient pb.ResultDBClient
	bbClient  grpcpb.BuildsClient
	out       io.Writer
	host      string
	bbHost    string
	jsonOut   bool
	legacy    bool // Unused; will be removed in the future.
}

func (r *idsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil || target == "" {
		fmt.Fprintf(os.Stderr, "Usage: luci ids [-json] <url_or_resource_name>\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)

	extracted, err := extractIDs(ctx, r.af, r.rdbClient, r.bbClient, r.host, r.bbHost, target, false, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs: %s\n", err)
		return 1
	}

	if extracted.IsEmpty() {
		fmt.Fprintf(os.Stderr, "no resource IDs could be extracted from %q\n", target)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}
	if err := printExtractedIDs(out, extracted, r.jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}

// TargetBindings specifies pointers to subcommand flag variables that should be
// populated from ExtractedIDs when unset (and copied back into ExtractedIDs when
// already set via explicit flags).
type TargetBindings struct {
	BugID                              *string
	Project                            *string
	RuleID                             *string
	ClusterAlgorithm                   *string
	ClusterID                          *string
	BuildID                            *string
	BuildIDInt64                       *int64
	InvocationID                       *string
	StepName                           *string
	LogName                            *string
	ModuleName                         *string
	WorkUnitID                         *string
	TestID                             *string
	ResultID                           *string
	ArtifactID                         *string
	VariantHash                        *string
	ResultDBHost                       string // Optional ResultDB host override for lazy client creation (defaults to chromeinfra.ResultDBHost).
	BuildbucketHost                    string // Optional Buildbucket host override for lazy client creation (defaults to chromeinfra.BuildbucketHost).
	DefaultRootForWorkUnit             bool   // When true, sets WorkUnitID = "root" if SummaryPlaceholder is true and WorkUnitID is empty.
	IgnoreWorkUnitOnSummaryPlaceholder bool   // When true, does not copy WorkUnitID into *b.WorkUnitID if SummaryPlaceholder is true.
}

// Apply synchronizes unset flag fields in b from e (and copies explicit flag values back into e).
func (e *ExtractedIDs) Apply(b TargetBindings) {
	if e == nil {
		return
	}
	if b.DefaultRootForWorkUnit && e.WorkUnitID == "" && e.SummaryPlaceholder {
		e.WorkUnitID = "root"
	}
	bind := func(flagPtr, extractedPtr *string) {
		if flagPtr == nil {
			return
		}
		if *flagPtr == "" {
			*flagPtr = *extractedPtr
		} else if *extractedPtr == "" {
			*extractedPtr = *flagPtr
		}
	}
	if b.BugID != nil || b.RuleID != nil || b.ClusterAlgorithm != nil || b.ClusterID != nil {
		hasSelector := (b.BugID != nil && *b.BugID != "") ||
			(b.RuleID != nil && *b.RuleID != "") ||
			(b.ClusterAlgorithm != nil && *b.ClusterAlgorithm != "") ||
			(b.ClusterID != nil && *b.ClusterID != "")
		if !hasSelector {
			if b.BugID != nil && e.BugID != "" {
				*b.BugID = e.BugID
			} else if b.RuleID != nil && e.RuleID != "" {
				*b.RuleID = e.RuleID
			} else {
				bind(b.ClusterAlgorithm, &e.ClusterAlgorithm)
				bind(b.ClusterID, &e.ClusterID)
			}
		} else {
			for _, p := range [][2]*string{{b.BugID, &e.BugID}, {b.RuleID, &e.RuleID}, {b.ClusterAlgorithm, &e.ClusterAlgorithm}, {b.ClusterID, &e.ClusterID}} {
				if p[0] != nil && *p[0] != "" && *p[1] == "" {
					*p[1] = *p[0]
				}
			}
		}
	}
	bind(b.Project, &e.Project)
	bind(b.BuildID, &e.BuildID)
	if b.BuildIDInt64 != nil {
		if *b.BuildIDInt64 <= 0 && e.BuildID != "" {
			if parsedID, err := strconv.ParseInt(e.BuildID, 10, 64); err == nil {
				*b.BuildIDInt64 = parsedID
			}
		} else if *b.BuildIDInt64 > 0 && e.BuildID == "" {
			e.BuildID = strconv.FormatInt(*b.BuildIDInt64, 10)
		}
	}
	bind(b.InvocationID, &e.InvocationID)
	bind(b.StepName, &e.StepName)
	bind(b.LogName, &e.LogName)
	bind(b.TestID, &e.TestID)
	if e.ModuleName == "" && e.TestID != "" {
		e.ModuleName = base.ExtractModuleNameFromTestID(e.TestID)
	}
	bind(b.ModuleName, &e.ModuleName)
	if b.WorkUnitID != nil {
		if !(b.IgnoreWorkUnitOnSummaryPlaceholder && e.SummaryPlaceholder) {
			bind(b.WorkUnitID, &e.WorkUnitID)
		} else if *b.WorkUnitID != "" && e.WorkUnitID == "" {
			e.WorkUnitID = *b.WorkUnitID
		}
	}
	bind(b.ResultID, &e.ResultID)
	bind(b.ArtifactID, &e.ArtifactID)
	bind(b.VariantHash, &e.VariantHash)
}

// EnrichFromTestResult populates unset ModuleName, WorkUnitID, ResultID, and
// VariantHash fields on e from a fetched ResultDB TestResult.
func (e *ExtractedIDs) EnrichFromTestResult(tr *pb.TestResult) {
	if e == nil || tr == nil {
		return
	}
	if e.ModuleName == "" {
		if tr.TestIdStructured != nil && tr.TestIdStructured.ModuleName != "" && tr.TestIdStructured.ModuleScheme != "legacy" {
			e.ModuleName = tr.TestIdStructured.ModuleName
		} else if m := base.ExtractModuleNameFromTestID(tr.TestId); m != "" {
			e.ModuleName = m
		}
	}
	if e.WorkUnitID == "" {
		if _, wuID := base.ExtractWorkUnitComponents(tr.Name); wuID != "" {
			e.WorkUnitID = base.NormalizeWorkUnit(wuID)
		}
	}
	if e.ResultID == "" {
		e.ResultID = tr.ResultId
	}
	if e.VariantHash == "" {
		e.VariantHash = tr.VariantHash
	}
}

// EnrichFromWorkUnit populates unset ModuleName on e from a fetched ResultDB WorkUnit.
func (e *ExtractedIDs) EnrichFromWorkUnit(wu *pb.WorkUnit) {
	if e == nil || wu == nil {
		return
	}
	if e.ModuleName == "" && wu.ModuleId != nil && wu.ModuleId.ModuleName != "" && wu.ModuleId.ModuleScheme != "legacy" {
		e.ModuleName = wu.ModuleId.ModuleName
	}
}

// EnrichFromAnalysis populates unset LUCI Analysis Project, RuleID,
// ClusterAlgorithm, ClusterID, and BugID fields on e.
func (e *ExtractedIDs) EnrichFromAnalysis(project, ruleID, clusterAlgorithm, clusterID, bugID string) {
	if e == nil {
		return
	}
	if e.Project == "" && project != "" {
		e.Project = project
	}
	if e.RuleID == "" && ruleID != "" {
		e.RuleID = ruleID
	}
	if e.ClusterAlgorithm == "" && clusterAlgorithm != "" {
		e.ClusterAlgorithm = clusterAlgorithm
	}
	if e.ClusterID == "" && clusterID != "" {
		e.ClusterID = clusterID
	}
	if strings.HasPrefix(clusterAlgorithm, "rules") && e.RuleID == "" && clusterID != "" {
		e.RuleID = clusterID
	}
	if e.BugID == "" && bugID != "" {
		e.BugID = bugID
	}
}

// EnrichFromArtifactName populates unset WorkUnitID, TestID, ResultID,
// ModuleName, and ArtifactID fields on e from a resolved artifact resource name.
func (e *ExtractedIDs) EnrichFromArtifactName(artName, fallbackArtifactID string) {
	if e == nil {
		return
	}
	meta := base.ExtractArtifactMetadata(artName)
	if e.WorkUnitID == "" && meta.WorkUnitID != "" {
		e.WorkUnitID = meta.WorkUnitID
	}
	if e.TestID == "" && meta.TestID != "" {
		e.TestID = meta.TestID
	}
	if e.ResultID == "" && meta.ResultID != "" {
		e.ResultID = meta.ResultID
	}
	if e.ModuleName == "" && e.TestID != "" {
		e.ModuleName = base.ExtractModuleNameFromTestID(e.TestID)
	}
	if e.ArtifactID == "" && fallbackArtifactID != "" {
		e.ArtifactID = fallbackArtifactID
	}
}

// ExtractWithAuth extracts IDs from a positional target (returning (nil, nil) if
// raw is empty), lazily creating ResultDB or Buildbucket clients via af if needed,
// and applies bindings to synchronize flag variables and extracted IDs.
func ExtractWithAuth(ctx context.Context, af *base.AuthFlags, rdbClient pb.ResultDBClient, bbClient grpcpb.BuildsClient, raw string, b TargetBindings) (*ExtractedIDs, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	extracted, err := extractIDs(ctx, af, rdbClient, bbClient, b.ResultDBHost, b.BuildbucketHost, raw, false, false)
	if err != nil {
		return nil, err
	}
	extracted.Apply(b)
	return extracted, nil
}

// PrintDerivedIDs prints all extracted IDs in human-readable format followed by a blank line.
func PrintDerivedIDs(out io.Writer, extracted *ExtractedIDs) {
	if extracted == nil || extracted.IsEmpty() {
		return
	}
	_ = printExtractedIDs(out, extracted, false)
	fmt.Fprintln(out)
}

// PrintMissingFlagsError prints any derived IDs from extracted, followed by msg
// and a context-aware hint suggesting the next CLI command to run when a
// positional target was of the wrong resource type or had insufficient IDs.
func PrintMissingFlagsError(out io.Writer, extracted *ExtractedIDs, msg string) {
	PrintDerivedIDs(out, extracted)
	fmt.Fprintln(out, strings.TrimRight(msg, "\n"))
	if hint := nextStepHint(extracted, msg); hint != "" {
		fmt.Fprintln(out, hint)
	}
}

func nextStepHint(extracted *ExtractedIDs, msg string) string {
	if extracted == nil || extracted.IsEmpty() || extracted.SummaryPlaceholder {
		return ""
	}

	// 1. Target has Bug / Rule / Cluster IDs, and command does not accept -bugid.
	if !strings.Contains(msg, "-bugid") {
		if extracted.BugID != "" {
			return fmt.Sprintf("Hint: to find failing invocations and tests for this bug, run:\n  luci analysis failures -bugid %s", extracted.BugID)
		}
		if extracted.Project != "" && extracted.RuleID != "" {
			return fmt.Sprintf("Hint: to find failing invocations and tests for this rule, run:\n  luci analysis failures -project %s -ruleid %s", extracted.Project, extracted.RuleID)
		}
		if extracted.Project != "" && extracted.ClusterAlgorithm != "" && extracted.ClusterID != "" {
			return fmt.Sprintf("Hint: to find failing invocations and tests for this cluster, run:\n  luci analysis failures -project %s -clusteralgorithm %s -clusterid %s", extracted.Project, extracted.ClusterAlgorithm, extracted.ClusterID)
		}
	}

	// 2. Command expects a bug/rule/cluster (-bugid), and target is a build, invocation, or non-rule cluster.
	if strings.Contains(msg, "-bugid") {
		if extracted.Project != "" && extracted.ClusterAlgorithm != "" && extracted.ClusterID != "" && extracted.RuleID == "" {
			return fmt.Sprintf("Hint: this target is a failure cluster (not a rule); to list failures in this cluster, run:\n  luci analysis failures -project %s -clusteralgorithm %s -clusterid %s", extracted.Project, extracted.ClusterAlgorithm, extracted.ClusterID)
		}
		if extracted.BuildID != "" && extracted.InvocationID != "" {
			return fmt.Sprintf("Hint: this target is a build/invocation (not a bug or cluster); to inspect it, run:\n  luci build get -buildid %s\n  luci verdict list -invocationid %s", extracted.BuildID, extracted.InvocationID)
		}
		if extracted.BuildID != "" {
			return fmt.Sprintf("Hint: this target is a build (not a bug or cluster); to inspect it, run:\n  luci build get -buildid %s", extracted.BuildID)
		}
	}

	// 3. Command expects -project (e.g. luci analysis test-history) and TestID is already known.
	if strings.Contains(msg, "-project") && extracted.Project == "" && extracted.TestID != "" {
		return fmt.Sprintf("Hint: specify -project <project> along with the extracted test ID, e.g.:\n  luci analysis test-history -project <project> -testid %q", extracted.TestID)
	}

	// 4. Command expects -step (luci build log get/head/tail).
	if strings.Contains(msg, "-step") && extracted.BuildID != "" && extracted.StepName == "" {
		return fmt.Sprintf("Hint: to list steps and logs for this build, run:\n  luci build get -buildid %s\n  luci build log list -buildid %s", extracted.BuildID, extracted.BuildID)
	}

	if extracted.InvocationID == "" {
		return ""
	}

	verdictGetCmd := func() string {
		cmd := fmt.Sprintf("luci verdict get -invocationid %s -testid %q", extracted.InvocationID, extracted.TestID)
		if extracted.VariantHash != "" {
			cmd += " -varianthash " + extracted.VariantHash
		}
		return cmd
	}

	// 5. Command expects an artifact ID (-artifactid).
	if strings.Contains(msg, "-artifactid") && extracted.ArtifactID == "" {
		if extracted.TestID != "" && extracted.ResultID != "" {
			return fmt.Sprintf("Hint: to list available artifacts for this test result, run:\n  luci artifact list -invocationid %s -testid %q -resultid %s", extracted.InvocationID, extracted.TestID, extracted.ResultID)
		}
		if extracted.TestID != "" {
			return fmt.Sprintf("Hint: to inspect this test verdict and find a result ID first, run:\n  %s", verdictGetCmd())
		}
		if extracted.WorkUnitID != "" {
			return fmt.Sprintf("Hint: to list available artifacts for this work unit, run:\n  luci artifact list -invocationid %s -workunitid %s", extracted.InvocationID, extracted.WorkUnitID)
		}
		return fmt.Sprintf("Hint: to list available artifacts in this invocation, run:\n  luci artifact list -invocationid %s", extracted.InvocationID)
	}

	// 6. Command expects -resultid and TestID is already known.
	if strings.Contains(msg, "-resultid") && extracted.ResultID == "" && extracted.TestID != "" {
		return fmt.Sprintf("Hint: to inspect this test verdict and list its result IDs, run:\n  %s", verdictGetCmd())
	}

	// 7. Command expects -workunitid (luci work-unit get or luci work-unit artifact list).
	if strings.Contains(msg, "-workunitid") && extracted.WorkUnitID == "" {
		return fmt.Sprintf("Hint: to inspect the root work unit or list failing verdicts in this invocation, run:\n  luci work-unit get -invocationid %s -workunitid root\n  luci verdict list -invocationid %s", extracted.InvocationID, extracted.InvocationID)
	}

	// 8. Default fallback when an InvocationID was extracted (missing -buildid, -testid, -resultid, -modulename, or -project).
	return fmt.Sprintf("Hint: to list failing test verdicts and test IDs in this invocation, run:\n  luci verdict list -invocationid %s", extracted.InvocationID)
}

func printExtractedIDs(out io.Writer, extracted *ExtractedIDs, jsonOut bool) error {
	if jsonOut {
		data, err := json.MarshalIndent(extracted, "", "  ")
		if err != nil {
			return errors.Fmt("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	// Aligned human-readable output
	if extracted.BugID != "" {
		fmt.Fprintf(out, "Bug ID:            %s\n", extracted.BugID)
	}
	if extracted.Project != "" {
		fmt.Fprintf(out, "Project:           %s\n", extracted.Project)
	}
	if extracted.RuleID != "" {
		fmt.Fprintf(out, "Rule ID:           %s\n", extracted.RuleID)
	}
	if extracted.ClusterAlgorithm != "" {
		fmt.Fprintf(out, "Cluster Algorithm: %s\n", extracted.ClusterAlgorithm)
	}
	if extracted.ClusterID != "" {
		fmt.Fprintf(out, "Cluster ID:        %s\n", extracted.ClusterID)
	}
	if extracted.BuildID != "" {
		fmt.Fprintf(out, "Build ID:          %s\n", extracted.BuildID)
	}
	if extracted.InvocationID != "" {
		fmt.Fprintf(out, "Invocation ID:     %s\n", extracted.InvocationID)
	}
	if extracted.StepName != "" {
		fmt.Fprintf(out, "Step Name:         %s\n", extracted.StepName)
	}
	if extracted.LogName != "" {
		fmt.Fprintf(out, "Log Name:          %s\n", extracted.LogName)
	}
	if extracted.ModuleName != "" {
		fmt.Fprintf(out, "Module Name:       %s\n", extracted.ModuleName)
	}
	if extracted.WorkUnitID != "" {
		fmt.Fprintf(out, "Work Unit ID:      %s\n", extracted.WorkUnitID)
	}
	if extracted.TestID != "" {
		fmt.Fprintf(out, "Test ID:           %s\n", extracted.TestID)
	}
	if extracted.ResultID != "" {
		fmt.Fprintf(out, "Result ID:         %s\n", extracted.ResultID)
	}
	if extracted.ArtifactID != "" {
		fmt.Fprintf(out, "Artifact ID:       %s\n", extracted.ArtifactID)
	}
	if extracted.VariantHash != "" {
		fmt.Fprintf(out, "Variant Hash:      %s\n", extracted.VariantHash)
	}
	if extracted.Legacy {
		fmt.Fprintf(out, "Legacy:            true\n")
	}
	if extracted.SummaryPlaceholder {
		invID := extracted.InvocationID
		wuID := extracted.WorkUnitID
		if wuID == "" {
			wuID = "root"
		}
		fmt.Fprintf(out, "\nNote: The AnTS test result in this target is an invocation-level summary placeholder (Test Case: #.), so -testid and -resultid are not emitted.\n")
		fmt.Fprintf(out, "  1. To list failing test cases in this invocation, run:\n")
		fmt.Fprintf(out, "       luci verdict list -invocationid=%s\n", invID)
		fmt.Fprintf(out, "  2. If no test verdicts failed (e.g. build, setup, or target_preparer error), inspect the work unit and host logs:\n")
		fmt.Fprintf(out, "       luci work-unit get -invocationid=%s -workunitid=%s\n", invID, wuID)
		fmt.Fprintf(out, "       luci artifact list -invocationid=%s\n", invID)
	}

	return nil
}

// ExtractIDs parses target string and extracts all available resource IDs.
func ExtractIDs(ctx context.Context, rdbClient pb.ResultDBClient, bbClient grpcpb.BuildsClient, raw string, legacy bool) (*ExtractedIDs, error) {
	return extractIDs(ctx, nil, rdbClient, bbClient, "", "", raw, legacy, true)
}

func extractIDs(ctx context.Context, af *base.AuthFlags, rdbClient pb.ResultDBClient, bbClient grpcpb.BuildsClient, host, bbHost, raw string, legacy, resolveLegacy bool) (*ExtractedIDs, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty target")
	}
	if host == "" {
		host = chromeinfra.ResultDBHost
	}
	if bbHost == "" {
		bbHost = chromeinfra.BuildbucketHost
	}

	ensureRDBClient := func() (context.Context, pb.ResultDBClient) {
		if af != nil {
			if af.ParsedOpts == nil {
				_ = af.Parse()
			}
			if af.ParsedOpts != nil {
				ctx = base.WithAuthOptions(ctx, *af.ParsedOpts)
			}
			if rdbClient == nil {
				if c, _, _, rdbErr := af.NewResultDBClient(ctx, host); rdbErr == nil {
					rdbClient = c
				}
			}
		}
		return ctx, rdbClient
	}

	extracted := &ExtractedIDs{}

	extractQueryParams(raw, extracted)

	// 0. Bug URL or shorthand (e.g. b/<digits>, http(s)://b/<digits>, issuetracker..., crbug.com/<digits>)
	if extractFromBugTarget(raw, extracted) {
		return extracted, nil
	}

	// 1. Android Test Investigate (ATI) URL or AnTS target
	if ok, err := extractFromAntsTarget(ctx, ensureRDBClient, raw, extracted); err != nil {
		return nil, err
	} else if !ok && !extractFromLogDogBuildURL(raw, extracted) && !extractFromLUCIAnalysisTarget(raw, extracted) {
		clean := base.TrimResourceURL(raw)

		// 2. Milo module URL without test cases: .../modules/<module>
		if !extractFromMiloModuleURL(clean, extracted) &&
			// 3. Milo / Chromium structured URL (/modules/.../variants/.../cases/..., /tests/.../variants/...)
			!extractFromMiloStructuredURL(clean, extracted) &&
			// 4. Milo Invocation and Build URLs (/ui/inv/..., /ui/b/..., /builders/...)
			!extractFromMiloBuildURL(clean, extracted) &&
			// 5. Milo Test History URL (/ui/test/:project/:testId)
			!extractFromMiloTestHistoryURL(raw, extracted) {
			// 6. Strip /artifacts/<art_id> suffix from ResultDB resource name if present
			clean = extractArtifactSuffix(clean, extracted)

			// 7. ResultDB test result resource name: .../tests/<escaped_test_id>/results/<result_id>
			if !extractFromTestResultResourceName(clean, extracted) &&
				// 8. ResultDB test resource name: .../tests/<escaped_test_id>
				!extractFromTestResourceName(clean, extracted) &&
				// 9. ResultDB work unit resource name: rootInvocations/<root_inv>/workUnits/<wu_id>
				!extractFromWorkUnitResourceName(clean, extracted) &&
				// 10. Builder path: <project>/<bucket>/<builder>/<build_number>
				!extractFromBuilderPath(clean, extracted) {
				// 11. ResultDB root invocation / invocation resource name or bare ID: rootInvocations/<inv>, invocations/<inv>, or bare invocation ID
				extractFromInvocationResourceName(clean, extracted)
			}
		}
	}

	ensureBBClient := func() grpcpb.BuildsClient {
		if bbClient == nil && af != nil {
			if af.ParsedOpts == nil {
				_ = af.Parse()
			}
			if af.ParsedOpts != nil {
				if c, _, bbErr := af.NewBuildsClient(ctx, bbHost); bbErr == nil {
					bbClient = c
				}
			}
		}
		return bbClient
	}

	if extracted.builder != "" && extracted.buildNumber != 0 && extracted.BuildID == "" {
		if err := resolveBuildID(ctx, ensureBBClient(), extracted); err != nil {
			return nil, err
		}
	}

	if extracted.logdogStreamPath != "" && extracted.BuildID != "" {
		resolveLogDogStepAndLog(ctx, ensureBBClient(), extracted)
	}

	populateBuildIDFromInvocation(extracted)

	if (extracted.lookupMiloVerdict || resolveLegacy) && extracted.InvocationID != "" {
		ctx, rdbClient = ensureRDBClient()
	}

	if (extracted.lookupMiloVerdict || resolveLegacy) && rdbClient != nil && extracted.InvocationID != "" {
		if resolved, err := base.ResolveInvocation(ctx, rdbClient, extracted.InvocationID); err == nil && resolved != nil {
			extracted.Legacy = resolved.Legacy
		}
	} else if legacy {
		extracted.Legacy = true
	}

	if extracted.lookupMiloVerdict && rdbClient != nil && extracted.InvocationID != "" {
		results, _, _, errQuery := base.QueryVerdictResultsAndExonerations(ctx, rdbClient, extracted.InvocationID, extracted.TestID, extracted.VariantHash, extracted.Legacy, 1000)
		if errQuery == nil {
			for _, tr := range results {
				if extracted.miloMatchFunc == nil || extracted.miloMatchFunc(tr) {
					extracted.TestID = tr.TestId
					if extracted.ResultID == "" {
						extracted.ResultID = tr.ResultId
					}
					break
				}
			}
		}
	}

	return extracted, nil
}

func resolveBuildID(ctx context.Context, bbClient grpcpb.BuildsClient, extracted *ExtractedIDs) error {
	if extracted.builder == "" || extracted.buildNumber == 0 {
		return nil
	}
	if bbClient == nil {
		return errors.Fmt("cannot resolve build ID for %s/%d: Buildbucket client not available", extracted.builder, extracted.buildNumber)
	}
	parts := strings.SplitN(extracted.builder, "/", 3)
	if len(parts) != 3 {
		return errors.Fmt("invalid builder format %q", extracted.builder)
	}
	req := &bbpb.GetBuildRequest{
		Builder: &bbpb.BuilderID{
			Project: unescapePathSegment(parts[0]),
			Bucket:  unescapePathSegment(parts[1]),
			Builder: unescapePathSegment(parts[2]),
		},
		BuildNumber: int32(extracted.buildNumber),
		Mask: &bbpb.BuildMask{
			Fields: &field_mask.FieldMask{Paths: []string{"id"}},
		},
	}
	b, err := bbClient.GetBuild(ctx, req)
	if err != nil {
		return errors.Fmt("failed to resolve build ID from Buildbucket for %s/%d: %w", extracted.builder, extracted.buildNumber, err)
	}
	if b.Id == 0 {
		return errors.Fmt("Buildbucket returned empty build ID for %s/%d", extracted.builder, extracted.buildNumber)
	}
	extracted.BuildID = strconv.FormatInt(b.Id, 10)
	extracted.InvocationID = fmt.Sprintf("build-%d", b.Id)
	return nil
}

// extractQueryParams extracts ?artifact=... and ?result=... query parameters if present.
func extractQueryParams(raw string, extracted *ExtractedIDs) {
	if u, err := url.Parse(raw); err == nil {
		if art := u.Query().Get("artifact"); art != "" {
			extracted.ArtifactID = art
		}
		if res := u.Query().Get("result"); res != "" {
			extracted.ResultID = res
		}
	}
}

// extractFromAntsTarget handles AnTS/ATI targets: standalone invocation/work-unit IDs,
// ATI URLs, and AnTS TR IDs.
func extractFromAntsTarget(ctx context.Context, ensureRDBClient func() (context.Context, pb.ResultDBClient), raw string, extracted *ExtractedIDs) (bool, error) {
	if base.IsAntsInvocationID(raw) {
		extracted.InvocationID = base.NormalizeInvocation(raw)
		return true, nil
	}

	wuID := ""
	if base.IsAntsWorkUnitID(raw) {
		wuID = raw
	}

	if !base.IsAntsURL(raw) && !base.IsAntsTestResultID(raw) && wuID == "" {
		return false, nil
	}

	trID := ""
	invID := ""
	if base.IsAntsTestResultID(raw) {
		trID = raw
	} else if base.IsAntsURL(raw) {
		invID, trID = base.ExtractAntsURLComponents(raw)
		if trID == "" {
			wuID = base.ExtractAntsURLWorkUnit(raw)
		}
	}

	// Resolve standalone AnTS work unit IDs or ATI work unit URLs (without a test result ID)
	// via the AnTS v4 API and ResultDB hierarchy lookup.
	if wuID != "" && trID == "" {
		extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
		if invID != "" {
			extracted.InvocationID = base.NormalizeInvocation(invID)
		}

		var client pb.ResultDBClient
		if ensureRDBClient != nil {
			ctx, client = ensureRDBClient()
		}

		info, err := base.ResolveAntsWorkUnit(ctx, wuID)
		if err != nil {
			// If we already have an invocation ID from the URL or a pre-resolved
			// composite/attempt work unit ID, succeed gracefully without AnTS API.
			if extracted.InvocationID != "" || strings.Contains(extracted.WorkUnitID, ":u-") || strings.Contains(extracted.WorkUnitID, "-attempt__") {
				return true, nil
			}
			return false, err
		}

		if info.InvocationID != "" {
			extracted.InvocationID = base.NormalizeInvocation(info.InvocationID)
		} else if info.ResultDBRootInvocationID != "" {
			extracted.InvocationID = base.NormalizeInvocation(info.ResultDBRootInvocationID)
		}
		if info.ModuleName != "" {
			extracted.ModuleName = info.ModuleName
		}

		extracted.WorkUnitID = base.ResolveResultDBAntsWorkUnit(ctx, client, extracted.InvocationID, wuID, info)
		return true, nil
	}

	if trID == "" {
		if invID != "" {
			extracted.InvocationID = base.NormalizeInvocation(invID)
			return true, nil
		}
		return false, nil
	}

	var client pb.ResultDBClient
	if ensureRDBClient != nil {
		ctx, client = ensureRDBClient()
	}

	info, err := base.ResolveAntsTestResult(ctx, trID)
	if err != nil {
		return false, err
	}

	if info.InvocationID != "" {
		extracted.InvocationID = base.NormalizeInvocation(info.InvocationID)
	} else if invID != "" {
		extracted.InvocationID = base.NormalizeInvocation(invID)
	}
	if info.WorkUnitID != "" {
		extracted.WorkUnitID = base.NormalizeWorkUnit(info.WorkUnitID)
	}

	// If this is a module error or has no method name, extract only InvocationID, WorkUnitID, and ModuleName (or mark as SummaryPlaceholder).
	if info.IsModuleError || info.MethodName == "" {
		if info.ModuleName == "" {
			extracted.SummaryPlaceholder = true
		} else {
			extracted.ModuleName = info.ModuleName
		}
		return true, nil
	}

	// Query ResultDB to resolve canonical TestID, ResultID, VariantHash
	if client != nil && extracted.InvocationID != "" {
		results, _, errQuery := queryAntsResultDBVerdict(ctx, client, extracted.InvocationID, info)
		if errQuery != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to query ResultDB for canonical test ID: %s\n", errQuery)
		} else if len(results) > 0 {
			extracted.TestID = results[0].TestId
			extracted.VariantHash = results[0].VariantHash
			if extracted.ResultID == "" {
				extracted.ResultID = results[0].ResultId
			}
			_, wuID := base.ExtractWorkUnitComponents(results[0].Name)
			if wuID != "" {
				extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
			}
		}
	}
	if extracted.TestID == "" && info.TestCase != "" {
		trimmed := strings.TrimPrefix(info.TestCase, "#")
		if !strings.HasPrefix(trimmed, ".") {
			extracted.TestID = trimmed
		}
	}
	if extracted.TestID == "" && info.ModuleName != "" {
		extracted.ModuleName = info.ModuleName
	}

	return true, nil
}

// extractFromMiloModuleURL handles Milo module URLs without test cases: .../modules/<module>
func extractFromMiloModuleURL(clean string, extracted *ExtractedIDs) bool {
	if strings.Contains(clean, "/modules/") && !strings.Contains(clean, "/cases/") && !strings.Contains(clean, "/tests/") {
		if idx := strings.Index(clean, "/modules/"); idx != -1 {
			invPrefix := clean[:idx]
			modSuffix := clean[idx+len("/modules/"):]
			if slashIdx := strings.Index(modSuffix, "/"); slashIdx != -1 {
				modSuffix = modSuffix[:slashIdx]
			}
			extracted.InvocationID = base.NormalizeInvocation(invPrefix)
			extracted.ModuleName = unescapePathSegment(modSuffix)
			return true
		}
	}
	return false
}

// extractFromMiloStructuredURL handles Milo test investigation URLs:
// - Structured URLs: .../modules/.../schemes/.../variants/.../cases/...
// - Old/legacy URLs: .../tests/.../variants/...
func extractFromMiloStructuredURL(clean string, extracted *ExtractedIDs) bool {
	if !strings.Contains(clean, "/variants/") || (!strings.Contains(clean, "/modules/") && !strings.Contains(clean, "/cases/") && !strings.Contains(clean, "/tests/")) {
		return false
	}

	invName, variantHash, _, matchFunc, err := base.ParseVerdictName(clean)
	if err != nil {
		return false
	}

	// Only set InvocationID if invName represents an invocation (not a project path like /ui/labs/p/chromium)
	if strings.HasPrefix(invName, "invocations/") || strings.HasPrefix(invName, "rootInvocations/") || strings.HasPrefix(invName, "build-") || (!strings.Contains(invName, "/") && invName != "") {
		extracted.InvocationID = base.NormalizeInvocation(invName)
	}
	extracted.VariantHash = variantHash

	// Extract TestID from URL path if present
	if caseIdx := strings.Index(clean, "/cases/"); caseIdx != -1 {
		afterCase := clean[caseIdx+len("/cases/"):]
		caseParts := strings.Split(afterCase, "/")
		if caseName, errUnescape := url.PathUnescape(caseParts[0]); errUnescape == nil && caseName != "" {
			extracted.TestID = caseName
		}
	} else if testIdx := strings.Index(clean, "/tests/"); testIdx != -1 {
		afterTest := clean[testIdx+len("/tests/"):]
		testParts := strings.Split(afterTest, "/")
		if testName, errUnescape := url.PathUnescape(testParts[0]); errUnescape == nil && testName != "" {
			extracted.TestID = testName
		}
	}

	extracted.lookupMiloVerdict = true
	extracted.miloMatchFunc = matchFunc

	return true
}

// extractFromLogDogBuildURL handles LogDog buildbucket URLs
// (e.g. https://logs.chromium.org/logs/<project>/buildbucket/<host>/<build_id>/+/u/<step>/<log>
// or logdog://logs.chromium.org/<project>/buildbucket/<host>/<build_id>/+/u/<step>/<log>).
func extractFromLogDogBuildURL(raw string, extracted *ExtractedIDs) bool {
	clean := strings.TrimSpace(raw)
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	idx := strings.Index(clean, "/buildbucket/")
	if idx == -1 {
		return false
	}
	after := clean[idx+len("/buildbucket/"):]
	parts := strings.SplitN(after, "/", 3)
	if len(parts) < 2 {
		return false
	}
	candidate := strings.TrimPrefix(parts[1], "b")
	if !isAllDigits(candidate) || len(candidate) <= 10 {
		return false
	}
	extracted.BuildID = candidate
	extracted.InvocationID = "build-" + candidate

	if len(parts) == 3 {
		rest := parts[2]
		if plusIdx := strings.Index(rest, "+/"); plusIdx != -1 {
			streamPath := strings.Trim(rest[plusIdx+2:], "/")
			if streamPath != "" {
				extracted.logdogStreamPath = streamPath
				if strings.HasPrefix(streamPath, "u/") {
					uRest := strings.TrimPrefix(streamPath, "u/")
					segs := strings.Split(uRest, "/")
					if len(segs) >= 2 {
						extracted.StepName = strings.Join(segs[:len(segs)-1], "|")
						extracted.LogName = segs[len(segs)-1]
					} else if len(segs) == 1 && segs[0] != "" {
						extracted.LogName = segs[0]
					}
				} else {
					segs := strings.Split(streamPath, "/")
					if len(segs) > 0 && segs[len(segs)-1] != "" {
						extracted.LogName = segs[len(segs)-1]
					}
				}
			}
		}
	}
	return true
}

func resolveLogDogStepAndLog(ctx context.Context, bbClient grpcpb.BuildsClient, extracted *ExtractedIDs) {
	if bbClient == nil || extracted.BuildID == "" || extracted.logdogStreamPath == "" {
		return
	}
	buildID, err := strconv.ParseInt(extracted.BuildID, 10, 64)
	if err != nil || buildID <= 0 {
		return
	}
	req := &bbpb.GetBuildRequest{
		Id: buildID,
		Mask: &bbpb.BuildMask{
			Fields: &field_mask.FieldMask{
				Paths: []string{"id", "steps", "output.logs"},
			},
		},
	}
	b, err := bbClient.GetBuild(ctx, req)
	if err != nil || b == nil {
		return
	}
	targetSuffix := "/+/" + extracted.logdogStreamPath
	for _, s := range b.Steps {
		for _, l := range s.Logs {
			if matchesLogDogStreamSuffix(l, targetSuffix) {
				extracted.StepName = s.Name
				extracted.LogName = l.Name
				return
			}
		}
	}
	for _, l := range b.GetOutput().GetLogs() {
		if matchesLogDogStreamSuffix(l, targetSuffix) {
			extracted.StepName = ""
			extracted.LogName = l.Name
			return
		}
	}
}

func matchesLogDogStreamSuffix(l *bbpb.Log, targetSuffix string) bool {
	return strings.HasSuffix(strings.TrimRight(l.Url, "/"), targetSuffix) ||
		strings.HasSuffix(strings.TrimRight(l.ViewUrl, "/"), targetSuffix)
}

// extractFromMiloBuildURL handles Milo UI and Buildbucket build URLs
// (e.g. /ui/inv/<inv>, /ui/b/<build_id>, /b/<build_id>, /build/<build_id>, /builders/.../<build_id>).
func extractFromMiloBuildURL(clean string, extracted *ExtractedIDs) bool {
	if idx := strings.Index(clean, "/inv/"); idx != -1 {
		after := clean[idx+len("/inv/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	}
	if idx := strings.Index(clean, "/ui/b/"); idx != -1 {
		after := clean[idx+len("/ui/b/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	} else if idx := strings.Index(clean, "/b/"); idx != -1 {
		after := clean[idx+len("/b/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	}
	if idx := strings.Index(clean, "/ui/build/"); idx != -1 {
		after := clean[idx+len("/ui/build/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	} else if idx := strings.Index(clean, "/build/"); idx != -1 {
		after := clean[idx+len("/build/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	} else if idx := strings.Index(clean, "/builds/"); idx != -1 {
		after := clean[idx+len("/builds/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	}
	if idx := strings.Index(clean, "/builders/"); idx != -1 {
		before := clean[:idx]
		after := clean[idx+len("/builders/"):]
		parts := strings.Split(after, "/")
		if len(parts) >= 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
			target := parts[2]
			trimmed := strings.TrimPrefix(target, "b")
			if isAllDigits(trimmed) && (len(trimmed) > 10 || strings.HasPrefix(target, "b")) {
				extracted.BuildID = trimmed
				extracted.InvocationID = "build-" + trimmed
				return true
			}
			project := ""
			if pIdx := strings.Index(before, "/p/"); pIdx != -1 {
				pAfter := before[pIdx+len("/p/"):]
				pParts := strings.Split(pAfter, "/")
				if len(pParts) > 0 {
					project = pParts[0]
				}
			}
			if num, err := strconv.Atoi(target); err == nil && project != "" {
				project = unescapePathSegment(project)
				bucket := unescapePathSegment(parts[0])
				builder := unescapePathSegment(parts[1])
				extracted.builder = fmt.Sprintf("%s/%s/%s", project, bucket, builder)
				extracted.buildNumber = num
				return true
			}
			extracted.InvocationID = base.NormalizeInvocation(target)
			return true
		}
	}
	return false
}

// extractFromBuilderPath handles builder paths with a build number:
// <project>/<bucket>/<builder>/<build_number>
func extractFromBuilderPath(clean string, extracted *ExtractedIDs) bool {
	parts := strings.Split(clean, "/")
	if len(parts) == 4 && parts[0] != "" && parts[1] != "" && parts[2] != "" && parts[3] != "" {
		if num, err := strconv.Atoi(parts[3]); err == nil {
			project := unescapePathSegment(parts[0])
			bucket := unescapePathSegment(parts[1])
			builder := unescapePathSegment(parts[2])
			extracted.builder = fmt.Sprintf("%s/%s/%s", project, bucket, builder)
			extracted.buildNumber = num
			return true
		}
	}
	return false
}

func unescapePathSegment(s string) string {
	if unescaped, err := url.PathUnescape(s); err == nil && unescaped != "" {
		return unescaped
	}
	return s
}

func populateBuildIDFromInvocation(extracted *ExtractedIDs) {
	if extracted.BuildID == "" && strings.HasPrefix(extracted.InvocationID, "build-") {
		trimmed := strings.TrimPrefix(extracted.InvocationID, "build-")
		if isAllDigits(trimmed) && len(trimmed) > 10 {
			extracted.BuildID = trimmed
		}
	}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// extractFromBugTarget extracts numeric Buganizer IDs from bug URLs and shorthands:
//   - b/<digits>
//   - http(s)://b/<digits>
//   - http(s)://b.corp.google.com/issues/<digits>
//   - http(s)://issuetracker.google.com/issues/<digits>
//   - http(s)://issues.chromium.org/issues/<digits>
//   - http(s)://crbug.com/<digits>
//   - crbug.com/<digits>
//
// Bare numbers are not matched so bare invocation and build IDs continue to work as before.
func extractFromBugTarget(raw string, extracted *ExtractedIDs) bool {
	clean := strings.Trim(strings.TrimSpace(raw), "/")
	if clean == "" {
		return false
	}
	if !strings.Contains(clean, "://") {
		if !strings.HasPrefix(clean, "b/") && !strings.HasPrefix(clean, "crbug.com/") {
			return false
		}
		clean = "https://" + clean
	}

	u, err := url.Parse(clean)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if host == "b" || strings.HasPrefix(host, "b.") ||
		strings.Contains(host, "issuetracker.google.com") ||
		strings.Contains(host, "issues.chromium.org") ||
		strings.Contains(host, "crbug.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 && isAllDigits(parts[len(parts)-1]) {
			extracted.BugID = parts[len(parts)-1]
			return true
		}
	}
	return false
}

// extractFromLUCIAnalysisTarget handles LUCI Analysis rule and cluster URLs and resource names
// without making any network calls:
// - Web URLs:
//   - https://luci-analysis.appspot.com/p/<project>/rules/<id>
//   - https://luci-milo.appspot.com/ui/tests/p/<project>/rules/<id>
//   - https://ci.chromium.org/ui/tests/p/<project>/rules/<id>
//   - https://.../p/<project>/clusters/<alg>/<id>
//
// - Canonical resource names:
//   - projects/<project>/rules/<id>
//   - projects/<project>/clusters/<alg>/<id>[/failures]
func extractFromLUCIAnalysisTarget(raw string, extracted *ExtractedIDs) bool {
	clean := strings.TrimSpace(raw)
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	clean = strings.TrimRight(clean, "/")

	// 1. Canonical LUCI Analysis resource names:
	//    projects/<project>/rules/<id> or projects/<project>/clusters/<alg>/<id>[/failures]
	res := clean
	if idx := strings.Index(clean, "/projects/"); idx != -1 {
		res = clean[idx+1:]
	}
	if strings.HasPrefix(res, "projects/") {
		if applyAnalysisResourceSegments(strings.Split(strings.TrimPrefix(res, "projects/"), "/"), extracted) {
			return true
		}
	}

	// 2. LUCI Analysis / Milo / CI web URLs:
	//    .../p/<project>/rules/<id> or .../p/<project>/clusters/<alg>/<id>
	if idx := strings.Index(clean, "/p/"); idx != -1 {
		if applyAnalysisResourceSegments(strings.Split(clean[idx+len("/p/"):], "/"), extracted) {
			return true
		}
	}

	return false
}

// applyAnalysisResourceSegments extracts Project, RuleID, ClusterAlgorithm, and ClusterID
// from path segments starting at <project>/(rules|clusters)/... .
func applyAnalysisResourceSegments(parts []string, extracted *ExtractedIDs) bool {
	if len(parts) >= 3 && parts[0] != "" && parts[1] == "rules" && parts[2] != "" {
		extracted.Project = unescapePathSegment(parts[0])
		extracted.RuleID = unescapePathSegment(parts[2])
		// Every LUCI Analysis rule has a corresponding rule cluster at clusters/rules/<rule_id>.
		extracted.ClusterAlgorithm = "rules"
		extracted.ClusterID = extracted.RuleID
		return true
	}
	if len(parts) >= 4 && parts[0] != "" && parts[1] == "clusters" && parts[2] != "" && parts[3] != "" {
		extracted.Project = unescapePathSegment(parts[0])
		extracted.ClusterAlgorithm = unescapePathSegment(parts[2])
		extracted.ClusterID = unescapePathSegment(parts[3])
		// Populate RuleID automatically when the cluster algorithm is "rules" (or versioned "rules-...").
		if strings.HasPrefix(extracted.ClusterAlgorithm, "rules") {
			extracted.RuleID = extracted.ClusterID
		}
		return true
	}
	return false
}

// extractFromMiloTestHistoryURL handles Milo test history URLs:
// /ui/test/:projectOrRealm/:testId
func extractFromMiloTestHistoryURL(raw string, extracted *ExtractedIDs) bool {
	idx := strings.Index(raw, "/ui/test/")
	if idx == -1 {
		idx = strings.Index(raw, "/test/")
	}
	if idx == -1 {
		return false
	}
	after := raw[idx:]
	if strings.HasPrefix(after, "/ui/test/") {
		after = after[len("/ui/test/"):]
	} else if strings.HasPrefix(after, "/test/") {
		after = after[len("/test/"):]
	}
	parts := strings.SplitN(after, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return false
	}
	testIDPart := parts[1]
	if qIdx := strings.IndexAny(testIDPart, "?#"); qIdx != -1 {
		testIDPart = testIDPart[:qIdx]
	}
	resolvedTestID := ""
	if unescaped, err := url.PathUnescape(testIDPart); err == nil && unescaped != "" {
		resolvedTestID = unescaped
	} else if testIDPart != "" {
		resolvedTestID = testIDPart
	}
	if resolvedTestID == "" {
		return false
	}
	// Strip optional ":<subrealm>" suffix when the first segment is a full realm (e.g. "chromium:ci").
	if projectOrRealm := unescapePathSegment(parts[0]); projectOrRealm != "" {
		extracted.Project = strings.SplitN(projectOrRealm, ":", 2)[0]
	}
	extracted.TestID = resolvedTestID
	return true
}

// extractArtifactSuffix strips /artifacts/<art_id> from a ResultDB resource name if present and sets extracted.ArtifactID.
func extractArtifactSuffix(clean string, extracted *ExtractedIDs) string {
	if artIdx := strings.Index(clean, "/artifacts/"); artIdx != -1 {
		artID := clean[artIdx+len("/artifacts/"):]
		if idx := strings.IndexAny(artID, "/?#"); idx != -1 {
			artID = artID[:idx]
		}
		if unescaped, err := url.PathUnescape(artID); err == nil {
			artID = unescaped
		}
		if artID != "" && extracted.ArtifactID == "" {
			extracted.ArtifactID = artID
		}
		return clean[:artIdx]
	}
	return clean
}

// extractFromTestResultResourceName handles ResultDB test result resource names:
// .../tests/<escaped_test_id>/results/<result_id>
func extractFromTestResultResourceName(clean string, extracted *ExtractedIDs) bool {
	if !strings.Contains(clean, "/tests/") || !strings.Contains(clean, "/results/") {
		return false
	}
	if strings.Contains(clean, "/workUnits/") {
		_, wuID := base.ExtractWorkUnitComponents(clean)
		if wuID != "" {
			extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
		}
	}
	inv, testID, resultID := base.ExtractTestResultComponents(clean)
	if inv != "" {
		extracted.InvocationID = base.NormalizeInvocation(inv)
	}
	if testID != "" {
		extracted.TestID = testID
	}
	if resultID != "" {
		extracted.ResultID = resultID
	}
	return true
}

// extractFromTestResourceName handles ResultDB test resource names:
// .../tests/<escaped_test_id>
func extractFromTestResourceName(clean string, extracted *ExtractedIDs) bool {
	testIdx := strings.Index(clean, "/tests/")
	if testIdx == -1 {
		return false
	}
	if strings.Contains(clean, "/workUnits/") {
		rootInv, wuID := base.ExtractWorkUnitComponents(clean)
		if rootInv != "" {
			extracted.InvocationID = base.NormalizeInvocation(rootInv)
		}
		if wuID != "" {
			extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
		}
	}
	prefix := clean[:testIdx]
	after := clean[testIdx+len("/tests/"):]
	parts := strings.Split(after, "/")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	if parts[0] == "view" && (strings.Contains(prefix, "android-build.googleplex.com") || strings.Contains(prefix, "android-build.corp.google.com")) {
		// /builds/tests/view is an Android Build web endpoint, not a test ID.
		return true
	}
	if unescaped, err := url.PathUnescape(parts[0]); err == nil {
		extracted.TestID = unescaped
	} else {
		extracted.TestID = parts[0]
	}
	if idx := strings.Index(prefix, "/variants/"); idx != -1 {
		extracted.VariantHash = prefix[idx+len("/variants/"):]
		prefix = prefix[:idx]
	}
	if extracted.InvocationID == "" {
		extracted.InvocationID = base.NormalizeInvocation(prefix)
	}
	return true
}

// extractFromWorkUnitResourceName handles ResultDB work unit resource names:
// rootInvocations/<root_inv>/workUnits/<wu_id>
func extractFromWorkUnitResourceName(clean string, extracted *ExtractedIDs) bool {
	if !strings.Contains(clean, "/workUnits/") {
		return false
	}
	rootInv, wuID := base.ExtractWorkUnitComponents(clean)
	if rootInv != "" {
		extracted.InvocationID = base.NormalizeInvocation(rootInv)
	}
	if wuID != "" {
		extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
	}
	return extracted.WorkUnitID != ""
}

// extractFromInvocationResourceName handles ResultDB root invocation / invocation resource names:
// rootInvocations/<inv>, invocations/<inv>, or bare invocation IDs.
func extractFromInvocationResourceName(clean string, extracted *ExtractedIDs) bool {
	if strings.HasPrefix(clean, "rootInvocations/") || strings.HasPrefix(clean, "invocations/") {
		trimmed := clean
		if strings.HasPrefix(trimmed, "rootInvocations/") {
			trimmed = strings.TrimPrefix(trimmed, "rootInvocations/")
		} else {
			trimmed = strings.TrimPrefix(trimmed, "invocations/")
		}
		parts := strings.Split(trimmed, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return true
		}
	}
	if extracted.InvocationID == "" && clean != "" && !strings.Contains(clean, "/") {
		extracted.InvocationID = base.NormalizeInvocation(clean)
		return true
	}
	return false
}

func queryAntsResultDBVerdict(ctx context.Context, client pb.ResultDBClient, rootInvID string, info *base.AntsTestResultInfo) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(rootInvID)

	var filter string
	if info.ModuleName != "" {
		filter = fmt.Sprintf("test_id_structured.module_name = %q", info.ModuleName)
	}
	if info.MethodName != "" {
		if filter != "" {
			filter += " AND "
		}
		filter += fmt.Sprintf("test_id_structured.case_name = %q", info.MethodName)
	}

	matchModuleName := func(trModuleName string) bool {
		if info.ModuleName == "" {
			return trModuleName == "" || trModuleName == "no-module-name"
		}
		return trModuleName == info.ModuleName
	}

	matchFunc := func(tr *pb.TestResult) bool {
		if info.MethodName != "" {
			if tr.TestIdStructured != nil && tr.TestIdStructured.CaseName == info.MethodName && matchModuleName(tr.TestIdStructured.ModuleName) {
				return true
			}
			if info.ModuleName == "" {
				if (strings.HasPrefix(tr.TestId, ":no-module-name!") || strings.HasPrefix(tr.TestId, ":!")) && strings.HasSuffix(tr.TestId, "#"+info.MethodName) {
					return true
				}
			} else {
				if strings.HasPrefix(tr.TestId, ":"+info.ModuleName+"!") && strings.HasSuffix(tr.TestId, "#"+info.MethodName) {
					return true
				}
			}
			return false
		}
		if info.ModuleName != "" {
			if tr.TestIdStructured != nil && matchModuleName(tr.TestIdStructured.ModuleName) {
				return true
			}
			return strings.HasPrefix(tr.TestId, ":"+info.ModuleName+"!")
		}
		return true
	}

	var verdictResults []*pb.TestResult
	var exList []*pb.TestExoneration
	pageToken := ""

	for {
		req := &pb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  1000,
			PageToken: pageToken,
			OrderBy:   "ui_priority",
		}
		if filter != "" {
			req.Predicate = &pb.TestVerdictPredicate{
				ContainsTestResultFilter: filter,
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, nil, errors.Fmt("QueryTestVerdicts RPC failed for %s (%s): %w", rootInvName, base.ResultDBPermissionReason(rootInvName, err), err)
			}
			return nil, nil, errors.Fmt("QueryTestVerdicts RPC failed for %s: %w", rootInvName, err)
		}
		for _, tv := range res.TestVerdicts {
			matched := false
			for _, tr := range tv.Results {
				if tr.VariantHash == "" && tv.TestIdStructured != nil {
					tr.VariantHash = tv.TestIdStructured.ModuleVariantHash
				}
				if tr.TestId == "" {
					tr.TestId = tv.TestId
				}
				if tr.TestMetadata == nil {
					tr.TestMetadata = tv.TestMetadata
				}
				if matchFunc(tr) {
					verdictResults = append(verdictResults, tr)
					matched = true
				}
			}
			if matched {
				exList = append(exList, tv.Exonerations...)
			}
		}
		if len(verdictResults) > 0 || res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return verdictResults, exList, nil
}
