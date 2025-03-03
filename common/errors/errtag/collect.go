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
	"sync"
)

// We unfortunately need to keep track of TagKey -> MergeFn
var tagKeyToMergeFn = map[TagKey]func(valuePtrs []any) any{}
var tagKeyToMergeFnMu sync.RWMutex

func registerMergeFn[T comparable](key TagKey, merge MergeFn[T]) {
	mfn := func(valuePtrs []any) any {
		arg := make([]*T, len(valuePtrs))
		for i, v := range valuePtrs {
			arg[i] = v.(*T)
		}
		return merge(arg)
	}

	tagKeyToMergeFnMu.Lock()
	tagKeyToMergeFn[key] = mfn
	tagKeyToMergeFnMu.Unlock()
}

func doMerge(key TagKey, valuePtrs []any) any {
	tagKeyToMergeFnMu.RLock()
	mfn := tagKeyToMergeFn[key]
	tagKeyToMergeFnMu.RUnlock()
	if mfn == nil {
		panic(fmt.Errorf("impossible: tagkey %q has no merge function", key))
	}
	return mfn(valuePtrs)
}

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

type collectedMultiValue map[TagKey][]any

func (c *collectedMultiValue) set(key TagKey, valuePtr any) {
	if *c == nil {
		*c = collectedMultiValue{}
	}
	(*c)[key] = []any{valuePtr}
}

func (c *collectedMultiValue) update(other CollectedValues) {
	if *c == nil {
		*c = collectedMultiValue{}
	}
	for key, value := range other {
		(*c)[key] = append((*c)[key], value)
	}
}

func (c *collectedMultiValue) flatten() CollectedValues {
	if len(*c) == 0 {
		return nil
	}
	ret := make(CollectedValues, len(*c))
	for key, valuePtrs := range *c {
		if lv := len(valuePtrs); lv == 1 {
			ret[key] = valuePtrs[0]
		} else if lv > 1 {
			ret[key] = doMerge(key, valuePtrs)
		}
	}
	return ret
}

// String renders the CollectedValues to a multi-line string.
//
// NOTE: If multiple TagKey's were created with the same description text, it is
// possible to see the same descriptive string on the left hand side multiple
// times. This probably indicates that there is a bug in the program (possibly
// due to copy/pasting a [MakeTag] invocation, or using MakeTag in a way where the
// output is not correctly reused across the process).
func (c CollectedValues) String() string {
	bld := strings.Builder{}
	keys := slices.Collect(maps.Keys(c))
	slices.SortFunc(keys, func(a, b TagKey) int {
		return cmp.Compare(*a.unique, *b.unique)
	})
	for _, k := range keys {
		fmt.Fprintf(&bld, "%s: %#v\n", k, c[k])
	}
	// TrimSpace to remove trailing \n
	return strings.TrimSpace(bld.String())
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
	var collectPtrs func(err error, exclude tagSet) CollectedValues
	collectPtrs = func(err error, exclude tagSet) CollectedValues {
		var mval collectedMultiValue

		// first, check to see if we're a tag - if we are, then ignore this tag
		// anywhere down the tree.
		var alsoExclude TagKey
		if werr, ok := err.(wrappedErrIface); ok {
			key, valPtr := werr.tagValuePtr()
			if !exclude.has(key) {
				mval.set(key, valPtr)
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
				mval.update(collectPtrs(other, exclude))
			}

		case interface{ Unwrap() error }:
			if alsoExclude.Valid() {
				exclude = maps.Clone(exclude)
				exclude.add(alsoExclude)
			}
			mval.update(collectPtrs(x.Unwrap(), exclude))
		}

		// we need to flatten because in the case of a multi-error we could have
		// multiple values for the same tag - mval.flatten() will apply the merge
		// function associated with the tag for those values, giving just a single
		// *T back.
		return mval.flatten()
	}

	retPtrs := collectPtrs(err, excludedKeys)
	if len(retPtrs) == 0 {
		return nil
	}

	// dereference all the values to get just plain T's
	ret := make(CollectedValues, len(retPtrs))
	for key, valPtr := range retPtrs {
		ret[key] = reflect.ValueOf(valPtr).Elem().Interface()
	}
	return ret
}
