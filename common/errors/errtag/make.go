// Copyright 2025 The LUCI Authors.
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

package errtag

// Make creates a new [Tag] with a default value.
//
// This generates a new TagKey (which is a very cheap operation).
//
// See [Tag.WithDefault] to change the default value.
//
// # Expected Use - Global Tags
//
// The main use for errtag is to define package-level Tag[T] instances so that
// packages can tag their own errors, or for multiple packages to coordinate
// between them for common tags, such as
// [go.chromium.org/luci/common/retry/transient.Tag].
//
// These can then either be exported (for multiple packages to write and/or read
// the tag values out of errors), or not (for just the package's own use passing
// data around).
//
// This pattern is far and away the most common use of error tags.
//
// # Expected Use - Local Tags
//
// Occasionally you need to tag an error within a very narrow context, for
// example if you have an outer function calling something and passing
// a callback. In this case, the 'something' may be doing `errors.Is` checks
// where the callback must return errors derived (or joined) with some specific
// errors. It would be OK to make a tag within the outer function, and use this
// in the callback to pass back some data which is only directly* observable by
// the outer function.
//
// Each outer function call would generate a new, unique, Tag[T].
//
// * However, note that if the outer function returns this error, the tag value
// could be observed indirectly via [Collect].
//
// # Merging values
//
// In the event of a 'multi error' (e.g. errors.Join, fmt.Errorf with multiple %w
// verbs, the LUCI errors.MultiError, etc.), when you read the tag value from an
// error, it may need to 'merge' multiple values for this tag.
//
// The Tag returned by Make will merge multiple values by simply picking the
// first value (so - traverse the error breadth-first, picking the 'leftmost'
// value at the highest level encountered).
//
// If you need to override this, use MakeWithMerge.
func Make[T comparable](description string, defaultValue T) Tag[T] {
	return makeImpl(description, &defaultValue, func(values []*T) *T {
		return values[0]
	})
}

// MakeWithMerge creates a new [Tag].
//
// This acts the same as [Make], but also allows you to set a merge function.
func MakeWithMerge[T comparable](description string, defaultValue T, merge MergeFn[T]) Tag[T] {
	return makeImpl(description, &defaultValue, merge)
}

func makeImpl[T comparable](description string, defaultValue *T, merge MergeFn[T]) Tag[T] {
	key := TagKey{&description}
	return Tag[T]{key, defaultValue, merge}
}

// WithDefault returns a variant of this Tag with a different default (for
// [Tag.Apply], etc.).
//
// The returned tag will have the SAME key as `t`. That is, you can probe the
// error with [Tag.Value] with either tag to see the current value. The only
// difference is that if the tag is entirely missing from the error, `t` will
// return the old default and the new tag will return the new default.
//
// This is useful for cases where you want to have distinct Go symbols for
// different well know values of the tag (for example, if the tag has an enum
// type, you may want one tag variant per enum value so that they can be simply
// applied via [Tag.Apply]).
func (t Tag[T]) WithDefault(newDefault T) Tag[T] {
	t.check()
	return Tag[T]{t.key, &newDefault, t.merge}
}
