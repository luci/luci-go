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
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// HaveType checks that the underlying type of `actual` is T.
//
// Use with {assert,check}.Loosely to see if some interface value has the
// specified type:
//
//	assert.Loosely(t, actual, should.HaveType[expectedType])
//
// This works because assert.Loosely will ensure that `actual` can cast
// losslessly to `expectedType`, and display an error if this fails.
func HaveType[T any](actual T) *failure.Summary {
	return nil
}

// StaticallyHaveSameTypeAs checks if two values statically have the same type.
//
// Like HaveType, it will fail to compile if the types are incompatible.
func StaticallyHaveSameTypeAs[T any](expected T) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		return nil
	}
}
