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

import "go.chromium.org/luci/common/testing/assert/comparison"

// BeTrue is a comparison.Func[bool] which asserts that the actual value is `true`.
func BeTrue(actual bool) *comparison.Failure {
	if actual {
		return nil
	}
	return comparison.NewFailureBuilder("should.BeTrue").Failure
}

// BeFalse is a comparison.Func[bool] which asserts that the actual value is `false`.
func BeFalse(actual bool) *comparison.Failure {
	if !actual {
		return nil
	}
	return comparison.NewFailureBuilder("should.BeFalse").Failure
}