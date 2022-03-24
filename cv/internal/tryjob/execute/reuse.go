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
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	recipe "go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// staleTryjobAge is the age after which a Tryjob is too old to reuse.
const staleTryjobAge time.Duration = 24 * time.Hour

// canReuseTryjob checks if a given Tryjob can be reused.
//
// There are 3 reasons why a tryjob may not be reuseable:
//  - The tryjob is too old to be reused.
//  - Reuse is disabled for the builder in the config.
//  - Reuse is disabled for this tryjob for a given mode.
func canReuseTryjob(ctx context.Context, tj *tryjob.Tryjob, def *tryjob.Definition, mode run.Mode) (bool, error) {
	switch result := tj.Result; {
	case result == nil:
		return false, errors.Reason("canReuseTryjob given a Tryjob with nil Result: %v", tj).Err()
	case def.DisableReuse:
		// The tryjob cannot be reused if the builder has the DisableReuse set.
		return false, nil
	case clock.Now(ctx).Sub(result.CreateTime.AsTime()) >= staleTryjobAge:
		// The tryjob cannot be reused if it is not fresh.
		return false, nil
	case !canReuseForMode(result.GetOutput(), mode):
		// The tryjob cannot be reused if the output from the recipe
		// opts this particular job out of reuse for the given Run mode.
		return false, nil
	default:
		return true, nil
	}
}

// canReuseForMode checks whether reuse is allowed for a given mode,
// given a list of the recipe output.
//
// If there are no modes in the mode allowlist, then reuse is allowed
// for all modes; if there's a non-empty allowlist, then reuse is only
// allowed for this modes in the allowlist.
func canReuseForMode(output *recipe.Output, mode run.Mode) bool {
	reusability := output.GetReusability()
	if reusability == nil || len(reusability.ModeAllowlist) == 0 {
		// If the mode allowlist is unspecified or empty, then reuse is allowed.
		return true
	}
	for _, allowed := range reusability.ModeAllowlist {
		if string(mode) == allowed {
			return true
		}
	}
	// No modes matched, so reuse is disallowed for the target mode.
	return false
}

// computeReuseKey computes the reuse key for a tryjob based on the CLs it
// involves.
func computeReuseKey(cls []*run.RunCL) string {
	clPatchsets := make([][]byte, len(cls))
	for i, cl := range cls {
		clPatchsets[i] = []byte(fmt.Sprintf("%d/%d", cl.ID, cl.Detail.GetMinEquivalentPatchset()))
	}
	sort.Slice(clPatchsets, func(i, j int) bool {
		return bytes.Compare(clPatchsets[i], clPatchsets[j]) < 0
	})
	h := sha256.New()
	h.Write(bytes.Join(clPatchsets, []byte{0}))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
