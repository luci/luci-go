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
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/changelist"
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

// canReuseTryjob checks if a given Tryjob can be reused.
//
// Tryjob *can* be reused iff the Tryjob ends successfully and is fresh enough
// (i.e created within `staleTryjobAge`). Tryjob *may* be reused if the Tryjob
// is fresh enough and still running or hasn't started yet.
func canReuseTryjob(ctx context.Context, tj *tryjob.Tryjob, mode run.Mode) reusability {
	switch status := tj.Status; {
	case status == tryjob.Status_STATUS_UNSPECIFIED:
		panic(fmt.Errorf("unspecified status for tryjob %d", tj.ID))
	case status == tryjob.Status_PENDING:
		return reuseMaybe
	case status == tryjob.Status_TRIGGERED && isTryjobStale(ctx, tj):
		return reuseDenied
	case status == tryjob.Status_TRIGGERED:
		return reuseMaybe
	case status == tryjob.Status_ENDED && canReuseResult(ctx, tj, mode):
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
func canReuseResult(ctx context.Context, tj *tryjob.Tryjob, mode run.Mode) bool {
	switch result := tj.Result; {
	case tj.Status != tryjob.Status_ENDED:
		panic(fmt.Errorf("canReuseResult must be called when tryjob status is ended. got %s", tj.Status))
	case result == nil:
		logging.Errorf(ctx, "tryjob %d has nil result but it has ended already", tj.ID)
		return false
	case isTryjobStale(ctx, tj):
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
