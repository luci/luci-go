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

// Func takes in a value-to-be-compared and returns a Failure if the value
// does not meet the expectation of this comparison.Func.
//
// Example:
//
//	func BeTrue(value bool) *Failure {
//	  if !value { return NewFailureBuilder("should.BeTrue").(*Failure) }
//	  return nil
//	}
//
// In this example, BeTrue is a comparison.Func.
type Func[T any] func(T) *Failure
