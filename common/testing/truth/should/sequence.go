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

// BeIn returns a Comparison which checks to see if `actual` is equal to any of
// the entries in `items`.
//
// Comparison is done via simple equality check.
func BeIn[T comparable](collection ...T) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		for _, obj := range collection {
			if actual == obj {
				return nil
			}
		}
		return comparison.NewSummaryBuilder("should.BeIn", actual).
			AddFindingf("Item", "%#v", actual).
			AddFindingf("Collection", "%#v", collection).
			Summary
	}
}

// NotBeIn returns a Comparison which checks to see if `actual` is not equal to
// any of the entries in `collection`.
//
// Comparison is done via simple equality check.
func NotBeIn[T comparable](collection ...T) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		for _, item := range collection {
			if actual == item {
				return comparison.NewSummaryBuilder("should.NotBeIn", actual).
					AddFindingf("Item", "%#v", actual).
					AddFindingf("Collection", "%#v", collection).
					Summary
			}
		}
		return nil
	}
}

// Contain returns a Comparison which checks to see if `actual` contains `item`.
//
// Comparison is done via simple equality check.
func Contain[T comparable](target T) comparison.Func[[]T] {
	return func(actual []T) *failure.Summary {
		for _, item := range actual {
			if item == target {
				return nil
			}
		}
		return comparison.NewSummaryBuilder("should.Contain", actual).
			AddFindingf("Collection", "%#v", actual).
			AddFindingf("Item", "%#v", target).
			Summary
	}
}

// NotContain returns a Comparison which checks to see if `actual` does not contain `item`.
//
// Comparison is done via simple equality check.
func NotContain[T comparable](target T) comparison.Func[[]T] {
	return func(actual []T) *failure.Summary {
		for _, item := range actual {
			if item == target {
				return comparison.NewSummaryBuilder("should.NotContain", actual).
					AddFindingf("Collection", "%#v", actual).
					AddFindingf("Item", "%#v", target).
					Summary
			}
		}
		return nil
	}
}
