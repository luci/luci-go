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
	"time"

	"golang.org/x/exp/constraints"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

// ShortDuration is the maximum difference allowed by similar times.
const ShortDuration = 3 * time.Millisecond

// MaxTestLength is a sensible maximum number of characters for the name of a test.
//
// If the combined length of all tests exceeds 512 bytes, this starts causing problems with
// downstream services as of 2024-07-22. However, with a budget of 100 characters you get
// five levels, which should be enough in practice.
//
// See b:354772098 for more information.
const MaxTestLength = 100

// Convey is a replacement for legacy Convey. Subconveys need a T argument, however.
//
// If the test name exceeds 100 characters, we will "helpfully" panic.
// See b:354772098 for details.
func Convey(name string, t testing.TB, cb func(*ftt.Test)) {
	if n := len(name); n > MaxTestLength {
		panic(fmt.Sprintf("test %q has length %d which exceeds %d", name, n, MaxTestLength))
	}
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

// ShouldBeLessThan is a generic function that wraps should.BeLessThan.
//
// This avoids needless duplication for ints, int64s, strings, and other
// stuff.
//
// Most of the comparisons below could also be made generic, but I'm choosing
// to genericize things on an as-needed basis to keep the code in this library
// as simple as possible.
func ShouldBeLessThan[T constraints.Ordered](upper T) comparison.Func[T] {
	return should.BeLessThan(upper)
}

// ShouldAlmostEqualTime compares two times and checks that they are similar.
//
// This is a compatibility function that exists only to allow bad tests
// that don't mock the clock to pass and not block rewrite efforts.
//
// Code that uses the clock should touch it through go.chromium.org/luci/common/clock ,
// and go.chromium.org/luci/common/clock/testclock .
//
// The time would then be set in a test like:
//
//	func TestWhatever(t *testing.T) {
//		t.Parallel()
//		ctx := context.Background()
//		tc, ctx := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
//		// do stuff
//	}
//
// Do not use this function for any other purpose.
func ShouldAlmostEqualTime(expected time.Time) comparison.Func[time.Time] {
	return func(actual time.Time) *failure.Summary {
		d := actual.Sub(expected)
		return ShouldBeLessThan(ShortDuration)(d)
	}
}

// DangerousShouldResemble uses the legacy behavior, which reaches inside
// protos (for example, it is not proto-aware) and ignores unexported fields.
//
// This function should only be used to port an ornery test to the glorious
// new FTT world.
var DangerousShouldResemble = should.Resemble[any]

var (
	ShouldAlmostEqual      = should.AlmostEqual[float64]
	ShouldBeEmpty          = should.BeEmpty
	ShouldBeFalse          = should.BeFalse
	ShouldBeNil            = should.BeNil
	ShouldBeTrue           = should.BeTrue
	ShouldContain          = should.Contain[any]
	ShouldContainString    = should.Contain[string]
	ShouldContainKey       = should.ContainKey[any]
	ShouldNotContainKey    = should.NotContainKey[any]
	ShouldEqual            = should.Equal[any]
	ShouldEqualInt64       = should.Equal[int64]
	ShouldHaveLength       = should.HaveLength
	ShouldNotBeNil         = should.NotBeNil
	ShouldResemble         = should.Match[any]
	ShouldContainSubstring = should.ContainSubstring
	ShouldBeBlank          = should.BeBlank
)
