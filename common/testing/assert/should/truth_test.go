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

import "testing"

func TestTruth(t *testing.T) {
	t.Parallel()

	t.Run("simple T/T", shouldPass(BeTrue(true)))
	t.Run("simple T/F", shouldFail(BeTrue(false)))
	t.Run("simple F/T", shouldFail(BeFalse(true)))
	t.Run("simple F/F", shouldPass(BeFalse(false)))
}
