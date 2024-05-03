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

// package comparison contains symbols for making your own [Comparison]
// implementations for use with [go.chromium.org/luci/common/testing/assert].
//
// Please see [go.chromium.org/luci/common/testing/assert/should] for a set of
// ready-made Comparisons.
//
// # Implementing Comparisons
//
// Comparison implementations will be used in the context of an
// [go.chromium.org/luci/common/testing/assert.Assert] or a
// [go.chromium.org/luci/common/testing/assert.Check] call like:
//
//	Assert(t, actualValue, comparisonName(comparisonArguments))
//	if Check(t, actualValue, comparisonName(comparisonArguments)) {
//	   // code if comparison passed
//	}
//
// With this in mind, try to pick comparisonName so that it reads well in this
// context. The default set of comparison implementations have the form
// "should.XXX", e.g. "should.Equal" or "should.NotBeNil" which work well in
// this context. If you are implementing comparisons in your own tests, you can
// achieve a similar result by dropping the ".", e.g. "shouldBeComplete", but
// you can also use other natural syntax like "isConsistentWith(databaseKey)",
// etc.
//
// Typically, Comparisons require additional data to produce a [*OldResult] for
// a given value. Implementations which take additional data typically look
// like:
//
//	func isConsistentWith(databaseKey *concreteType) results.Comparison[expectedType] {
//	  return func(actualValue expectedType) *results.Result {
//	     // compare databaseKey with actualValue here
//	  }
//	}
//
// Functions returning Comparisons may be generic, if necessary, but frequently
// when writing them in package-specific contexts, generic type arguments are
// not needed. In general, if you can make your function accept a narrow,
// specific, type, it will be better. If you feel like you really need to make a
// generic Comparison or function returning a Comparison, please discuss
// directly contributing it to [go.chromium.org/luci/common/testing/assert/should].
//
// When the assert library uses a Comparison, it will attempt to losslessly
// convert the actual value to the T type of the Comparison. The high-level
// takeaway is that it implements simple conversions which preserve the actual
// data, but may adjust the type. These conversion rules ONLY look at the types,
// not the value, so e.g. 'int64(1) -> float32 will not work, even though
// float32(1) is storable exactly.
//
// See [go.chromium.org/luci/common/data/convert.LosslesslyTo] for how this conversion
// works in more detail.
//
// With that in mind, if you don't want any of this conversion to take place
// (e.g. customStringType -> string, int8 -> int64, etc.), you can use "any" for
// T in the Comparison.
//
// # Crafting *Results
//
// After your [Comparison] makes its evaluation of the actual data, if it
// fails, it needs to return a [*OldResult].
//
// The cardinal rule to follow is that [*OldResult] should contain
// exactly the necessary+sufficient information to let a reader understand why
// the Comparison failed. Try to avoid populating Values with redundant information
// just because it's easily available.
//
// For example, it is not necessary to have all of the following in a comparison named
// 'shouldContain':
//
// 1) a 'Because' line explaining that "a" is not in "b".
// 2) the actual value of "a"
// 3) the expected value of "b"
// 4) a Diff demonstrating that "a" is not in "b".
//
// (4) alone would be enough.
//
// A *Result has a couple different pieces:
//
//   - The name of the Comparison (or the function which generated the
//     Comparison)
//   - (optional) A series of "Values".
//   - (optional) A diff of two pieces of data.
//
// The name of the *Result is always required, and will be rendered like:
//
//	resultName FAILED
//
// For some Comparisons (like should.BeTrue or should.NotBeEmpty, etc.) this
// is enough context for the user to figure out what happened.
//
// However, most Comparisons will need to give additional context to the
// failure, which is where values and diff come in.
//
// Values are named data items, and can be added with [*OldResult.Value] or
// [*OldResult.Valuef]. These will be rendered under the name like:
//
//	resultName FAILED
//	  ValueName: value contents
//
// If you use Value() to add the data item, it will be rendered with
// [fmt.Sprintf]("%#v"). If you use Valuef(), this allows you to directly set
// the text which will be displayed. Values which have newlines in their
// representation will be split across lines like:
//
//	resultName FAILED
//	  ValueName:
//	  | This is a very long
//	  | value with
//	  | linebreaks.
//
// By convention, this package defines a couple different common labels w/
// helper functions which are useful in many, but not all, Comparisons.
//
//   - [*Result.Because]: This formats a string with fmt.Sprintf and sets it to
//     have the "Because" name. This should be a descriptive explaination of why
//     the Comparison failed.
//   - [*OldResult.Actual]/[*Result.Actualf]: These can be used to reflect the
//     "actual" value of the assertion back to the reader of the assertion
//     failure. Sometimes this is useful (e.g. should.AlmostEqual reflects the
//     actual value back, which is a float32). However, sometimes this is not
//     useful, e.g. when the actual value is a gigantic struct (see
//     [*OldResult.Diff]).
//   - [*OldResult.Expected]/[*Result.Expected]: These can be used to reflect the
//     "expected" value of the comparison back to the reader of the assertion
//     failure. This has very similar tradeoffs to Actual/Actualf.
//
// Finally, *Result can have a Diff, which is what you will likely want to use
// if the actual and expected values have large representations (long lists,
// structs, potentially large maps, etc.). This is usually set with [*OldResult.Diff],
// and will use [github.com/google/go-cmp] to generate the difference between
// two arbitrary objects. This function accepts zero or more
// [github.com/google/go-cmp/cmp.Option] objects, which can allow you to compare
// more advanced structures, or structures which contain important unexported
// fields, etc.
//
// Diff will also generate a simpler 1-line diff for simple object kinds
// (numbers, strings, etc.) if both objects have matching types and have
// small representations (e.g. for strings, it would only use this form if the
// length of both strings sum to less than 60 characters or so).
//
// By default, the diff will print a hint like "(-actual, +expected)" or "(actual
// != expected)" to help orient the reader about how to interpret the diff. This
// hint works well, but occasionally a Comparison may need to set different
// labels for these. To do this, refer to the [*OldResult.DiffHintNames] method.
package comparison
