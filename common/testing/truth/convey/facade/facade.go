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

// Package facade is a transitional package that mimics the API of GoConvey
// classic. If you use it, you get a familiar syntax but no parallelism or
// compile-time errors from less permissive type signatures.
//
// New code shouldn't use it.
//
// Arguably, old code shouldn't use it either.
//
// This thing is a very thin wrapper on top of ftt and truth/{assert,comparison,should}.
// There is no immediate harm mixing and matching this library with the
// libraries that it wraps in the same leaf code, but if you did that you might eventually
// need to migrate.
package facade

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"
)

// Convey is a replacement for legacy Convey. Subconveys need a T argument, however.
func Convey(name string, t testing.TB, cb func(*ftt.Test)) {
	t.Helper()
	switch v := t.(type) {
	case *testing.T:
		ftt.Run(name, v, cb)
	case *ftt.Test:
		v.Run(name, cb)
	default:
		panic(fmt.Sprintf("bad type: %T", v))
	}
}

// T is a convenient alias for ftt.Test
type T = ftt.Test

// So is the equivalent of GoConvey's So. Note that it always takes three arguments.
func So[T any](t testing.TB, actual any, compare comparison.Func[T]) {
	t.Helper()
	assert.Loosely(t, actual, compare)
}

var ShouldAlmostEqual = should.AlmostEqual[float64]
var ShouldBeNil = should.BeNil
var ShouldNotBeNil = should.NotBeNil
var ShouldBeTrue = should.BeTrue
var ShouldBeFalse = should.BeFalse
var ShouldEqual = should.Equal[any]
var ShouldBeEmpty = should.BeEmpty
var ShouldResemble = should.Match[any]
var ShouldContainKey = should.ContainKey[any]
var ShouldHaveLength = should.HaveLength
