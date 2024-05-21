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
	"fmt"
	"testing"
)

func TestBeNil(t *testing.T) {
	t.Parallel()

	t.Run("untyped", shouldPass(BeNil(nil)))
	t.Run("slice", shouldPass(BeNil([]int(nil))))
	t.Run("pointer", shouldPass(BeNil((*struct{})(nil))))

	// error is specially cased
	t.Run("error", shouldFail(BeNil(fmt.Errorf("what")), "ErrLikeError"))
	t.Run("empty string not nil", shouldFail(BeNil(""), "cannot be checked for nil"))
	t.Run("non-empty slice", shouldFail(BeNil([]string{"yo"}), "Actual"))
}

func TestNotBeNil(t *testing.T) {
	t.Parallel()

	t.Run("non-empty slice", shouldPass(NotBeNil([]string{"yo"})))
	t.Run("pointer", shouldPass(NotBeNil(&struct{}{})))

	t.Run("untyped", shouldFail(NotBeNil(nil)))
	t.Run("slice", shouldFail(NotBeNil([]int(nil))))
	t.Run("channel", shouldFail(NotBeNil((chan int)(nil))))

	t.Run("empty string not nil", shouldFail(NotBeNil(""), "cannot be checked for nil"))
}

func TestBeZero(t *testing.T) {
	t.Parallel()

	t.Run("nil", shouldPass(BeZero(nil)))
	t.Run("nil slice", shouldPass(BeZero([]int(nil))))
	t.Run("nil pointer", shouldPass(BeZero((*struct{})(nil))))
	t.Run("empty string", shouldPass(BeZero("")))
	t.Run("int", shouldPass(BeZero(0)))

	t.Run("allocated slice", shouldFail(BeZero([]int{})))
}

func TestNotBeZero(t *testing.T) {
	t.Parallel()

	t.Run("string", shouldPass(NotBeZero("hi")))
	t.Run("int", shouldPass(NotBeZero(100)))

	t.Run("nil", shouldFail(NotBeZero(nil)))
	t.Run("empty string", shouldFail(NotBeZero("")))
}
