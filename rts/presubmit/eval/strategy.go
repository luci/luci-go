// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"

	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Strategy evaluates how much a given test is affected by given changed files.
type Strategy func(context.Context, Input, *Output) error

// Input is input to a selection strategy.
type Input struct {
	// ChangedFiles is a list of changed files.
	ChangedFiles []*evalpb.SourceFile

	// The strategy needs to decide how much each of these test variants is
	// affected by the changed files.
	TestVariants []*evalpb.TestVariant
}

// ensureChangedFilesInclude ensures that in.ChangedFiles includes all changed
// files in all of the patchsets.
func (in *Input) ensureChangedFilesInclude(pss ...*evalpb.GerritPatchset) {
	type key struct {
		repo, path string
	}
	set := map[key]struct{}{}
	for _, f := range in.ChangedFiles {
		set[key{repo: f.Repo, path: f.Path}] = struct{}{}
	}
	for _, ps := range pss {
		for _, f := range ps.ChangedFiles {
			k := key{repo: f.Repo, path: f.Path}
			if _, ok := set[k]; !ok {
				set[k] = struct{}{}
				in.ChangedFiles = append(in.ChangedFiles, f)
			}
		}
	}
}

// Output is the output of a selection strategy.
type Output struct {
	// TestVariantAffectedness is how much Input.TestVariants are affected by the
	// code change, where TestVariantAffectedness[i]
	// corresponds to Input.TestVariants[i].
	//
	// When Strategy() is called, TestVariantAffectedness is pre-initialized
	// with a slice with the same length as Input.TestVariants, and zero elements.
	// Thus by default all tests are affected (distance=0) and unranked (rank=0).
	TestVariantAffectedness AffectednessSlice
}

// Affectedness is how much a test is affected by the code change.
// The zero value means a test is affected and unranked.
type Affectedness struct {
	// Distance is a non-negative number, where 0.0 means the code change is
	// extremely likely to affect the test, and +inf means extremely unlikely.
	// If a test's distance is less or equal than a given MaxDistance threshold,
	// then the test is selected.
	// A selection strategy doesn't have to use +inf as the upper boundary if the
	// threshold uses the same scale.
	Distance float64

	// Rank orders all tests in the repository by Distance, with rank 1 assigned
	// to the test with the lowest distance.
	// Iff two tests have the same distance, then they may, but don't have to,
	// have same the same rank.
	// Zero rank means the rank is unknown, e.g. if the selection strategy is not
	// aware of other tests.
	//
	// Example:
	// There are three tests exists in the codebase: T1, T2 and T3.
	// Their distances are 1, 2 and 3, respectively.
	// Input.TestVariants has only one element and it refers to T2.
	// Then Output.TestVariantAffectedness[0].Rank should be 2.
	Rank int
}

// AffectednessSlice is a slice of Affectedness structs.
type AffectednessSlice []Affectedness

// closest returns index of the closest Affectedness, in terms of distance and
// rank.
// If two tests have the same distance and ranks are available, then ranks are
// used to break the tie.
//
// Returns an error if an inconsistency is found among ranks and distances,
// see also checkConsistency.
func (s AffectednessSlice) closest() (int, error) {
	if len(s) == 0 {
		return 0, errors.New("empty")
	}
	closest := 0
	for i, af := range s {
		closestAf := s[closest]
		if err := checkConsistency(closestAf, af); err != nil {
			return 0, err
		}
		haveRanks := af.Rank != 0 && closestAf.Rank != 0
		if closestAf.Distance > af.Distance || haveRanks && closestAf.Rank > af.Rank {
			closest = i
		}
	}
	return closest, nil
}

// checkConsistency returns a non-nil error if the order implied by ranks
// and distances is inconsistent in a and b, e.g. a is closer, but its rank
// is greater.
func checkConsistency(a, b Affectedness) error {
	if a.Rank == 0 || b.Rank == 0 {
		return nil
	}
	if (a.Distance < b.Distance && a.Rank >= b.Rank) || (a.Distance > b.Distance && a.Rank <= b.Rank) {
		return errors.Reason("ranks and distances are inconsistent: %#v and %#v", a, b).Err()
	}
	return nil
}
