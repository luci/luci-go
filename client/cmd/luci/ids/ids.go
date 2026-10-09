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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ExtractedIDs holds all extracted resource identifiers.
type ExtractedIDs struct {
	Project          string `json:"project,omitempty"`
	RuleID           string `json:"rule_id,omitempty"`
	ClusterAlgorithm string `json:"cluster_algorithm,omitempty"`
	ClusterID        string `json:"cluster_id,omitempty"`
	BuildID          string `json:"build_id,omitempty"`
	InvocationID     string `json:"invocation_id,omitempty"`
	StepName         string `json:"step_name,omitempty"`
	LogName          string `json:"log_name,omitempty"`
	ModuleName       string `json:"module_name,omitempty"`
	WorkUnitID       string `json:"work_unit_id,omitempty"`
	TestID           string `json:"test_id,omitempty"`
	ResultID         string `json:"result_id,omitempty"`
	ArtifactID       string `json:"artifact_id,omitempty"`
	VariantHash      string `json:"variant_hash,omitempty"`
	Legacy           bool   `json:"legacy,omitempty"`

	// builder, buildNumber, and logdogStreamPath are unexported internal fields used during ID extraction
	// to query Buildbucket for the build ID or canonical step/log names. They are not output.
	builder          string
	buildNumber      int
	logdogStreamPath string
	legacyResolved   bool
}

// IsEmpty returns true if no identifiers were extracted.
func (e *ExtractedIDs) IsEmpty() bool {
	return e.Project == "" &&
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
		LongDesc: "Parse a URL or resource name (including Milo / Buildbucket / LogDog / LUCI Analysis URLs, ResultDB / LUCI Analysis resource names, and AnTS / ATI URLs)\n" +
			"and extract the canonical IDs (-project, -ruleid, -buildid, -invocationid, -step, -log, -workunitid, -testid, -resultid, -artifactid, -varianthash)\n" +
			"for use with other commands.",
		CommandRun: func() subcommands.CommandRun {
			r := &idsRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.bbHost, "bb-host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output extracted IDs in JSON format")
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
	host      string
	bbHost    string
	jsonOut   bool
	legacy    bool
}

func (r *idsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: luci ids [-json] <url_or_resource_name>\n")
		return 1
	}

	target := strings.TrimSpace(args[0])
	ctx := cli.GetContext(a, r, env)

	rdbClient := r.rdbClient
	bbClient := r.bbClient
	if rdbClient == nil || bbClient == nil {
		if err := r.af.Parse(); err == nil {
			if rdbClient == nil {
				if c, _, _, errClient := r.af.NewResultDBClient(ctx, r.host); errClient == nil {
					rdbClient = c
				}
			}
			if bbClient == nil {
				if c, _, errClient := r.af.NewBuildsClient(ctx, r.bbHost); errClient == nil {
					bbClient = c
				}
			}
		}
	}

	extracted, err := ExtractIDs(ctx, rdbClient, bbClient, target, r.legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs: %s\n", err)
		return 1
	}

	if extracted.IsEmpty() {
		fmt.Fprintf(os.Stderr, "no resource IDs could be extracted from %q\n", target)
		return 1
	}

	if err := printExtractedIDs(os.Stdout, extracted, r.jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
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
		fmt.Fprintf(out, "Legacy:            true (subsequent commands require -legacy)\n")
		fmt.Fprintf(out, "\nNote: This is a legacy invocation. You will need to pass the -legacy flag to subsequent commands (e.g. 'luci verdict', 'luci test-result', 'luci test-result artifact').\n")
	}

	return nil
}

// ExtractIDs parses target string and extracts all available resource IDs.
func ExtractIDs(ctx context.Context, rdbClient pb.ResultDBClient, bbClient grpcpb.BuildsClient, raw string, legacy bool) (*ExtractedIDs, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty target")
	}

	extracted := &ExtractedIDs{}

	extractQueryParams(raw, extracted)

	// 1. Android Test Investigate (ATI) URL or AnTS target
	if ok, err := extractFromAntsTarget(ctx, rdbClient, raw, extracted); err != nil {
		return nil, err
	} else if !ok && !extractFromLogDogBuildURL(raw, extracted) && !extractFromLUCIAnalysisTarget(raw, extracted) {
		clean := base.TrimResourceURL(raw)

		// 2. Milo module URL without test cases: .../modules/<module>
		if !extractFromMiloModuleURL(clean, extracted) &&
			// 3. Milo / Chromium structured URL (/modules/.../variants/.../cases/..., /tests/.../variants/...)
			!extractFromMiloStructuredURL(ctx, rdbClient, clean, legacy, extracted) &&
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

	if extracted.builder != "" && extracted.buildNumber != 0 && extracted.BuildID == "" {
		if err := resolveBuildID(ctx, bbClient, extracted); err != nil {
			return nil, err
		}
	}

	if extracted.logdogStreamPath != "" && extracted.BuildID != "" {
		resolveLogDogStepAndLog(ctx, bbClient, extracted)
	}

	populateBuildIDFromInvocation(extracted)
	resolveLegacy(ctx, rdbClient, extracted, legacy)

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
func extractFromAntsTarget(ctx context.Context, client pb.ResultDBClient, raw string, extracted *ExtractedIDs) (bool, error) {
	if base.IsAntsInvocationID(raw) {
		extracted.InvocationID = base.NormalizeInvocation(raw)
		return true, nil
	}

	if base.IsAntsWorkUnitID(raw) {
		extracted.WorkUnitID = base.NormalizeWorkUnit(raw)
		return true, nil
	}

	if !base.IsAntsURL(raw) && !base.IsAntsTestResultID(raw) {
		return false, nil
	}

	trID := ""
	invID := ""
	if base.IsAntsTestResultID(raw) {
		trID = raw
	} else {
		invID, trID = base.ExtractAntsURLComponents(raw)
	}

	if trID == "" {
		if invID != "" {
			extracted.InvocationID = base.NormalizeInvocation(invID)
			return true, nil
		}
		return false, nil
	}

	info, err := base.ResolveAntsTestResult(ctx, trID)
	if err != nil {
		return false, err
	}

	extracted.InvocationID = base.NormalizeInvocation(info.InvocationID)

	// If this is a module error or has no method name, extract only InvocationID and ModuleName.
	if info.IsModuleError || (info.ModuleName != "" && info.MethodName == "") {
		extracted.ModuleName = info.ModuleName
		return true, nil
	}

	// Query ResultDB to resolve canonical TestID, ResultID, VariantHash
	if client != nil && extracted.InvocationID != "" {
		results, _, errQuery := queryAntsResultDBVerdict(ctx, client, extracted.InvocationID, info)
		if errQuery == nil && len(results) > 0 {
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
func extractFromMiloStructuredURL(ctx context.Context, client pb.ResultDBClient, clean string, legacy bool, extracted *ExtractedIDs) bool {
	if !strings.Contains(clean, "/variants/") || (!strings.Contains(clean, "/modules/") && !strings.Contains(clean, "/cases/") && !strings.Contains(clean, "/tests/")) {
		return false
	}

	invName, variantHash, _, matchFunc, err := verdict.ParseVerdictName(clean)
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

	resolveLegacy(ctx, client, extracted, legacy)

	// If ResultDB client available, resolve canonical TestID and ResultID
	if client != nil && extracted.InvocationID != "" {
		results, _, _, errQuery := verdict.QueryVerdictResultsAndExonerations(ctx, client, extracted.InvocationID, extracted.TestID, variantHash, extracted.Legacy, 1000)
		if errQuery == nil {
			for _, tr := range results {
				if matchFunc == nil || matchFunc(tr) {
					extracted.TestID = tr.TestId
					if extracted.ResultID == "" {
						extracted.ResultID = tr.ResultId
					}
					break
				}
			}
		}
	}

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
	extracted.InvocationID = base.NormalizeInvocation(prefix)
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

// isLegacyInvocation queries ResultDB directly to determine if an invocation is a legacy invocation.
// It queries for the root invocation, and if not found, queries for the legacy invocation.
func isLegacyInvocation(ctx context.Context, client pb.ResultDBClient, invID string) bool {
	normalized := base.NormalizeInvocation(invID)
	_, err := client.GetRootInvocation(ctx, &pb.GetRootInvocationRequest{
		Name: "rootInvocations/" + normalized,
	})
	if err == nil {
		return false
	}
	code := status.Code(err)
	if code == codes.Unknown {
		code = appstatus.Code(err)
	}
	if code == codes.NotFound {
		_, errLegacy := client.GetInvocation(ctx, &pb.GetInvocationRequest{
			Name: "invocations/" + normalized,
		})
		if errLegacy == nil {
			return true
		}
	}
	return false
}

func resolveLegacy(ctx context.Context, client pb.ResultDBClient, extracted *ExtractedIDs, legacy bool) {
	if extracted == nil || extracted.legacyResolved {
		return
	}
	if extracted.Legacy || legacy {
		extracted.Legacy = true
		extracted.legacyResolved = true
		return
	}
	if client != nil && extracted.InvocationID != "" {
		extracted.Legacy = isLegacyInvocation(ctx, client, extracted.InvocationID)
		extracted.legacyResolved = true
	}
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
