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
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/assert/comparison"
	"go.chromium.org/luci/common/testing/typed"
)

// Resemble returns a Comparison which checks if the actual value 'resembles'
// `expected`.
//
// Semblance is computed with "github.com/google/go-cmp/cmp", and this
// function accepts additional cmp.Options to allow for handling of different
// types/fields/filtering proto Message semantics, etc.
//
// For convenience, `opts` implicitly includes:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//
// This is done via the go.chromium.org/luci/common/testing/registry package,
// which also allows process-wide registration of additional default
// cmp.Options, should you need it.
//
// It is recommended that you use should.Equal when comparing primitive types.
func Resemble[T any](expected T, opts ...cmp.Option) comparison.Func[T] {
	cmpName := "should.Resemble"

	return func(actual T) *comparison.Failure {
		diff := typed.Diff(expected, actual)

		if diff == "" {
			return nil
		}

		return comparison.NewFailureBuilder(cmpName, expected).
			Actual(actual).WarnIfLong().
			Expected(expected).WarnIfLong().
			AddCmpDiff(diff).
			Failure
	}
}
