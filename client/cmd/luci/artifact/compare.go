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
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func CompareCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "compare [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Compare a failing test result artifact against recent passing runs"
		longDesc = desc + " by a positional URL/ID or explicit ID flags, using LUCI Analysis (TestHistory.QueryRecentPasses) and ResultDB (CompareArtifactLines)."
	case ParentTypeWorkUnit:
		usage = "compare [-invocationid <invocation_id> -workunitid <work_unit_id>] [-testid <test_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Compare a work unit artifact against recent passing runs"
		longDesc = desc + " by a positional URL/ID or explicit ID flags, using LUCI Analysis (TestHistory.QueryRecentPasses) and ResultDB (CompareArtifactLines)."
	default:
		usage = "compare [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Compare a log artifact against recent passing runs"
		longDesc = desc + " by a positional URL/ID or explicit ID flags, using LUCI Analysis (TestHistory.QueryRecentPasses) and ResultDB (CompareArtifactLines).\n\n" +
			"Unless explicit -passing-result or -compare-artifact flags are provided, queries LUCI Analysis\n" +
			"(TestHistory.QueryRecentPasses) for recent passing test results of the specified -testid\n" +
			"(or an unexpected test variant in the invocation if -testid is omitted on a work unit artifact)\n" +
			"and passes them to ResultDB's CompareArtifactLines RPC."
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &artifactCompareRun{
				passesLimit: 5,
				pageSize:    100,
				viewStr:     "content",
			}
			r.registerFetchFlags(af, parentType)
			if parentType == ParentTypeWorkUnit {
				r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			}
			r.Flags.StringVar(&r.analysisHost, "analysis-host", chromeinfra.AnalysisHost, "LUCI Analysis host")
			r.Flags.StringVar(&r.variantHash, "varianthash", "", "Optional variant hash for QueryRecentPasses")
			r.Flags.Var(&r.passingResults, "passing-result", "Explicit passing test result resource name (repeatable; skips LUCI Analysis lookup)")
			r.Flags.Var(&r.compareArtifacts, "compare-artifact", "Explicit comparison artifact resource name (repeatable; skips LUCI Analysis lookup)")
			r.Flags.IntVar(&r.passesLimit, "passes-limit", 5, "Max passing test results to request from LUCI Analysis (default 5, max 25)")
			r.Flags.IntVar(&r.pageSize, "n", 100, "Max failure-only ranges to return (default 100)")
			r.Flags.IntVar(&r.pageSize, "limit", 100, "Max failure-only ranges to return (alias for -n)")
			r.Flags.StringVar(&r.pageToken, "page-token", "", "Page token from a previous compare call to fetch the next page")
			r.Flags.StringVar(&r.viewStr, "view", "content", "Comparison view: 'content' (RANGES_WITH_CONTENT) or 'ranges' (RANGES_ONLY)")
			return r
		},
	}
}

type artifactCompareRun struct {
	artifactCommonRun
	thClient         analysispb.TestHistoryClient
	analysisHost     string
	variantHash      string
	passingResults   stringlistflag.Flag
	compareArtifacts stringlistflag.Flag
	passesLimit      int
	pageSize         int
	pageToken        string
	viewStr          string
}

func (r *artifactCompareRun) effectiveCompareParentType() ParentType {
	if r.parentType != ParentTypeUnknown {
		return r.parentType
	}
	if r.resultID != "" {
		return ParentTypeTestResult
	}
	if r.workUnitID != "" {
		return ParentTypeWorkUnit
	}
	return ParentTypeInvocation
}

func (r *artifactCompareRun) resolveComparePositionalTarget(ctx context.Context) bool {
	if r.posTarget == "" {
		return true
	}
	bindings := ids.TargetBindings{
		InvocationID: &r.invocationID,
		WorkUnitID:   &r.workUnitID,
		ArtifactID:   &r.artifactID,
		VariantHash:  &r.variantHash,
	}
	switch r.parentType {
	case ParentTypeWorkUnit:
		bindings.TestID = &r.testID
		bindings.DefaultRootForWorkUnit = true
	case ParentTypeTestResult:
		bindings.TestID = &r.testID
		bindings.ResultID = &r.resultID
	default:
		if r.workUnitID == "" {
			bindings.TestID = &r.testID
			bindings.ResultID = &r.resultID
			bindings.IgnoreWorkUnitOnSummaryPlaceholder = true
		}
	}
	extracted, err := ids.ExtractWithAuth(ctx, r.af, r.client, nil, r.posTarget, bindings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", r.posTarget, err)
		return false
	}
	r.extracted = extracted
	return true
}

func (r *artifactCompareRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if handled, exitCode := r.checkHelpAndPositionalArgs(args); handled {
		return exitCode
	}

	var view pb.CompareArtifactLinesRequest_View
	switch strings.ToLower(strings.TrimSpace(r.viewStr)) {
	case "content", "ranges_with_content", "ranges-with-content", "":
		view = pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT
	case "ranges", "ranges_only", "ranges-only":
		view = pb.CompareArtifactLinesRequest_RANGES_ONLY
	default:
		fmt.Fprintf(os.Stderr, "invalid -view %q: must be 'content' or 'ranges'\n", r.viewStr)
		return 1
	}

	effectiveParentType := r.effectiveCompareParentType()
	if r.posTarget == "" {
		if err := ValidateArtifactFlags(effectiveParentType, r.invocationID, r.workUnitID, r.testID, r.resultID, r.artifactID); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	if !r.ensureClients(ctx, false) {
		return 1
	}
	if r.posTarget != "" {
		if !r.resolveComparePositionalTarget(ctx) {
			return 1
		}
		effectiveParentType = r.effectiveCompareParentType()
		if err := ValidateArtifactFlags(effectiveParentType, r.invocationID, r.workUnitID, r.testID, r.resultID, r.artifactID); err != nil {
			ids.PrintMissingFlagsError(os.Stderr, r.extracted, err.Error())
			return 1
		}
	}
	rdbClient := r.client

	resolved, err := base.ResolveInvocation(ctx, rdbClient, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve invocation: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if r.extracted != nil {
		r.extracted.Legacy = resolved.Legacy
	}
	if resolved.Legacy && effectiveParentType == ParentTypeWorkUnit {
		fmt.Fprintf(os.Stderr, "work unit artifacts are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	var artName string
	var resolvedArtID string
	discoveredVariantHash := r.variantHash
	if effectiveParentType == ParentTypeInvocation && !resolved.Legacy {
		art, err := ResolveInvocationArtifact(ctx, rdbClient, resolved.ID, r.artifactID, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
		artName = art.Name
		resolvedArtID = art.ArtifactId
	} else if effectiveParentType == ParentTypeInvocation && resolved.Legacy {
		artName, err = resolveLegacyInvocationArtifact(ctx, rdbClient, resolved.ID, r.testID, r.artifactID, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
	} else if effectiveParentType == ParentTypeTestResult && (resolved.Legacy || r.workUnitID == "") {
		trName, vh, err := resolveTestResultAndVariantHash(ctx, rdbClient, resolved.ID, r.testID, r.resultID, discoveredVariantHash, resolved.Legacy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
		artName = trName + "/artifacts/" + url.PathEscape(r.artifactID)
		if discoveredVariantHash == "" {
			discoveredVariantHash = vh
		}
	} else {
		artName, err = ResolveArtifactResourceName(ctx, rdbClient, effectiveParentType, resolved.ID, r.workUnitID, r.testID, r.resultID, r.artifactID, resolved.Legacy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
	}

	testIDForAnalysis := r.testID
	resultIDForAnalysis := r.resultID
	if testIDForAnalysis == "" || resultIDForAnalysis == "" {
		meta := base.ExtractArtifactMetadata(artName)
		if testIDForAnalysis == "" && meta.TestID != "" {
			testIDForAnalysis = meta.TestID
		}
		if resultIDForAnalysis == "" && meta.ResultID != "" {
			resultIDForAnalysis = meta.ResultID
		}
	}

	passingResults := []string(r.passingResults)
	compareArtifacts := []string(r.compareArtifacts)

	if len(passingResults) == 0 && len(compareArtifacts) == 0 {
		thClient := r.thClient
		if thClient == nil {
			_, _, thClient, err = r.af.NewAnalysisClients(ctx, r.analysisHost)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create luci analysis client: %s\n", err)
				return 1
			}
		}
		var resolvedProject, resolvedTestID, resolvedVariantHash string
		passingResults, resolvedProject, resolvedTestID, resolvedVariantHash, err = resolveRecentPassesFromAnalysis(
			ctx, rdbClient, thClient, resolved, testIDForAnalysis, resultIDForAnalysis, discoveredVariantHash, r.passesLimit,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve passing results from LUCI Analysis: %s\n", err)
			return 1
		}
		if len(passingResults) == 0 {
			fmt.Fprintf(os.Stderr, "no recent passing results found in LUCI Analysis for project=%q test=%q variant_hash=%q\n",
				resolvedProject, resolvedTestID, resolvedVariantHash)
			return 1
		}
		if discoveredVariantHash == "" && resolvedVariantHash != "" {
			discoveredVariantHash = resolvedVariantHash
		}
	}

	resp, err := rdbClient.CompareArtifactLines(ctx, &pb.CompareArtifactLinesRequest{
		Name:           artName,
		PassingResults: passingResults,
		Artifacts:      compareArtifacts,
		View:           view,
		PageSize:       int32(r.pageSize),
		PageToken:      r.pageToken,
	})
	if err != nil && effectiveParentType != ParentTypeInvocation &&
		(status.Code(err) == codes.NotFound || status.Code(err) == codes.InvalidArgument) {
		if _, baseName, ok := extractFallbackPath(r.artifactID); ok && baseName != r.artifactID {
			if idx := strings.LastIndex(artName, "/artifacts/"); idx != -1 {
				fbName := artName[:idx] + "/artifacts/" + url.PathEscape(baseName)
				if fbResp, fbErr := rdbClient.CompareArtifactLines(ctx, &pb.CompareArtifactLinesRequest{
					Name:           fbName,
					PassingResults: passingResults,
					Artifacts:      compareArtifacts,
					View:           view,
					PageSize:       int32(r.pageSize),
					PageToken:      r.pageToken,
				}); fbErr == nil {
					fmt.Fprintf(os.Stderr, "Note: resolved path %q to artifact %q\n", r.artifactID, baseName)
					artName = fbName
					resolvedArtID = baseName
					resp = fbResp
					err = nil
				}
			}
		}
		if err != nil && !resolved.Legacy {
			if fbArt, fbErr := ResolveInvocationArtifact(ctx, rdbClient, resolved.ID, r.artifactID, os.Stderr); fbErr == nil && fbArt.Name != artName {
				artName = fbArt.Name
				resolvedArtID = fbArt.ArtifactId
				resp, err = rdbClient.CompareArtifactLines(ctx, &pb.CompareArtifactLinesRequest{
					Name:           artName,
					PassingResults: passingResults,
					Artifacts:      compareArtifacts,
					View:           view,
					PageSize:       int32(r.pageSize),
					PageToken:      r.pageToken,
				})
				if err == nil && r.extracted != nil {
					r.extracted.WorkUnitID = ""
					r.extracted.TestID = ""
					r.extracted.ResultID = ""
					r.extracted.ModuleName = ""
				}
			}
		}
	}
	if err != nil {
		if base.IsPermissionDenied(err) {
			fmt.Fprintf(os.Stderr, "CompareArtifactLines RPC failed (%s): %s\n", base.ResultDBPermissionReason(artName, err), err)
		} else {
			fmt.Fprintf(os.Stderr, "CompareArtifactLines RPC failed: %s\n", err)
		}
		return 1
	}

	stdoutOut := r.out
	if stdoutOut == nil {
		stdoutOut = os.Stdout
	}
	out := stdoutOut
	if r.outputFile != "" {
		f, err := os.Create(r.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create output file: %s\n", err)
			return 1
		}
		defer f.Close()
		out = f
	}

	if r.extracted != nil {
		if r.extracted.VariantHash == "" && discoveredVariantHash != "" {
			r.extracted.VariantHash = discoveredVariantHash
		}
		fallbackArtID := resolvedArtID
		if fallbackArtID == "" {
			fallbackArtID = base.ExtractArtifactMetadata(artName).ArtifactID
		}
		if fallbackArtID == "" {
			fallbackArtID = r.artifactID
		}
		r.extracted.ArtifactID = ""
		r.extracted.EnrichFromArtifactName(artName, fallbackArtID)
	}
	ids.PrintDerivedIDs(stdoutOut, r.extracted)

	if len(resp.Artifacts) == 0 {
		fmt.Fprintf(out, "No matching comparison artifacts found in passing runs (all lines in the failing artifact will appear below).\n\n")
	} else {
		fmt.Fprintf(out, "Removed lines matching these %d passing artifact(s):\n", len(resp.Artifacts))
		for _, a := range resp.Artifacts {
			fmt.Fprintf(out, "  - %s\n", a)
		}
		fmt.Fprintf(out, "\nNote: Lines are compared after stripping numbers, dates, hex/hash tokens, IP/MAC addresses, URLs, and system paths (/tmp, /dev, etc.).\n")
		fmt.Fprintf(out, "Lines differing by non-numeric text (e.g. device/target names, short IDs, or run-specific flags) will not match passing runs and will still appear below as false positives. Surrounding lines that also occurred in passing runs are omitted between ranges.\n\n")
	}

	fmt.Fprintf(out, "Failure-Only Ranges (%d):\n", len(resp.FailureOnlyRanges))
	for _, rng := range resp.FailureOnlyRanges {
		fmt.Fprintf(out, "\n--- Lines %d..%d (bytes %d..%d) ---\n", rng.StartLine+1, rng.EndLine, rng.StartByte, rng.EndByte)
		if view == pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT {
			for i, line := range rng.Lines {
				fmt.Fprintf(out, "%6d | %s\n", int(rng.StartLine)+1+i, line)
			}
		}
	}
	if resp.NextPageToken != "" {
		fmt.Fprintf(out, "\n(More ranges available; pass -page-token=%s to fetch the next page)\n", resp.NextPageToken)
	}
	return 0
}

// resolveTestResultAndVariantHash resolves both the canonical test result resource name
// and its VariantHash in a single QueryVerdictResultsAndExonerations call.
func resolveTestResultAndVariantHash(ctx context.Context, client pb.ResultDBClient, invID, testID, resultID, variantHash string, legacy bool) (string, string, error) {
	results, _, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, client, invID, testID, variantHash, legacy, 0)
	if err != nil {
		return "", "", errors.Fmt("failed to query test results: %w", err)
	}
	for _, tr := range results {
		if tr.ResultId == resultID {
			return tr.Name, tr.VariantHash, nil
		}
	}
	return "", "", errors.Fmt("test result %q not found for test %q in invocation %q", resultID, testID, invID)
}

// resolveLegacyInvocationArtifact resolves an artifact across a legacy invocation
// (and its included invocations) when -resultid and -workunitid are omitted.
func resolveLegacyInvocationArtifact(ctx context.Context, client pb.ResultDBClient, normInvID, testID, artID string, errOut io.Writer) (string, error) {
	directName := "invocations/" + normInvID + "/artifacts/" + url.PathEscape(artID)
	if client == nil {
		return directName, nil
	}
	res, err := client.QueryArtifacts(ctx, &pb.QueryArtifactsRequest{
		Invocations: []string{"invocations/" + normInvID},
		Predicate: &pb.ArtifactPredicate{
			FollowEdges: &pb.ArtifactPredicate_EdgeTypeSet{
				IncludedInvocations: true,
				TestResults:         false,
			},
			ArtifactIdRegexp: regexp.QuoteMeta(artID),
		},
		PageSize: int32(DefaultMaxArtifacts),
	})
	if err != nil {
		return directName, nil
	}
	var matches []*pb.Artifact
	for _, a := range res.Artifacts {
		if a.ArtifactId == artID {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		if trimmedPath, baseName, ok := extractFallbackPath(artID); ok {
			fbRegexp := "(?:.*/)?" + regexp.QuoteMeta(baseName)
			if !strings.Contains(artID, "/") {
				fbRegexp = ".+/" + regexp.QuoteMeta(baseName)
			}
			fbRes, fbErr := client.QueryArtifacts(ctx, &pb.QueryArtifactsRequest{
				Invocations: []string{"invocations/" + normInvID},
				Predicate: &pb.ArtifactPredicate{
					FollowEdges: &pb.ArtifactPredicate_EdgeTypeSet{
						IncludedInvocations: true,
						TestResults:         false,
					},
					ArtifactIdRegexp: fbRegexp,
				},
				PageSize: int32(DefaultMaxArtifacts),
			})
			if fbErr == nil {
				matches = rankPathFallbackMatches(fbRes.Artifacts, trimmedPath, baseName)
				if len(matches) == 1 && errOut != nil {
					fmt.Fprintf(errOut, "Note: resolved path %q to artifact %q\n", artID, matches[0].ArtifactId)
				}
			}
		}
	}
	if len(matches) == 1 {
		return matches[0].Name, nil
	}
	if len(matches) > 1 && testID != "" {
		// If -testid is given, check if one of the matches is on the leaf invocation where testID ran.
		results, _, _, rErr := verdict.QueryVerdictResultsAndExonerations(ctx, client, normInvID, testID, "", true, 0)
		if rErr == nil {
			leafInvs := make(map[string]bool)
			for _, tr := range results {
				if idx := strings.Index(tr.Name, "/tests/"); idx > 0 {
					leafInvs[tr.Name[:idx]] = true
				}
			}
			var filtered []*pb.Artifact
			for _, m := range matches {
				if idx := strings.Index(m.Name, "/artifacts/"); idx > 0 && leafInvs[m.Name[:idx]] {
					filtered = append(filtered, m)
				}
			}
			if len(filtered) == 1 {
				return filtered[0].Name, nil
			}
		}
	}
	if len(matches) > 1 {
		return "", formatAmbiguousArtifactsError(matches, res.NextPageToken != "", normInvID, artID)
	}
	return directName, nil
}

// resolveRecentPassesFromAnalysis replicates the Test Investigate UI flow:
// 1. Fetches the invocation's realm to determine the LUCI project.
// 2. Fetches the test variant's VariantHash and Sources from ResultDB.
// 3. Queries LUCI Analysis TestHistory.QueryRecentPasses for recent passing test results.
func resolveRecentPassesFromAnalysis(
	ctx context.Context,
	rdbClient pb.ResultDBClient,
	thClient analysispb.TestHistoryClient,
	resolved *base.ResolvedInvocation,
	testID, resultID, variantHash string,
	limit int,
) (passingResults []string, project, resolvedTestID, resolvedVariantHash string, err error) {
	normInv := resolved.ID
	legacy := resolved.Legacy
	var parent string
	var realm string
	var rdbSources *pb.Sources
	if legacy {
		parent = "invocations/" + normInv
		inv := resolved.Invocation
		if inv == nil {
			var err error
			inv, err = rdbClient.GetInvocation(ctx, &pb.GetInvocationRequest{Name: parent})
			if err != nil {
				return nil, "", "", "", errors.Fmt("GetInvocation(%s): %w", parent, err)
			}
		}
		realm = inv.GetRealm()
		rdbSources = inv.GetSourceSpec().GetSources()
	} else {
		parent = "rootInvocations/" + normInv
		rootInv := resolved.RootInvocation
		if rootInv == nil {
			var err error
			rootInv, err = rdbClient.GetRootInvocation(ctx, &pb.GetRootInvocationRequest{Name: parent})
			if err != nil {
				return nil, "", "", "", errors.Fmt("GetRootInvocation(%s): %w", parent, err)
			}
		}
		realm = rootInv.GetRealm()
		rdbSources = rootInv.GetSources()
	}
	project = strings.Split(realm, ":")[0]

	resolvedTestID = testID
	resolvedVariantHash = variantHash

	if resolvedTestID != "" {
		if resolvedVariantHash == "" {
			maxFetch := 1
			if resultID != "" {
				maxFetch = 0
			}
			results, _, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, rdbClient, normInv, resolvedTestID, "", legacy, maxFetch)
			if err != nil {
				return nil, "", "", "", errors.Fmt("querying test results for %q: %w", resolvedTestID, err)
			}
			if len(results) == 0 {
				return nil, "", "", "", errors.Fmt("no test results found for test %q in invocation %q", resolvedTestID, normInv)
			}
			if resultID != "" {
				for _, tr := range results {
					if tr.ResultId == resultID {
						resolvedVariantHash = tr.VariantHash
						break
					}
				}
				if resolvedVariantHash == "" {
					return nil, "", "", "", errors.Fmt("test result %q not found for test %q in invocation %q", resultID, resolvedTestID, normInv)
				}
			} else {
				resolvedVariantHash = results[0].VariantHash
			}
		}

		if rdbSources == nil || rdbSources.GetBaseSources() == nil {
			bResp, err := rdbClient.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Parent:     getRootInvocationParentField(parent, legacy),
				Invocation: getLegacyInvocationField(parent, legacy),
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{
						TestId:      resolvedTestID,
						VariantHash: resolvedVariantHash,
					},
				},
			})
			if err != nil {
				return nil, "", "", "", errors.Fmt("BatchGetTestVariants: %w", err)
			}
			if len(bResp.TestVariants) == 0 {
				return nil, "", "", "", errors.Fmt("test variant %q (hash %s) not found in %s", resolvedTestID, resolvedVariantHash, parent)
			}
			tv := bResp.TestVariants[0]
			if tv.SourcesId != "" {
				rdbSources = bResp.Sources[tv.SourcesId]
			}
		}
	} else {
		// No -testid specified (e.g. comparing a work unit artifact): pick a top-priority (unexpected or first) test verdict/variant in the invocation.
		if !legacy {
			vResp, err := rdbClient.QueryTestVerdicts(ctx, &pb.QueryTestVerdictsRequest{
				Parent:   parent,
				OrderBy:  "ui_priority",
				PageSize: 1,
			})
			if err == nil && len(vResp.GetTestVerdicts()) > 0 {
				tv := vResp.TestVerdicts[0]
				resolvedTestID = tv.TestId
				resolvedVariantHash = tv.GetTestIdStructured().GetModuleVariantHash()
				if resolvedVariantHash == "" && len(tv.Results) > 0 {
					resolvedVariantHash = tv.Results[0].VariantHash
				}
			}
		}
		if resolvedTestID == "" || rdbSources == nil || rdbSources.GetBaseSources() == nil {
			qtvReq := &pb.QueryTestVariantsRequest{
				Parent:      getRootInvocationParentField(parent, legacy),
				Invocations: getLegacyInvocationsField(parent, legacy),
				Predicate: &pb.TestVariantPredicate{
					Status: pb.TestVariantStatus_UNEXPECTED_MASK,
				},
				PageSize: 1,
			}
			qtvResp, err := rdbClient.QueryTestVariants(ctx, qtvReq)
			if err != nil || len(qtvResp.TestVariants) == 0 {
				qtvReq.Predicate = nil
				qtvResp, err = rdbClient.QueryTestVariants(ctx, qtvReq)
				if err != nil && resolvedTestID == "" {
					return nil, "", "", "", errors.Fmt("QueryTestVariants: %w", err)
				}
			}
			if len(qtvResp.GetTestVariants()) == 0 && resolvedTestID == "" {
				return nil, "", "", "", errors.Fmt("no test variants found in %s; please specify -testid", parent)
			}
			if len(qtvResp.GetTestVariants()) > 0 {
				tv := qtvResp.TestVariants[0]
				if resolvedTestID == "" {
					resolvedTestID = tv.TestId
					resolvedVariantHash = tv.VariantHash
				}
				if (rdbSources == nil || rdbSources.GetBaseSources() == nil) && tv.SourcesId != "" {
					rdbSources = qtvResp.Sources[tv.SourcesId]
				}
			}
		}
	}

	if rdbSources == nil || rdbSources.GetBaseSources() == nil {
		return nil, "", "", "", errors.Fmt("invocation %q has no source position (gitiles_commit or submitted_android_build) recorded in ResultDB; pass -passing-result or -compare-artifact explicitly", parent)
	}

	var analysisSources analysispb.Sources
	bJSON, err := protojson.Marshal(rdbSources)
	if err != nil {
		return nil, "", "", "", errors.Fmt("marshaling sources: %w", err)
	}
	if err := protojson.Unmarshal(bJSON, &analysisSources); err != nil {
		return nil, "", "", "", errors.Fmt("unmarshaling sources for LUCI Analysis: %w", err)
	}

	if limit <= 0 {
		limit = 25
	}
	passesResp, err := thClient.QueryRecentPasses(ctx, &analysispb.QueryRecentPassesRequest{
		Project:     project,
		TestId:      resolvedTestID,
		VariantHash: resolvedVariantHash,
		Sources:     &analysisSources,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, "", "", "", errors.Fmt("TestHistory.QueryRecentPasses: %w", err)
	}

	for _, pr := range passesResp.PassingResults {
		passingResults = append(passingResults, pr.Name)
	}
	return passingResults, project, resolvedTestID, resolvedVariantHash, nil
}

func getRootInvocationParentField(parent string, legacy bool) string {
	if legacy {
		return ""
	}
	return parent
}

func getLegacyInvocationField(parent string, legacy bool) string {
	if legacy {
		return parent
	}
	return ""
}

func getLegacyInvocationsField(parent string, legacy bool) []string {
	if legacy {
		return []string{parent}
	}
	return nil
}
