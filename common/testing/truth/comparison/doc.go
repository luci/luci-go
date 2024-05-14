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

// Package comparison contains symbols for making your own comparison.Func
// implementations for use with [go.chromium.org/luci/common/testing/truth].
//
// Please see [go.chromium.org/luci/common/testing/truth/should] for a set of
// ready-made comparison functions.
//
// # Implementing comparisons
//
// Comparison implementations will be used in the context of
// [go.chromium.org/luci/common/testing/truth/assert] and
// [go.chromium.org/luci/common/testing/truth/check] like:
//
//	assert.That(t, actualValue, comparisonName(comparisonArguments))
//
//	if check.That(t, actualValue, comparisonName(comparisonArguments)) {
//	   // code if comparison passed
//	}
//
// With this in mind, try to pick comparisonName so that it reads well in this
// context. The default set of comparison implementations have the form
// "should.XXX", e.g. "should.Equal" or "should.NotBeNil" which work well here.
// If you are implementing comparisons in your own tests, you can make
// a similar Failure by dropping the ".", e.g. "shouldBeComplete", but you can
// also use other natural syntax like "isConsistentWith(databaseKey)", etc.
//
// Typically, comparisons require additional data to produce a [*Failure] for
// a given value. Implementations which take additional data typically look
// like:
//
//	func isConsistentWith(databaseKey *concreteType) comparison.Func[expectedType] {
//	  return func(actualValue expectedType) *comparison.Failure {
//	     // compare databaseKey with actualValue here
//	  }
//	}
//
// Functions returning comparison.Func may be generic, if necessary, but when
// writing them in package-specific contexts, generic type arguments are usually
// not needed. In general, if you can make your function accept a narrow,
// specific, type, it will be better. If you feel like you really need to make
// a generic comparison or function returning a comparison, please discuss
// directly contributing it to
// [go.chromium.org/luci/common/testing/truth/should].
//
// When the truth library uses a comparison with a 'loosely' variant (e.g.
// truth.AssertLoosely, or assert.Loosely), it will attempt to losslessly
// convert the actual value to the T type of the comparison.Func. The high-level
// takeaway is that it implements simple conversions which preserve the actual
// data, but may adjust the type. These conversion rules ONLY look at the types,
// not the value, so e.g. 'int64(1) -> float32' will not work, even though
// float32(1) is storable exactly, because in general, not all int64 fit into
// float32.
//
// See [go.chromium.org/luci/common/data/convert.LosslesslyTo] for how this
// conversion works in more detail.
//
// With that in mind, if you don't want any of this conversion to take place
// (e.g. customStringType -> string, int8 -> int64, etc.), you can use "any" for
// T in the comparison (in which case your comparison will need to do work to
// pull supported types out itself via type switches or reflect) OR you can use
// the `truth.Assert`, `truth.Check`, `assert.That`, or `check.That` functions
// to ensure that the actual value exactly matches the comparison.Func[T] type
// at compile-time.
//
// # Crafting *Failure
//
// After your [Func] makes its evaluation of the actual data, if it
// fails, it needs to return a [*Failure], which is a proto message.
//
// As a proto, you can construct [*Failure] however you like, but most
// comparison implementations will use [NewFailureBuilder] to construct
// a [*Failure] using a 'fluent' interface.
//
// The cardinal rule to follow is that [*Failure] should contain exactly the
// information which allows the reader understand why the comparison failed. If
// you have Findings which are possibly redundant, you can mark them with the
// 'Warn' level to hide them from the test output until the user passes `go test
// -v`.
//
// For example, in a comparison named 'shouldEqual', including all of the
// following would be redundant, even though they are all individually helpful:
//
//	Check shouldEqual[string] FAILED
//	  Because: "actual" does not equal "hello"
//	  Actual: "actual"
//	  Expected: "hello"
//	  Diff: \
//	        string(
//	      -  "actual",
//	      +  "hello",
//	        )
//
// (4) alone would be enough, but if necessary, you could have (2) and (3)
// marked with a warning-level level, in case the diff can be hard to interpret.
//
// A *Failure has a couple different pieces:
//
//   - The name of the comparison (or the function which generated the
//     comparison), its type arguments (if any), and any particularly
//     helpful expected arguments (e.g. the expected length of
//     should.HaveLength).
//   - Zero or more "Findings".
//
// The name of the *Failure is always required, and will be rendered like:
//
//	resultName FAILED
//
// For some comparisons (like should.BeTrue or should.NotBeEmpty, etc.) this
// is enough context for the user to figure out what happened.
//
// However, most comparisons will need to give additional context to the
// failure, which is where findings come in.
//
// Findings are named data items in sequence. These will be rendered under the
// name like:
//
//	resultName FAILED
//	  ValueName: value contents
//
// Finding values are a list of lines - in the event that the value contains
// multiple lines, you'll see it rendered like:
//
//	resultName FAILED
//	  ValueName: \
//	    This is a very long
//	    value with
//	    multiple lines.
//
// By convention, this package defines a couple different common Finding labels
// w/ helper functions which are useful in many, but not all, comparisons.
//
//   - "Because" - This Finding should have a descriptive explanation of why
//     the comparison failed.
//   - "Actual"/"Expected" - These reflect the actual or expected value of the
//     assertion back to the reader of the assertion failure. Sometimes this is
//     useful (e.g. should.AlmostEqual[float32] reflects the actual value back).
//     However, sometimes this is not useful, e.g. when the value is a gigantic
//     struct.
//   - "Diff" - This is the difference between Actual and Expected, usually
//     computed with cmp.Diff (or, alternately, as a unified diff). These
//     Findings should be marked with a diff type hint, so that they can have
//     syntax coloring applied to them when rendered in a terminal.
//
// As the implementor of the comparison, it is up to you to decide what Findings
// are the best way to explain to the test failure reader what happened and,
// potentially why. Sometimes none of these finding labels will be the right way
// to communicate that, and that's OK :).
package comparison
