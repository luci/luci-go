// Copyright 2023 The LUCI Authors.
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
)

func TestContainKey(t *testing.T) {
	t.Parallel()

	t.Run("contains", shouldPass(ContainKey("key")(map[string]int{
		"key": 1,
	})))

	t.Run("missing", shouldFail(ContainKey("notkey")(map[string]int{
		"key": 1,
	}), "Expected"))

	t.Run("wrong keytype", shouldFail(ContainKey(10)(map[string]int{}), "not convertible"))
	t.Run("not a map", shouldFail(ContainKey(10)("sandwich"), "not a map"))
}

func TestNotContainKey(t *testing.T) {
	t.Parallel()

	t.Run("missing", shouldPass(NotContainKey("notkey")(map[string]int{
		"key": 1,
	})))
	t.Run("missing (nil)", shouldPass(NotContainKey("notkey")(map[string]int(nil))))

	t.Run("contains", shouldFail(NotContainKey("key")(map[string]int{
		"key": 1,
	}), "Unexpected Key"))

	t.Run("wrong keytype", shouldFail(NotContainKey(10)(map[string]int{}), "not convertible"))
	t.Run("not a map", shouldFail(NotContainKey(10)("sandwich"), "not a map"))
}
