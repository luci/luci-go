// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/git/footer"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gitiles"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// staleTryjobAge is the age after which a Tryjob is too old to reuse.
const staleTryjobAge time.Duration = 24 * time.Hour

type reusability int

const (
	reuseAllowed reusability = iota + 1
	reuseMaybe
	reuseDenied
)

// canReuseTryjob checks if candidate Tryjob tj can be reused to satisfy target requirement def.
//
// Tryjob *can* be reused iff the Tryjob ends successfully and is fresh enough
// against target requirement def (i.e created within `staleTryjobAge` or within the commit distance limit).
// Tryjob *may* be reused if the Tryjob is fresh enough and still running or
// hasn't started yet.
// Note: Freshness is evaluated against target requirement def rather than historical snapshot tj.Definition
// to ensure live project configuration updates take immediate effect.
func (w *worker) canReuseTryjob(ctx context.Context, tj *tryjob.Tryjob, mode run.Mode, def *tryjob.Definition) reusability {
	switch status := tj.Status; {
	case status == tryjob.Status_STATUS_UNSPECIFIED:
		panic(fmt.Errorf("unspecified status for tryjob %d", tj.ID))
	case status == tryjob.Status_PENDING:
		return reuseMaybe
	case status == tryjob.Status_TRIGGERED && !w.evaluateReuseWindow(ctx, tj, def):
		return reuseDenied
	case status == tryjob.Status_TRIGGERED:
		return reuseMaybe
	case status == tryjob.Status_ENDED && w.canReuseResult(ctx, tj, mode, def):
		return reuseAllowed
	case status == tryjob.Status_ENDED:
		return reuseDenied
	case status == tryjob.Status_CANCELLED:
		return reuseDenied
	case status == tryjob.Status_UNTRIGGERED:
		return reuseDenied
	default:
		panic(fmt.Errorf("unknown status %s for tryjob %d", tj.Status, tj.ID))
	}
}

// canReuseResult checks if the result of the Tryjob can be reused.
func (w *worker) canReuseResult(ctx context.Context, tj *tryjob.Tryjob, mode run.Mode, def *tryjob.Definition) bool {
	switch result := tj.Result; {
	case tj.Status != tryjob.Status_ENDED:
		panic(fmt.Errorf("canReuseResult must be called when tryjob status is ended. got %s", tj.Status))
	case result == nil:
		logging.Errorf(ctx, "tryjob %d has nil result but it has ended already", tj.ID)
		return false
	case !w.evaluateReuseWindow(ctx, tj, def):
		return false
	case result.GetStatus() != tryjob.Result_SUCCEEDED:
		return false // Only a succeeded Tryjob can be reused.
	case isModeAllowed(mode, result.GetOutput().GetReusability().GetModeAllowlist()):
		return true
	}
	return false
}

// isTryjobStale checks whether the Tryjob is too old to use.
func isTryjobStale(ctx context.Context, tj *tryjob.Tryjob) bool {
	createTime := tj.Result.GetCreateTime()
	if createTime == nil {
		logging.Errorf(ctx, "Tryjob %d has nil create time when checking whether tryjob is stale", tj.ID)
		return true // Be defensive. Consider Tryjob stale.
	}
	return clock.Now(ctx).Sub(createTime.AsTime()) >= staleTryjobAge
}

// evaluateReuseWindow checks whether candidate Tryjob tj is fresh enough to
// satisfy target requirement def.
//
// Freshness is evaluated against target requirement def rather than historical
// snapshot tj.Definition to ensure live project configuration updates take
// immediate effect.
//
// Reuse window refers to both the commit-distance-based window and the
// time-based window. If max_commit_distance is configured on def, we first check
// whether the distance between the target branch tip and tj's base commit position
// is within the configured limit. If max_commit_distance is not set, or if commit
// distance calculation fails unexpectedly (e.g., due to Gitiles errors or missing
// metadata), we gracefully fall back to the time-based window.
//
// Returns true if the Tryjob satisfies the applicable reuse window, and false
// otherwise.
func (w *worker) evaluateReuseWindow(ctx context.Context, tj *tryjob.Tryjob, def *tryjob.Definition) bool {
	if maxDist := def.GetReuseWindow().GetMaxCommitDistance(); maxDist > 0 {
		reusable, err := w.verifyCommitDistance(ctx, tj, maxDist)
		if err == nil {
			return reusable
		}
		logging.Warningf(ctx, "Commit distance verification failed for tryjob %d: %s; falling back to time-based reuse window", tj.ID, err)
	}
	return !isTryjobStale(ctx, tj)
}

func (w *worker) verifyCommitDistance(ctx context.Context, tj *tryjob.Tryjob, maxDist uint32) (bool, error) {
	gc := tj.Result.GetBuildbucket().GetOutputGitilesCommit()
	if gc == nil || gc.GetHost() == "" || gc.GetProject() == "" || gc.GetRef() == "" || gc.GetPosition() == 0 {
		if tj.Status == tryjob.Status_ENDED {
			logging.Warningf(ctx, "tryjob %d lacks complete OutputGitilesCommit metadata", tj.ID)
		} else {
			logging.Debugf(ctx, "tryjob %d lacks complete OutputGitilesCommit metadata", tj.ID)
		}
		return false, errors.New("missing complete OutputGitilesCommit metadata")
	}

	if w == nil || w.gitilesFactory == nil || w.run == nil {
		logging.Warningf(ctx, "branch tip resolver is nil for tryjob %d", tj.ID)
		return false, errors.New("branch tip resolver is nil")
	}

	luciProject := w.run.ID.LUCIProject()
	tipPos, err := resolveBranchTip(ctx, w.gitilesFactory, gc.GetHost(), gc.GetProject(), gc.GetRef(), luciProject)
	if err != nil {
		logging.Infof(ctx, "failed to resolve branch tip for tryjob %d (%s/%s/%s): %s", tj.ID, gc.GetHost(), gc.GetProject(), gc.GetRef(), err)
		return false, fmt.Errorf("failed to resolve branch tip: %w", err)
	}

	buildPos := int64(gc.GetPosition())
	if tipPos < buildPos {
		logging.Infof(ctx, "resolved branch tip commit position (%d) is behind tryjob %d commit position (%d)", tipPos, tj.ID, buildPos)
		return false, fmt.Errorf("resolved branch tip commit position (%d) is behind build commit position (%d)", tipPos, buildPos)
	}

	dist := tipPos - buildPos
	if dist > int64(maxDist) {
		return false, nil
	}
	return true, nil
}

// isModeAllowed checks whether the Run Mode is in the given allowlist.
func isModeAllowed(mode run.Mode, allowlist []string) bool {
	if len(allowlist) == 0 { // Empty list means allowing all modes.
		return true
	}
	for _, allowed := range allowlist {
		if string(mode) == allowed {
			return true
		}
	}
	return false
}

// computeReuseKey computes the reuse key for a Tryjob based on the CLs it
// involves.
func computeReuseKey(cls []*run.RunCL, disableReuseFooters []string) string {
	disableReuseFooterSet := stringset.NewFromSlice(disableReuseFooters...)
	clPatchsets := make([][]byte, len(cls))
	for i, cl := range cls {
		hashInputs := []string{
			fmt.Sprintf("%d", cl.ID),
			fmt.Sprintf("%d", cl.Detail.GetMinEquivalentPatchset()),
		}
		var footers []*changelist.StringPair
		for _, f := range cl.Detail.Metadata {
			if disableReuseFooterSet.Has(f.Key) {
				footers = append(footers, f)
			}
		}
		sortFooters(footers)
		for _, f := range footers {
			hashInputs = append(hashInputs, fmt.Sprintf("%s:%s", f.Key, f.Value))
		}
		clPatchsets[i] = []byte(strings.Join(hashInputs, "/"))
	}
	sort.Slice(clPatchsets, func(i, j int) bool {
		return bytes.Compare(clPatchsets[i], clPatchsets[j]) < 0
	})
	h := sha256.New()
	h.Write(bytes.Join(clPatchsets, []byte{0}))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func sortFooters(footers []*changelist.StringPair) {
	slices.SortFunc(footers, func(f1, f2 *changelist.StringPair) int {
		if keyCmp := cmp.Compare(f1.Key, f2.Key); keyCmp != 0 {
			return keyCmp
		}
		return cmp.Compare(f1.Value, f2.Value)
	})
}

// commitPositionPattern matches the Cr-Commit-Position footer format.
// See https://chromium.googlesource.com/infra/gerrit-plugins/git-numberer/
var commitPositionPattern = regexp.MustCompile(`^(.+)@{#(\d+)}$`)

// gitilesTipCache caches resolved target branch tip commit positions.
var gitilesTipCache = layered.RegisterCache(layered.Parameters[int64]{
	ProcessCacheCapacity: 1000,
	GlobalNamespace:      "gitiles_branch_tip_v1",
	Marshal: func(pos int64) ([]byte, error) {
		return []byte(strconv.FormatInt(pos, 10)), nil
	},
	Unmarshal: func(blob []byte) (int64, error) {
		return strconv.ParseInt(string(blob), 10, 64)
	},
})

const gitilesTipCacheTTL = 15 * time.Second

// resolveBranchTip queries Gitiles to resolve the target branch tip commit
// position.
//
// Results are cached in a process-level layered cache for a short duration.
// Network queries against Gitiles are executed with a bounded 5-second timeout.
//
// Returns an error if the branch ref extracted from the Cr-Commit-Position
// footer does not match the requested target ref. Note that when a new branch
// is branched off main, its tip commit initially retains main's
// Cr-Commit-Position footer. This ref mismatch causes resolveBranchTip to
// return an error, safely falling back to the time-based window until the
// first new commit lands on the branch.
func resolveBranchTip(ctx context.Context, gf gitiles.Factory, host, project, ref, luciProject string) (int64, error) {
	if gf == nil {
		return 0, errors.New("gitiles factory is nil")
	}

	cacheKey := fmt.Sprintf("%s/%s/%s/%s", luciProject, host, url.PathEscape(project), url.PathEscape(ref))

	pos, err := gitilesTipCache.GetOrCreate(ctx, cacheKey, func() (int64, time.Duration, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		client, err := gf.MakeClient(ctx, host, luciProject)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to make Gitiles client: %w", err)
		}
		resp, err := client.Log(ctx, &gitilespb.LogRequest{
			Project:    project,
			Committish: ref,
			PageSize:   1,
		})
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get Gitiles log: %w", err)
		}

		if len(resp.GetLog()) == 0 {
			return 0, 0, errors.New("empty Gitiles log")
		}

		commit := resp.GetLog()[0]
		message := commit.GetMessage()

		footers := footer.ParseMessage(message)
		cpFooters := footers[footer.NormalizeKey("Cr-Commit-Position")]
		if len(cpFooters) == 0 {
			return 0, 0, errors.New("missing Cr-Commit-Position footer")
		}
		if len(cpFooters) > 1 {
			return 0, 0, fmt.Errorf("multiple Cr-Commit-Position footers found (%d)", len(cpFooters))
		}
		rawFooter := cpFooters[0]

		match := commitPositionPattern.FindStringSubmatch(rawFooter)
		if match == nil {
			return 0, 0, fmt.Errorf("malformed Cr-Commit-Position: %q", rawFooter)
		}

		extractedRef := match[1]
		posStr := match[2]

		// Ensure the extracted branch ref matches the requested target branch
		// ref. On newly cut branches prior to their first commit, extractedRef
		// still points to the parent branch (e.g., refs/heads/main). Rejecting
		// the mismatch causes CV to safely fall back to the time-based window
		// until a commit lands on the branch.
		if extractedRef != ref {
			return 0, 0, fmt.Errorf("branch ref %q in Cr-Commit-Position footer does not match requested target ref %q", extractedRef, ref)
		}

		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse commit position %q: %w", posStr, err)
		}

		return pos, gitilesTipCacheTTL, nil
	})
	if err != nil {
		return 0, err
	}
	return pos, nil
}
