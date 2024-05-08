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
)

func TestBeIn(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeIn(7, 8, 9, 10)(10)))
	t.Run("string", shouldPass(BeIn("hi", "there")("hi")))

	t.Run("string missing", shouldFail(BeIn("hi", "there")("morp")))
}

func TestNotBeIn(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(NotBeIn(7, 8, 9, 10)(11)))
	t.Run("string", shouldPass(NotBeIn("hi", "there")("morp")))

	t.Run("int present", shouldFail(NotBeIn(1, 2, 3)(1)))
}

func TestContain(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(Contain(10)([]int{7, 8, 9, 10})))
	t.Run("string", shouldPass(Contain("hi")([]string{"hi", "there"})))

	t.Run("int missing", shouldFail(Contain(9)([]int{1, 2, 3})))
}

func TestNotContain(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(NotContain(10)([]int{1, 2, 3})))

	t.Run("int present", shouldFail(NotContain(2)([]int{1, 2, 3})))
	t.Run("string present", shouldFail(NotContain("hi")([]string{"hi", "there"})))
}
