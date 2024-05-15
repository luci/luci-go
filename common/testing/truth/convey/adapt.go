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

// Package convey provides a temporary symbol which converts a convey-style
// comparison of the type:
//
//	func(actual any, expected ...any) string
//
// Into an truth compatible symbol:
//
//	comparison.Func[any]
package convey

import (
	"fmt"
	"reflect"
	"runtime"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Adapt takes an old convey-style comparison function and converts it to a
// comparison.Func[any].
//
// This is intended to be a temporary tool which should only be used during the
// transitional period where we switch away from Convey-style assertions. Code
// using this adapter should be rewritten to exclusively use truth/comparison.
//
// # Example
//
//	assert.That(t, actualValue, convey.Adapt(myCustomShouldFunction)(expected))
func Adapt(oldComparison func(actual any, expected ...any) string) func(expected ...any) comparison.Func[any] {
	return func(expected ...any) comparison.Func[any] {
		return func(actual any) *failure.Summary {
			text := oldComparison(actual, expected...)
			if text == "" {
				return nil
			}
			name := runtime.FuncForPC(reflect.ValueOf(oldComparison).Pointer()).Name()
			return comparison.NewSummaryBuilder(fmt.Sprintf("adapt.Convey(%s)", name)).Because(text).Summary
		}
	}
}
