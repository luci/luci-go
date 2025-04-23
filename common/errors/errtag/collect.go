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
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

type tagSet map[TagKey]struct{}

func (t *tagSet) add(key TagKey) {
	if *t == nil {
		*t = tagSet{}
	}
	(*t)[key] = struct{}{}
}

func (t tagSet) has(key TagKey) bool {
	_, ret := t[key]
	return ret
}

// CollectedValues maps Tag description to one or more values for an error,
// obtained via [Collect].
type CollectedValues map[TagKey]any

type mergeableCollectedValuesItem struct {
	valuePtrs []any
	// mergeFn will only be set if len(valuePtrs) > 1
	mergeFn func(valuePtrs []any) any
}

func (m *mergeableCollectedValuesItem) flatten() any {
	var valuePtr any
	if len(m.valuePtrs) == 1 {
		valuePtr = m.valuePtrs[0]
	} else {
		valuePtr = m.mergeFn(m.valuePtrs)
	}
	return valuePtr
}

type mergeableCollectedValues map[TagKey]*mergeableCollectedValuesItem

func (m *mergeableCollectedValues) set(key TagKey, valPtr any, merge func(valuePtrs []any) any) {
	if *m == nil {
		*m = mergeableCollectedValues{}
	}
	(*m)[key] = &mergeableCollectedValuesItem{valuePtrs: []any{valPtr}, mergeFn: merge}
}

func (m *mergeableCollectedValues) update(other mergeableCollectedValues) {
	if *m == nil {
		*m = mergeableCollectedValues{}
	}
	for key, value := range other {
		if cur, ok := (*m)[key]; ok {
			cur.valuePtrs = append(cur.valuePtrs, value.valuePtrs...)
		} else {
			(*m)[key] = value
		}
	}
}

func (m mergeableCollectedValues) flatten() CollectedValues {
	if len(m) == 0 {
		return nil
	}
	ret := make(CollectedValues, len(m))
	for key, item := range m {
		// recall that the merge function may return nil to indicate 'remove this
		// value'
		val := item.flatten()
		if val != nil {
			// ret contains dereferenced values
			ret[key] = reflect.ValueOf(val).Elem().Interface()
		}
	}
	return ret
}

// String renders the CollectedValues to a single multi-line string with no
// prefix and no trailing newline.
//
// See note on [CollectedValues.Format].
func (c CollectedValues) String() string {
	return strings.Join(c.Format(""), "\n")
}

// Format renders the CollectedValues to an array of lines.
//
// `prefix` will be inserted verbatim before each line.
//
// Each line will have one `TagKey.String(): value` and will not include
// a newline.
//
// NOTE: If multiple TagKey's were created with the same description text, it is
// possible to see the same descriptive string on the left hand side multiple
// times. This probably indicates that there is a bug in the program (possibly
// due to copy/pasting a [MakeTag] invocation, or using MakeTag in a way where the
// output is not correctly reused across the process).
func (c CollectedValues) Format(prefix string) []string {
	keys := slices.SortedFunc(maps.Keys(c), func(a, b TagKey) int {
		return cmp.Compare(*a.unique, *b.unique)
	})
	ret := make([]string, len(c))
	for i, k := range keys {
		ret[i] = fmt.Sprintf("%s%q: %#v", prefix, k, c[k])
	}
	return ret
}

// Collect scans the error for all known tag keys and returns a map of tag
// description to the current value for that tag.
func Collect(err error, exclude ...TagType) CollectedValues {
	var excludedKeys tagSet
	if len(exclude) > 0 {
		excludedKeys = make(tagSet, len(exclude))
		for _, ek := range exclude {
			excludedKeys[ek.Key()] = struct{}{}
		}
	}

	// NOTE: CollectedValues will return *T values instead of T values, because of
	// wrappedErr.tagValuePtr.
	var collectPtrs func(err error, exclude tagSet) mergeableCollectedValues
	collectPtrs = func(err error, exclude tagSet) mergeableCollectedValues {
		var ret mergeableCollectedValues

		// first, check to see if we're a tag - if we are, then ignore this tag
		// anywhere down the tree.
		var alsoExclude TagKey
		if werr, ok := err.(wrappedErrIface); ok {
			key, valPtr, merge := werr.tagValuePtr()
			if !exclude.has(key) {
				ret.set(key, valPtr, merge)
				alsoExclude = key
			}
		}

		// next, recurse, ignoring our own tag, if one is set.
		switch x := err.(type) {
		case interface{ Unwrap() []error }:
			if alsoExclude.Valid() {
				exclude = maps.Clone(exclude)
				exclude.add(alsoExclude)
			}
			for _, other := range x.Unwrap() {
				ret.update(collectPtrs(other, exclude))
			}

		case interface{ Unwrap() error }:
			if alsoExclude.Valid() {
				exclude = maps.Clone(exclude)
				exclude.add(alsoExclude)
			}
			ret.update(collectPtrs(x.Unwrap(), exclude))
		}

		return ret
	}

	return collectPtrs(err, excludedKeys).flatten()
}
