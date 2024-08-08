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
	"testing"

	"go.chromium.org/luci/common/testing/truth/comparison"
)

func TestHasType(t *testing.T) {
	var fn comparison.Func[string] = HaveType[string]
	var actual any

	actual = "hello"
	shouldPass(fn.CastCompare(actual))

	actual = 100
	shouldFail(fn.CastCompare(actual))
}
