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

import (
	"fmt"
)

// TagKey is a process-unique key associated with a family of Tag[T]'s derived
// from the same [Make]/[MakeWithMerge].
//
// These are `comparable`, so e.g.
//
//	SomeTag.Key() == SomeTag.WithDefault(...).Key()
//
// Will always compile and be `true`.
type TagKey struct {
	unique *string
}

// Valid returns `true` if this TagKey was obtained via [Make]/[MakeWithMerge].
func (k TagKey) Valid() bool {
	return k.unique != nil
}

// String returns the original description associated with this TagKey.
func (k TagKey) String() string {
	if k.unique == nil {
		return "TagKey(<invalid>)"
	}
	return *k.unique
}

// Tag holds everything necessary to associate values of type T with errors.
//
// Construct this with [Make].
//
// As a compatibility feature, a Tag can currently be used with:
//   - errors.Annotate(err, ...).Tag(<the tag>)
//   - errors.New(err, <the tag>))
//
// However, these usages are NOT recommended, and exactly equivalent to just
// doing `tag.Apply(err)`. At some point in the medium-term future, the
// errors.Annotate construct will be removed entirely and replaced with
// `fmt.Errorf("... %w ...", ...)`.
//
// # Tag keys
//
// Every Tag[T] has a unique description, which is enforced by [Make].
//
// # Default Value
//
// Tags have a default value, which is the zero value of T, but can be changed
// with [Tag.WithDefault].
//
// The default value is used for [Tag.Apply], [Tag.In], [Tag.Value] and
// [Tag.ValueOrDefault].
//
// # Merging
//
// When extracting a Value from an error, Tags do a process called
// 'merging'. Merging happens when an error in the stack
// implements `Unwrap() []error`, and multiple of those wrapped errors contain
// a value for Tag[T]. To provide a cohesive understanding of what the current
// singular value is for such an error, there needs to be some defined way to
// merge or pick one value.
//
// By default Tags include a Merge function which simply picks the top-left-most
// value encountered with a breadth-first search, which has historically been
// good-enough. However some tag value types like status codes may have
// a concept of a 'worst' value or such. The Merge function can be set with
// [MakeWithMerge].
type Tag[T comparable] struct {
	key TagKey

	// defaultValue is kept as a pointer to make copying `Tag[T]` in user code
	// cheap. All external facing methods of Tag, however, deal directly with T,
	// not *T.
	defaultValue *T

	merge MergeFn[T]
}

// Key returns the unique TagKey that corresponds with this tag.
func (t Tag[T]) Key() TagKey {
	t.check()
	return t.key
}

// Is checks to see if this Tag[T] is the same key as `other`.
//
// This does not compare default values of `t` and `other`.
func (t Tag[T]) Is(other Tag[T]) bool {
	return t.Key() == other.Key()
}

// check should be called from every method of TagType
func (t Tag[T]) check() {
	if !t.key.Valid() {
		panic(fmt.Sprintf("%T has been default-constructed. Use errtag.Make instead.", t))
	}
}

// MergeFn is the type signature of the Merging function that
// [Tag] uses.
//
// This function will only ever be called with len(values) > 1, and every item
// in values is non-nil.
//
// It may return one of the items in `values`, or synthesize an entirely new
// value.
//
// If this returns `nil`, it indicates "no value" (e.g. if your merge
// function allows sibling errors to 'cancel out').
//
// This function MUST NOT modify the contents of the *T in `values`. Pointers
// are used here simply as a way to avoid copying many potentially large
// T objects. If Go provided a way to make these immutable non-pointers which
// weren't copied, I would do it, but unfortunately the best I can do is ask via
// this doc for your pinkie-promise :/.
//
// This function MUST be left associative, i.e.
//
//	Merge(A, B, C, D)
//
// Must be the same as:
//
//	Merge(A, Merge(B, Merge(C, D)))
type MergeFn[T comparable] func(values []*T) *T

// TagType is implemented by all Tag[T] from this package.
//
// This allows [Collect] operate across all Tag[??].
type TagType interface {
	Key() TagKey
}

func (t Tag[T]) findExisting(err error) *T {
	if err == nil {
		return nil
	}

	switch x := err.(type) {
	case *wrappedErr[T]:
		if t.key == x.key {
			return &x.value
		}
		return t.findExisting(x.inner)

	case interface{ Unwrap() []error }:
		errs := x.Unwrap()
		if len(errs) == 0 {
			return nil
		}
		vals := make([]*T, 0, len(errs))
		for _, e := range errs {
			val := t.findExisting(e)
			if val != nil {
				vals = append(vals, val)
			}
		}
		if len(vals) == 0 {
			return nil
		}
		if len(vals) == 1 {
			return vals[0]
		}
		return t.merge(vals)

	case interface{ Unwrap() error }:
		return t.findExisting(x.Unwrap())
	}

	return nil
}

// Value retrieves the current value associated with this Tag in the error,
// also indicating if the tag was found at all.
//
// If the tag was not found, returns (<defaultValue>, false).
func (t Tag[T]) Value(err error) (value T, found bool) {
	t.check()
	existing := t.findExisting(err)
	if existing != nil {
		value = *existing
		found = true
	} else {
		value = *t.defaultValue
	}
	return
}

// ValueOrDefault is the same as [Tag.Value], but ignoring `found`.
func (t Tag[T]) ValueOrDefault(err error) T {
	ret, _ := t.Value(err)
	return ret
}

// ApplyValue wraps `err` with an error containing the given value.
//
// If `err` is nil, this returns nil.
func (t Tag[T]) ApplyValue(err error, value T) (wrapped error) {
	t.check()
	if err == nil {
		return err
	}
	return &wrappedErr[T]{key: t.key, inner: err, value: value, merge: t.merge}
}

// Apply is shorthand for ApplyValue(err, <defaultValue>).
func (t Tag[T]) Apply(err error) (wrapped error) {
	// NOTE: we don't actually do t.ApplyValue(err, *t.defaultValue) here because
	// we want to do t.check() before we access t.defaultValue. At that point we
	// may as well copy the implementation.
	t.check()
	if err == nil {
		return err
	}
	return &wrappedErr[T]{key: t.key, inner: err, value: *t.defaultValue, merge: t.merge}
}

var _ interface {
	// These are both for compatibility with the errors.Annotate(...).Tag(X) functionality.
	GenerateErrorTagValue() (key, value any)
	Apply(err error) (wrapped error)
} = Tag[bool]{} // pick bool as arbitrary T for assertion

// GenerateErrorTagValue allows this tag to be compatible with
// errors.Annotate.Tag.
//
// This is a no-op and just allows this to wrapper to be used in the
// errors.Annotate(...).Tag the function type signature. This should be removed
// when the Annotate pattern is removed from the errors package.
//
// Deprecated: Do not use.
func (t Tag[T]) GenerateErrorTagValue() (key, value any) {
	t.check()
	return nil, nil
}

// In returns true iff the Tag is present in the error AND currently has the
// defaultValue for the tag.
//
// If you want to check the presence of a tag, regardless of value, use
// [Tag.Value].
//
// NOTE: This is almost only ever useful for Tags of type `bool`. However, it is
// so useful for those that it's worth keeping, even though other Tag types
// can't make good use of it.
//
// It is an alias for:
//
//	val, found := t.Value(err)
//	return found && val == <defaultValue>
func (t Tag[T]) In(err error) (existsWithDefault bool) {
	val, found := t.Value(err)
	return found && val == *t.defaultValue
}
