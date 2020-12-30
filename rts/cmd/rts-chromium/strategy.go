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

package main

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/presubmit/eval"
)

// selectTests calls skipFile for test files that should be skipped.
func (r *selectRun) selectTests(skipFile func(name string) error) (err error) {
	// Check if any of the changed files requires all tests.
	for f := range r.changedFiles {
		if requiresAllTests(f) {
			return nil
		}
	}
	r.strategy.Select(r.changedFiles.ToSlice(), func(fileName string) (keepGoing bool) {
		if !r.testFiles.Has(fileName) {
			return true
		}
		err = skipFile(fileName)
		return err == nil
	})
	return
}

func (r *createModelRun) selectTests(ctx context.Context, in eval.Input, out *eval.Output) error {
	for _, f := range in.ChangedFiles {
		switch {
		case f.Repo != "https://chromium-review.googlesource.com/chromium/src":
			return errors.Reason("unexpected repo %q", f.Repo).Err()
		case requiresAllTests(f.Path):
			return nil
		}
	}

	return r.fg.EvalStrategy(ctx, in, out)
}

// requiresAllTests returns true if changedFile requires running all tests.
// If a CL changes such a file, RTS gets disabled.
func requiresAllTests(changedFile string) bool {
	switch {
	case strings.HasPrefix(changedFile, "//testing/"):
		// This CL changes the way tests run or their configurations.
		// Run all tests.
		return true

	case changedFile == "//DEPS":
		// The full list of modified files is not available, and the
		// graph does not include DEPSed file changes anyway.
		return true

	default:
		return false
	}
}
