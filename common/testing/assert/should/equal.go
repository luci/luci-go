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

	"go.chromium.org/luci/common/testing/assert/results"
)

// Equal checks whether two objects are equal.
func Equal[T comparable](expected T) results.Comparison[T] {
	cmpName := "should.Equal"
	rtype := reflect.TypeOf(expected)

	return func(actual T) (ret *results.Result) {
		// The weird-looking second condition is for floats.
		// NaN doesn't compare equal to itself.
		//
		// I don't know whether it's better to paper over this footgun in the way that
		// I have or go with the simpler code `actual == expected`.
		if actual == expected || (actual != actual && expected != expected) {
			return nil
		}

		return results.NewResultBuilder().
			SetName(cmpName, rtype).
			Actual(actual).
			Expected(expected).
			Result()
	}
}
