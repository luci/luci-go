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
	ChangedFiles []*SourceFile

	// The algorithm needs to decide whether to run these test variants.
	TestVariants []*evalpb.TestVariant
}

// SourceFile identifies a source file.
type SourceFile struct {
	// Repo is a repository identifier.
	// For googlesource.com repositories, it is a canonical URL, e.g.
	// https://chromium.googlesource.com/chromium/src
	Repo string

	// Path to the file relative to the repo root.
	// Starts with "//".
	Path string
}

// Output is the output of an RTS algorithm.
type Output struct {
	// TestVariantDistances are distances for each Input.TestVariants, in the
	// same order. A distance is a non-negative value, where 0.0 means
	// the changed files are extremely likely to affect the test variant,
	// and +inf means extremely unlikely.
	// Given a distance threshold, too-far away tests are skipped.
	//
	// The algorithm doesn't have to use [0, +inf] scale. The distance threshold
	// is computed automatically to satisfy ChangeRecall and TestRecall.
	//
	// When Algorithm() is called, TestVariantDistances is pre-initialized with a
	// slice with the same length as Input.TestVariants, and filled with zeros.
	// Thus by default, all tests are selected.
	TestVariantDistances []float64
}
