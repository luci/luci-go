// Copyright 2017 The LUCI Authors.
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

package errors

import (
	"sort"
)

type (
	tagDescription struct {
		description string
	}

	// TagKey objects are used for applying tags and finding tags/values in
	// errors. See NewTag for details.
	//
	// Deprecated: Use errors.MakeBoolTag() instead.
	TagKey *tagDescription

	tagKeySlice []TagKey

	// TagValue represents a (tag, value) to be used with Annotate.Tag, or may be
	// applied to an error directly with the Apply method.
	//
	// Usually tag implementations will have a typesafe With method that generates
	// these. Avoid constructing these ad-hoc so that a given tag definition can
	// control the type safety around these.
	//
	// Deprecated: Use go.chromium.org/common/errors/errtag instead.
	TagValue struct {
		Key   TagKey
		Value any
	}

	// TagValueGenerator generates (TagKey, value) pairs, for use with Annoatator.Tag
	// and New().
	//
	// If the TagValueGenerator implements NormalGoWrapper, then
	// NormalGoWrapper.Apply will be used instead.
	//
	// Deprecated: Use go.chromium.org/common/errors/errtag instead.
	TagValueGenerator interface {
		// key must be a TagKey from this package. This strange interface is because
		// TagValueGenerator is being actively deprecated.
		//
		// NormalGoWrapper implementations must return (nil, nil).
		GenerateErrorTagValue() (key, value any)
	}

	// NormalGoWrapper is an interface that tags may implement, to just wrap
	// `err`, returning `wrapped`, rather than actually adding a tag to the
	// internal annotator stack guts.
	//
	// This will be used to migrate away from these annotator tags to just
	// standard go errors.
	//
	// Deprecated: Use go.chromium.org/common/errors/errtag instead.
	NormalGoWrapper interface {
		Apply(err error) (wrapped error)
	}
)

var _ sort.Interface = tagKeySlice(nil)

func (s tagKeySlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s tagKeySlice) Len() int           { return len(s) }
func (s tagKeySlice) Less(i, j int) bool { return s[i].description < s[j].description }

// TagValueIn will retrieve the tagged value from the error that's associated
// with this key, and a boolean indicating if the tag was present or not.
//
// Deprecated: Use go.chromium.org/common/errors/errtag instead.
func TagValueIn(t TagKey, err error) (value any, ok bool) {
	Walk(err, func(err error) bool {
		if sc, isSC := err.(stackContexter); isSC {
			if value, ok = sc.stackContext().tags[t]; ok {
				return false
			}
		}
		return true
	})
	return
}

// GenerateErrorTagValue implements TagValueGenerator
func (t TagValue) GenerateErrorTagValue() (key, value any) { return t.Key, t.Value }

// Apply applies this tag value (key+value) directly to the error. This is
// a shortcut for `errors.Annotate(err, "").Tag(t).Err()`.
func (t TagValue) Apply(err error) error {
	if err == nil {
		return nil
	}
	a := &Annotator{err, nil, stackContext{frameInfo: stackFrameInfoForError(1, err)}}
	return a.Tag(t).Err()
}

// BoolTag is deprecated.
//
// Deprecated: Use go.chromium.org/common/errors/errtag instead.
type BoolTag struct{ Key TagKey }

// GenerateErrorTagValue implements TagValueGenerator, and returns a default
// value for the tag of `true`. If you want to set this BoolTag value to false,
// use BoolTag.Off().
func (b BoolTag) GenerateErrorTagValue() (key, value any) { return b.Key, true }

// Off allows you to "remove" this boolean tag from an error (by setting it to
// false).
func (b BoolTag) Off() TagValue { return TagValue{b.Key, false} }

// Apply is a shortcut for With(true).Apply(err)
func (b BoolTag) Apply(err error) error {
	if err == nil {
		return nil
	}
	a := &Annotator{err, nil, stackContext{frameInfo: stackFrameInfoForError(1, err)}}
	return a.Tag(b).Err()
}

// In returns true iff this tag value has been set to true on this error.
func (b BoolTag) In(err error) bool {
	v, ok := TagValueIn(b.Key, err)
	if !ok {
		return false
	}
	return v.(bool)
}

// NewTagKey creates a new TagKey.
//
// Use this with a BoolTag or your own custom tag implementation.
//
// Example (bool tag):
//
//	var myTag = errors.BoolTag{Key: errors.NewTagKey("this error is a user error")}
//
//	err = myTag.Apply(err)
//	myTag.In(err) // == true
//
//	err2 := myTag.Off().Apply(err)
//	myTag.In(err2) // == false
//
// Example (custom tag)
//
//	type SomeType int
//	type myTag struct { Key errors.TagKey }
//	func (m myTag) With(value SomeType) errors.TagValue {
//	  return errors.TagValue{Key: m.Key, Value: value}
//	}
//	func (m myTag) In(err error) (v SomeType, ok bool) {
//	  d, ok := errors.TagValueIn(m.Key, err)
//	  if ok {
//	    v = d.(SomeType)
//	  }
//	  return
//	}
//	var MyTag = myTag{errors.NewTagKey("has a SomeType")}
//
// You could then use it like:
//
//	err = MyTag.With(100).Apply(err)
//	MyTag.In(err) // == true
//	errors.ValueIn(err) // == (SomeType(100), true)
//
// Deprecated: Use go.chromium.org/common/errors/errtag instead.
func NewTagKey(description string) TagKey {
	return &tagDescription{description}
}

// GetTags returns a map of all TagKeys set in this error to their value.
//
// A nil value means that the tag is present, but has a nil associated value.
//
// This is done in a depth-first traversal of the error stack, with the
// most-recently-set value of the tag taking precedence.
//
// Deprecated: Use go.chromium.org/common/errors/errtag instead.
func GetTags(err error) map[TagKey]any {
	ret := map[TagKey]any{}
	Walk(err, func(err error) bool {
		if sc, ok := err.(stackContexter); ok {
			ctx := sc.stackContext()
			for k, v := range ctx.tags {
				if _, ok := ret[k]; !ok {
					ret[k] = v
				}
			}
		}
		return true
	})
	return ret
}
