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
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	defaultAndroidBranch     = "git_main"
	defaultAndroidDataRealm  = "prod"
	maxSourceVerdictVariants = 4
	sourceVerdictPageSize    = 200
	maxSourceVerdictPages    = 3
)

type androidBranchHistoryOptions struct {
	thClient            analysispb.TestHistoryClient
	tvbClient           analysispb.TestVariantBranchesClient
	project             string
	testID              string
	subRealm            string
	branch              string
	branchExplicit      bool
	dataRealm           string
	rawVariants         []string
	fallbackVariantHash string
	days                int
	limit               int
	adaptive            bool
	now                 time.Time
}

func validateAndroidBranchFlag(branch string) error {
	trimmed := strings.TrimSpace(branch)
	if trimmed == "" {
		return errors.New("flag -branch cannot be empty")
	}
	if strings.HasPrefix(trimmed, "refs/") || strings.Contains(trimmed, "/") {
		return errors.Fmt("invalid -branch %q: expected an Android Build branch name (e.g. git_main), not a Gitiles ref", branch)
	}
	return nil
}

func isValidVariantHash(h string) bool {
	if len(h) != 16 {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// resolveAndroidBranch returns the Android build branch to query (explicit
// -branch, target-extracted branch, or "git_main" for -project android), or ""
// when the query does not use Android build branches.
func resolveAndroidBranch(project, explicitBranch, extractedBranch string) string {
	if b := strings.TrimSpace(explicitBranch); b != "" {
		return b
	}
	if b := strings.TrimSpace(extractedBranch); b != "" {
		return b
	}
	if project == "android" {
		return defaultAndroidBranch
	}
	return ""
}

func buildAndroidSourceRef(branch, dataRealm string) *analysispb.SourceRef {
	realm := strings.TrimSpace(dataRealm)
	if realm == "" {
		realm = defaultAndroidDataRealm
	}
	return &analysispb.SourceRef{
		System: &analysispb.SourceRef_AndroidBuild{
			AndroidBuild: &analysispb.AndroidBuildBranch{
				DataRealm: realm,
				Branch:    strings.TrimSpace(branch),
			},
		},
	}
}

// fetchAndroidBranchHistory queries TestVariantBranches.Query and
// TestHistory.QuerySourceVerdicts for Android RootInvocation test history,
// which is stored in TestVariantBranch and TestResultsBySourcePosition rather
// than legacy TestResults.
func fetchAndroidBranchHistory(ctx context.Context, opts androidBranchHistoryOptions) (*testHistoryReport, error) {
	days := min(opts.days, maxTestHistoryDays)
	variantFilters, err := parseVariantFilters(opts.rawVariants)
	if err != nil {
		return nil, err
	}

	now := opts.now.UTC()
	earliest := now.Add(-time.Duration(days) * 24 * time.Hour)
	sourceRef := buildAndroidSourceRef(opts.branch, opts.dataRealm)

	branchRes, err := fetchTestVariantBranchStats(ctx, opts.tvbClient, opts.project, opts.testID, sourceRef, variantFilters, earliest)
	if err != nil {
		return nil, err
	}

	stats := branchRes.stats
	totalCounts := branchRes.totalCounts
	truncated := branchRes.truncated
	hasPostsubmitStats := len(stats) > 0
	postsubmitNonPassingHashes := make(map[string]bool)
	for _, s := range stats {
		if s.nonPassingCount() > 0 {
			postsubmitNonPassingHashes[s.VariantHash] = true
		}
	}
	candidateHashes := selectSourceVerdictVariantHashes(stats, branchRes.variantDefs, branchRes.allMatchedHashes, variantFilters, opts.fallbackVariantHash)

	effectiveLimit := opts.limit
	if opts.adaptive && effectiveLimit > 0 {
		effectiveLimit = adaptiveExampleLimit(effectiveLimit, max(1, len(stats)))
	}

	verdictRes, err := fetchSourceVerdictTimeline(ctx, opts.thClient, opts.project, opts.testID, sourceRef, candidateHashes, branchRes.variantDefs, earliest, now, effectiveLimit, hasPostsubmitStats, postsubmitNonPassingHashes)
	if err != nil {
		return nil, err
	}

	// Only stamp LatestNonPassingDate onto postsubmit variants that actually had
	// non-passing segment counts, using clean submitted source verdicts.
	for i := range stats {
		if stats[i].nonPassingCount() > 0 {
			if latestDate := verdictRes.cleanLatestNonPassingByHash[stats[i].VariantHash]; latestDate != "" {
				stats[i].LatestNonPassingDate = latestDate
			}
		}
	}

	statsSourceNote := "branch segment counts"
	if branchRes.segmentsPrecedeWindow {
		statsSourceNote = "branch segment counts; active segments start before window"
	}
	// If TestVariantBranches had 0 postsubmit segments in window (e.g. a
	// presubmit-only test from an ATI URL), use observed source-verdict counts.
	if len(stats) == 0 && len(verdictRes.observedStats) > 0 {
		stats = verdictRes.observedStats
		totalCounts = verdictRes.observedTotalCounts
		statsSourceNote = "sampled source verdict counts"
		truncated = truncated || verdictRes.truncated
	}

	parsedStructured, _ := pbutil.ParseStructuredTestIdentifierForInput(opts.testID, nil)

	return &testHistoryReport{
		Project:           opts.project,
		TestID:            opts.testID,
		TestIDStructured:  parsedStructured,
		SubRealm:          opts.subRealm,
		Branch:            opts.branch,
		BranchExplicit:    opts.branchExplicit,
		SourceVerdictMode: true,
		StatsSourceNote:   statsSourceNote,
		Days:              days,
		StatsTruncated:    truncated,
		AllVerdictsPassed: verdictRes.allPassed,
		NewerPassingCount: verdictRes.newerPassingCount,
		TotalCounts:       totalCounts,
		VariantStats:      stats,
		Verdicts:          verdictRes.verdicts,
	}, nil
}

type branchStatsResult struct {
	stats                 []variantStatsSummary
	totalCounts           *rdbpb.TestAggregation_VerdictCounts
	variantDefs           map[string]map[string]string
	allMatchedHashes      []string
	segmentsPrecedeWindow bool
	truncated             bool
}

func fetchTestVariantBranchStats(ctx context.Context, tvbClient analysispb.TestVariantBranchesClient, project, testID string, ref *analysispb.SourceRef, variantFilters map[string]string, earliest time.Time) (*branchStatsResult, error) {
	res := &branchStatsResult{
		totalCounts: &rdbpb.TestAggregation_VerdictCounts{},
		variantDefs: make(map[string]map[string]string),
	}
	if tvbClient == nil {
		return res, nil
	}

	pageToken := ""
	for page := 0; page < maxTestHistoryPages; page++ {
		resp, err := tvbClient.Query(ctx, &analysispb.QueryTestVariantBranchRequest{
			Project:   project,
			TestId:    testID,
			Ref:       ref,
			PageSize:  testHistoryPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return res, errors.Fmt("TestVariantBranches.Query RPC failed for %s / %s: %w", project, testID, err)
		}

		for _, tvb := range resp.GetTestVariantBranch() {
			hash := tvb.GetVariantHash()
			if hash == "" {
				continue
			}
			def := tvb.GetVariant().GetDef()
			if !matchesVariantFilters(def, variantFilters) {
				continue
			}
			if len(def) > 0 {
				res.variantDefs[hash] = def
			}
			res.allMatchedHashes = append(res.allMatchedHashes, hash)

			counts := &rdbpb.TestAggregation_VerdictCounts{}
			for _, seg := range tvb.GetSegments() {
				if endHour := seg.GetEndHour(); endHour != nil && endHour.AsTime().Before(earliest) {
					continue
				}
				if startHour := seg.GetStartHour(); startHour != nil && startHour.AsTime().Before(earliest) {
					res.segmentsPrecedeWindow = true
				}
				sc := seg.GetCounts()
				failed := sc.GetUnexpectedVerdicts()
				flaky := sc.GetFlakyVerdicts()
				passed := max(0, sc.GetTotalVerdicts()-failed-flaky)

				counts.Failed += failed
				counts.Flaky += flaky
				counts.Passed += passed
				addVerdictCounts(res.totalCounts, &rdbpb.TestAggregation_VerdictCounts{
					Failed: failed,
					Flaky:  flaky,
					Passed: passed,
				})
			}

			if total := int32(format.VerdictCountsTotal(counts)); total > 0 {
				res.stats = append(res.stats, variantStatsSummary{
					VariantHash: hash,
					Variant:     def,
					Counts:      counts,
					Total:       total,
				})
			}
		}

		if resp.GetNextPageToken() == "" {
			break
		}
		if page == maxTestHistoryPages-1 {
			res.truncated = true
		}
		pageToken = resp.GetNextPageToken()
	}

	sortVariantStatsSummaries(res.stats)
	return res, nil
}

func sortVariantStatsSummaries(stats []variantStatsSummary) {
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].nonPassingCount() != stats[j].nonPassingCount() {
			return stats[i].nonPassingCount() > stats[j].nonPassingCount()
		}
		if stats[i].Total != stats[j].Total {
			return stats[i].Total > stats[j].Total
		}
		return stats[i].VariantHash < stats[j].VariantHash
	})
}

func addVerdictCounts(dst, src *rdbpb.TestAggregation_VerdictCounts) {
	dst.Failed += src.Failed
	dst.Flaky += src.Flaky
	dst.Passed += src.Passed
	dst.Skipped += src.Skipped
	dst.ExecutionErrored += src.ExecutionErrored
	dst.Precluded += src.Precluded
	dst.Exonerated += src.Exonerated
}

func selectSourceVerdictVariantHashes(stats []variantStatsSummary, variantDefs map[string]map[string]string, allMatchedHashes []string, variantFilters map[string]string, fallbackVariantHash string) []string {
	var selected []string
	seen := make(map[string]bool)
	add := func(h string) {
		if h == "" || seen[h] || len(selected) >= maxSourceVerdictVariants {
			return
		}
		seen[h] = true
		selected = append(selected, h)
	}

	if isValidVariantHash(fallbackVariantHash) && (len(variantFilters) == 0 || matchesVariantFilters(variantDefs[fallbackVariantHash], variantFilters)) {
		add(fallbackVariantHash)
	}

	hasNonPassingPostsubmit := false
	for _, s := range stats {
		if s.nonPassingCount() > 0 {
			hasNonPassingPostsubmit = true
			add(s.VariantHash)
		}
	}
	if !hasNonPassingPostsubmit {
		for _, s := range stats {
			add(s.VariantHash)
		}
		if len(selected) == 0 {
			for _, h := range allMatchedHashes {
				add(h)
			}
		}
	}
	return selected
}

type sourceVerdictTimelineResult struct {
	verdicts                    []testHistoryVerdictEntry
	allPassed                   bool
	newerPassingCount           int
	cleanLatestNonPassingByHash map[string]string
	observedStats               []variantStatsSummary
	observedTotalCounts         *rdbpb.TestAggregation_VerdictCounts
	truncated                   bool
}

type perVariantSourceVerdicts struct {
	variantHash           string
	cleanNonPassing       []testHistoryVerdictEntry
	unsubmittedNonPassing []testHistoryVerdictEntry
	passing               []testHistoryVerdictEntry
	cleanLatestNonPassing string
	anyLatestNonPassing   string
	counts                *rdbpb.TestAggregation_VerdictCounts
	truncated             bool
}

func fetchSourceVerdictTimeline(ctx context.Context, thClient analysispb.TestHistoryClient, project, testID string, ref *analysispb.SourceRef, hashes []string, variantDefs map[string]map[string]string, earliest, now time.Time, limit int, hasPostsubmitStats bool, postsubmitNonPassingHashes map[string]bool) (*sourceVerdictTimelineResult, error) {
	out := &sourceVerdictTimelineResult{
		cleanLatestNonPassingByHash: make(map[string]string),
		observedTotalCounts:         &rdbpb.TestAggregation_VerdictCounts{},
	}
	if thClient == nil || len(hashes) == 0 {
		return out, nil
	}

	perVariant := make([]perVariantSourceVerdicts, len(hashes))
	g, gCtx := errgroup.WithContext(ctx)
	for idx, vh := range hashes {
		g.Go(func() error {
			pv, err := queryVariantSourceVerdicts(gCtx, thClient, project, testID, ref, vh, variantDefs[vh], earliest, now, limit, hasPostsubmitStats, postsubmitNonPassingHashes[vh])
			if err != nil {
				return err
			}
			perVariant[idx] = pv
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var cleanNonPassing []testHistoryVerdictEntry
	var unsubmittedNonPassing []testHistoryVerdictEntry
	var allPassing []testHistoryVerdictEntry
	for _, pv := range perVariant {
		cleanNonPassing = append(cleanNonPassing, pv.cleanNonPassing...)
		unsubmittedNonPassing = append(unsubmittedNonPassing, pv.unsubmittedNonPassing...)
		allPassing = append(allPassing, pv.passing...)
		if pv.cleanLatestNonPassing != "" {
			out.cleanLatestNonPassingByHash[pv.variantHash] = pv.cleanLatestNonPassing
		}
		if pv.truncated {
			out.truncated = true
		}
		if total := int32(format.VerdictCountsTotal(pv.counts)); total > 0 {
			out.observedStats = append(out.observedStats, variantStatsSummary{
				VariantHash:          pv.variantHash,
				Variant:              variantDefs[pv.variantHash],
				Counts:               pv.counts,
				Total:                total,
				LatestNonPassingDate: pv.anyLatestNonPassing,
			})
			addVerdictCounts(out.observedTotalCounts, pv.counts)
		}
	}
	sortVariantStatsSummaries(out.observedStats)
	sortVerdictEntriesDesc(cleanNonPassing)
	sortVerdictEntriesDesc(unsubmittedNonPassing)
	sortVerdictEntriesDesc(allPassing)

	var selectedNonPassing []testHistoryVerdictEntry
	if hasPostsubmitStats && len(cleanNonPassing) > 0 {
		selectedNonPassing = cleanNonPassing
		if limit <= 0 || len(selectedNonPassing) < limit {
			selectedNonPassing = append(selectedNonPassing, unsubmittedNonPassing...)
			sortVerdictEntriesDesc(selectedNonPassing)
		}
	} else {
		selectedNonPassing = append(cleanNonPassing, unsubmittedNonPassing...)
		sortVerdictEntriesDesc(selectedNonPassing)
	}

	if len(selectedNonPassing) > 0 {
		if newestTime := selectedNonPassing[0].PartitionTime; newestTime != "" {
			for _, p := range allPassing {
				if p.PartitionTime > newestTime {
					out.newerPassingCount++
				}
			}
		}
		if limit > 0 && len(selectedNonPassing) > limit {
			selectedNonPassing = selectedNonPassing[:limit]
		}
		out.verdicts = selectedNonPassing
		return out, nil
	}

	if limit > 0 && len(allPassing) > limit {
		allPassing = allPassing[:limit]
	}
	out.verdicts = allPassing
	out.allPassed = len(allPassing) > 0
	return out, nil
}

func queryVariantSourceVerdicts(ctx context.Context, thClient analysispb.TestHistoryClient, project, testID string, ref *analysispb.SourceRef, variantHash string, variantDef map[string]string, earliest, now time.Time, limit int, hasPostsubmitStats, expectPostsubmitNonPassing bool) (perVariantSourceVerdicts, error) {
	pv := perVariantSourceVerdicts{
		variantHash: variantHash,
		counts:      &rdbpb.TestAggregation_VerdictCounts{},
	}
	pageToken := ""
	for page := 0; page < maxSourceVerdictPages; page++ {
		resp, err := thClient.QuerySourceVerdicts(ctx, &analysispb.QuerySourceVerdictsV2Request{
			Project: project,
			Test: &analysispb.QuerySourceVerdictsV2Request_TestIdFlat{
				TestIdFlat: &analysispb.FlatTestIdentifier{
					TestId:      testID,
					VariantHash: variantHash,
				},
			},
			SourceRef: ref,
			PageSize:  sourceVerdictPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return pv, errors.Fmt("TestHistory.QuerySourceVerdicts RPC failed for %s / %s (variant %s): %w", project, testID, variantHash, err)
		}

		pageHadInWindow := false
		sawBeforeEarliest := false
		for _, sv := range resp.GetSourceVerdicts() {
			for _, iv := range sv.GetInvocationVerdicts() {
				partTime := ""
				partDate := ""
				if ts := iv.GetPartitionTime(); ts != nil {
					t := ts.AsTime().UTC()
					if t.Before(earliest) {
						sawBeforeEarliest = true
						continue
					}
					if t.After(now) {
						continue
					}
					partTime = t.Format(time.RFC3339)
					partDate = t.Format("2006-01-02")
				}
				pageHadInWindow = true

				invRaw := iv.GetRootInvocation()
				if invRaw == "" {
					invRaw = iv.GetInvocation()
				}
				label, nonPassing := classifySourceInvocationStatus(iv.GetStatus(), pv.counts)
				entry := testHistoryVerdictEntry{
					InvocationID:   base.NormalizeInvocation(invRaw),
					VariantHash:    variantHash,
					Variant:        variantDef,
					Status:         label,
					HasChangelists: len(iv.GetChangelists()) > 0,
					IsSourcesDirty: iv.GetIsSourcesDirty(),
					PartitionTime:  partTime,
				}
				if nonPassing {
					if partDate > pv.anyLatestNonPassing {
						pv.anyLatestNonPassing = partDate
					}
					if !entry.HasChangelists && !entry.IsSourcesDirty {
						pv.cleanNonPassing = append(pv.cleanNonPassing, entry)
						if partDate > pv.cleanLatestNonPassing {
							pv.cleanLatestNonPassing = partDate
						}
					} else {
						pv.unsubmittedNonPassing = append(pv.unsubmittedNonPassing, entry)
					}
				} else {
					pv.passing = append(pv.passing, entry)
				}
			}
		}

		nonPassingCount := len(pv.cleanNonPassing)
		if !hasPostsubmitStats {
			nonPassingCount += len(pv.unsubmittedNonPassing)
		}
		reachedLimit := limit > 0 && nonPassingCount >= limit
		enoughPassesOnly := hasPostsubmitStats && !expectPostsubmitNonPassing && len(pv.cleanNonPassing) == 0 && len(pv.unsubmittedNonPassing) == 0 && limit > 0 && len(pv.passing) >= limit
		outOfWindow := sawBeforeEarliest && !pageHadInWindow
		if reachedLimit || enoughPassesOnly || outOfWindow || resp.GetNextPageToken() == "" {
			break
		}
		if page == maxSourceVerdictPages-1 {
			pv.truncated = true
		}
		pageToken = resp.GetNextPageToken()
	}
	return pv, nil
}

func classifySourceInvocationStatus(st analysispb.TestVerdict_Status, counts *rdbpb.TestAggregation_VerdictCounts) (label string, nonPassing bool) {
	switch st {
	case analysispb.TestVerdict_FAILED:
		counts.Failed++
		return "FAILED", true
	case analysispb.TestVerdict_FLAKY:
		counts.Flaky++
		return "FLAKY", true
	case analysispb.TestVerdict_EXECUTION_ERRORED:
		counts.ExecutionErrored++
		return "EXECUTION_ERRORED", true
	case analysispb.TestVerdict_PRECLUDED:
		counts.Precluded++
		return "PRECLUDED", true
	case analysispb.TestVerdict_SKIPPED:
		counts.Skipped++
		return "SKIPPED", false
	case analysispb.TestVerdict_PASSED:
		counts.Passed++
		return "PASSED", false
	default:
		return "UNKNOWN", false
	}
}

func sortVerdictEntriesDesc(entries []testHistoryVerdictEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PartitionTime != entries[j].PartitionTime {
			return entries[i].PartitionTime > entries[j].PartitionTime
		}
		if entries[i].InvocationID != entries[j].InvocationID {
			return entries[i].InvocationID > entries[j].InvocationID
		}
		return entries[i].VariantHash < entries[j].VariantHash
	})
}
