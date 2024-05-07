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

func TestHaveLength(t *testing.T) {
	t.Parallel()

	t.Run("string", shouldPass(HaveLength(5)("hello")))
	t.Run("slice", shouldPass(HaveLength(5)([]int{1, 2, 3, 4, 5})))
	t.Run("map", shouldPass(HaveLength(5)(map[int]struct{}{
		1: {},
		2: {},
		3: {},
		4: {},
		5: {},
	})))
	t.Run("array", shouldPass(HaveLength(5)([5]int{})))
	t.Run("*array", shouldPass(HaveLength(5)(&[5]int{})))

	ch := make(chan int, 5)
	for i := 0; i < 5; i++ {
		ch <- i
	}
	t.Run("chan", shouldPass(HaveLength(5)(ch)))

	t.Run("fail length", shouldFail(HaveLength(7)("hello"), "len(Actual)", "Expected"))
	t.Run("negative length", shouldFail(HaveLength(-5)("hello"), "negative", "Expected"))
	t.Run("non-lengthable", shouldFail(HaveLength(5)(struct{}{}), "does not support"))
}

func TestBeEmpty(t *testing.T) {
	t.Parallel()

	t.Run("string", shouldPass(BeEmpty("")))
	t.Run("slice", shouldPass(BeEmpty([]int{})))
	t.Run("slice(nil)", shouldPass(BeEmpty([]int(nil))))

	t.Run("non-empty slice", shouldFail(BeEmpty([]int{1})))
	t.Run("non-lengthable", shouldFail(BeEmpty(struct{}{}), "does not support"))
}

func TestNotBeEmpty(t *testing.T) {
	t.Parallel()

	t.Run("non-empty slice", shouldPass(NotBeEmpty([]int{1})))

	t.Run("empty string", shouldFail(NotBeEmpty("")))
	t.Run("non-lengthable", shouldFail(NotBeEmpty(struct{}{}), "does not support"))
}
