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

// Package assert contains two alternate spellings of truth.Assert and
// truth.AssertLoosely, spelled `assert.That` and `assert.Loosely`
// respectively.
//
// This allows test writers to more concisely express assertions like:
//
//	assert.That(t, 10, should.Equal(20))
//	assert.Loosely(t, myCustomInt(10), should.Equal(20))
//
// Rather than
//
//	truth.Assert(t, 10, should.Equal(20))
//	truth.AssertLoosely(t, myCustomInt(10), should.Equal(20))
//
// This package has a counterpart sibling `check` which covers `Check` and
// `CheckLoosely`.
package assert

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/comparison"
)

// That is an alternate name for truth.Assert.
//
// Example: `assert.That(t, 10, should.Equal(20))`
func That[T any](t testing.TB, actual T, compare comparison.Func[T]) {
	t.Helper()
	truth.Assert(t, actual, compare)
}

// Loosely is an alternate name for truth.AssertLoosely.
//
// Example: `assert.Loosely(t, myCustomInt(10), should.Equal(20))`
func Loosely[T any](t testing.TB, actual any, compare comparison.Func[T]) {
	t.Helper()
	truth.AssertLoosely(t, actual, compare)
}
