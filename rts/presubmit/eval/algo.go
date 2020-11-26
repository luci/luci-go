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

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Algorithm accepts a list of changed files and a test description and
// decides whether to run it.
type Algorithm func(context.Context, Input, *Output) error

// Input is input to an RTS Algorithm.
type Input struct {
	// ChangedFiles is a list of files changed in a patchset.
	ChangedFiles []*evalpb.SourceFile

	// The algorithm needs to decide whether to run these test variants.
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

// Output is the output of an RTS algorithm.
type Output struct {
	// TestVariantAffectedness is how much Input.TestVariants are affected by the
	// code change.
	//
	// When Algorithm() is called, TestVariantAffectedness is pre-initialized
	// with a slice with the same length as Input.TestVariants, and zero elements.
	// Thus by default, all tests are selected (distance=0) and unranked (rank=0).
	TestVariantAffectedness []Affectedness
}

// Affectedness is how much a test is affected by the code change.
type Affectedness struct {
	// Distance is a non-negative number, where 0.0 means the code change is
	// extremely likely to affect the test, and +inf means extremely unlikely.
	//
	// If a test's distance is less or equal than a given MaxDistance threshold,
	// then the test is selected.
	Distance float64

	// Rank is one-based index in a list of all tests, sorted by Distance.
	// Zero means rank is unknown, e.g. if the algorithm is not aware of other
	// tests.
	//
	// Example:
	// There are three tests exists in the codebase: T1, T2 and T3.
	// Their distances are 1, 2 and 3, respectively.
	// Input.TestVariants has only one element and it refers to T2.
	// Then Output.TestVariantAffectedness[0].Rank should be 2.
	Rank int
}
