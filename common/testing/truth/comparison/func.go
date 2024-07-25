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

package comparison

import (
	"fmt"

	"go.chromium.org/luci/common/data"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Func takes in a value-to-be-compared and returns a failure.Summary if the value
// does not meet the expectation of this comparison.Func.
//
// Example:
//
//	func BeTrue(value bool) *failure.Summary {
//	  if !value {
//	    return comparison.NewSummaryBuilder("should.BeTrue").Summary
//	  }
//	  return nil
//	}
//
// In this example, BeTrue is a comparison.Func.
type Func[T any] func(T) *failure.Summary

// CastCompare allows you to compare a value `actual` of type `any` with
// a specifically-typed Func[T].
//
// This uses data.LosslessConvertTo[T] to ensure that the underlying type of
// `actual` can fit inside of the specified type T without data loss.
//
// If data loss could occur, this returns a new failure.Summary describing the
// type mismatch.
//
// Otherwise, this returns the result of comparison(T(actual)).
func (compare Func[T]) CastCompare(actual any) *failure.Summary {
	converted, ok := data.LosslessConvertTo[T](actual)
	if ok {
		return compare(converted)
	}

	sb := NewSummaryBuilder(fmt.Sprintf("comparison.Func[%T].CastCompare", converted))
	sb.Findings = append(sb.Findings, &failure.Finding{
		Name:  "ActualType",
		Value: []string{fmt.Sprintf("%T", actual)},
	})
	return sb.Summary
}
