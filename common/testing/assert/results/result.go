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

package results

// Comparison takes in an item-to-be-compared and returns a Result.
type Comparison[T any] func(T) *Result

// Result is the data returned from a [Comparison].
//
// It represents a successful or failed comparison.
type Result struct {
	failed bool
}

// Ok returns whether a Result represents success or failure.
func (r *Result) Ok() bool {
	return r == nil || !r.failed
}

// Equal returns whether two Results are semantically equal or not.
func (r *Result) Equal(s *Result) bool {
	return r.Ok() == s.Ok()
}
