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
	// TestVariantDistances are distances for Input.TestVariants, in the same
	// order. A distance is a non-negative value, where 0.0 means
	// the changed files are extremely likely to affect the test variant,
	// and +inf means extremely unlikely.
	// Given a distance threshold, too-far away tests are skipped.
	//
	// When Algorithm() is called, TestVariantDistances is pre-initialized with a
	// slice with the same length as Input.TestVariants, and filled with zeros.
	// Thus by default, all tests are selected.
	TestVariantDistances []float64
}
