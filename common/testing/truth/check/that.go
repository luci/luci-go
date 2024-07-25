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

// Package check contains two alternate spellings of truth.Check and
// truth.CheckLoosely, spelled `check.That` and `check.Loosely`
// respectively.
//
// This allows test writers to have fluent expressions in tests such allows
//
//	check.That(t, 10, should.Equal(20))
//	check.Loosely(t, myCustomInt(10), should.Equal(20))
//
// Rather than
//
//	truth.Check(t, 10, should.Equal(20))
//	truth.CheckLoosely(t, myCustomInt(10), should.Equal(20))
//
// This package has a counterpart sibling `assert` which cover `Check` and
// `CheckLoosely`.
package check

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/comparison"
)

// That is an alternate name for truth.Check.
//
// Example: `check.That(t, 10, should.Equal(20))`
func That[T any](t testing.TB, actual T, compare comparison.Func[T], opts ...truth.Option) (ok bool) {
	t.Helper()
	return truth.Check(t, actual, compare, opts...)
}

// Loosely is an alternate name for truth.CheckLoosely.
//
// Example: `check.Loosely(t, myCustomInt(10), should.Equal(20))`
func Loosely[T any](t testing.TB, actual any, compare comparison.Func[T], opts ...truth.Option) (ok bool) {
	t.Helper()
	return truth.CheckLoosely(t, actual, compare)
}
