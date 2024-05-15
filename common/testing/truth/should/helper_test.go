// Copyright 2024 The LUCI Authors.
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

package should

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/failure"
)

// shouldPass returns a function you can pass to `t.Run` which tests that `f`
// 'passed' (i.e., is nil).
//
// Otherwise it has a fatal error and prints the Failure.
func shouldPass(f *failure.Summary) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		t.Helper()

		if f == nil {
			return
		}
		t.Fatal("unexpected failure", f)
	}
}

// shouldFail returns a function you can pass to `t.Run` which tests that `f`
// 'failed' (i.e., is NOT nil).
//
// It also takes zero or more strings to look for in the contents of the
// textpb-serialized Failure object. All given strings must be present in the
// Failure object.
//
// Otherwise it has a fatal error.
func shouldFail(f *failure.Summary, containing ...string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		t.Helper()

		if f == nil {
			t.Fatal("unexpected pass (nil)")
		}

		fail := f.String()
		failing := false
		for _, needle := range containing {
			if !strings.Contains(fail, needle) {
				if !failing {
					t.Logf("got failure: %q", fail)
					failing = true
				}
				t.Logf("missing text %q", needle)
			}
		}
		if failing {
			t.FailNow()
		}
	}
}
