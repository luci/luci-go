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

package should

import (
	"reflect"

	"go.chromium.org/luci/common/data"
	"go.chromium.org/luci/common/testing/assert/comparison"
)

// ContainKey returns a comparison.Func which checks to see if some `map[~K]?`
// contains a given key.
//
// Example: `Assert(t, someMap, should.ContainKey("someKey"))`
//
// NOTE: Go doesn't have something like a `rest` type where we could express
// that this is an `comparison.Func[map[K]?]` so we have to accept `any` here.
func ContainKey[K comparable](key K) comparison.Func[any] {
	const cmpName = "should.ContainKey"

	return func(actual any) *comparison.Failure {
		has, fail := mapContainsImpl(cmpName, actual, key)
		if has || fail != nil {
			return fail
		}
		return comparison.NewFailureBuilder(cmpName, key).Expected(key).Failure
	}
}

// NotContainKey returns a comparison.Func which checks to see if some
// `map[~K]?` does not contain a given key.
//
// Example: `Assert(t, someMap, should.NotContainKey("someKey"))`
//
// NOTE: Go doesn't have something like a `rest` type where we could express
// that this is an `comparison.Func[map[K]?]` so we have to accept `any` here.
func NotContainKey[K comparable](key K) comparison.Func[any] {
	const cmpName = "should.NotContainKey"

	return func(actual any) *comparison.Failure {
		has, fail := mapContainsImpl(cmpName, actual, key)
		if !has || fail != nil {
			return fail
		}
		return comparison.NewFailureBuilder(cmpName, key).
			AddFindingf("Unexpected Key", "%#v", key).
			Failure
	}
}

// mapContainsImpl implements ContainKey and NotContainKey.
//
// This requires that aMap is a map type with a key type ~K (value can be
// any type).
//
// Returns has=true if aMap contains key. Returns fail != nil if aMap is not
// a map type or does not have a key type which `key` can convert to losslessly.
func mapContainsImpl[K comparable](cmpName string, aMap any, key K) (has bool, fail *comparison.Failure) {
	aMapT := reflect.TypeOf(aMap)
	if aMapT.Kind() != reflect.Map {
		return false, comparison.NewFailureBuilder(cmpName).
			Because("Actual is not a map (got %T)", aMap).
			Failure
	}
	keyT := aMapT.Key()

	toLookup := reflect.New(keyT).Elem()
	if !data.LosslessConvertToReflect(key, toLookup) {
		return false, comparison.NewFailureBuilder(cmpName).
			Because("map's key type (%s) is not convertible to %T", keyT, key).
			Failure
	}
	val := reflect.ValueOf(aMap).MapIndex(toLookup)
	return val.IsValid(), nil
}
