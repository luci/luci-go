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
	"math"
	"testing"
)

func TestEqual(t *testing.T) {
	t.Parallel()

	t.Run("int equal", shouldPass(Equal(10)(10)))

	t.Run("int inequal", shouldFail(Equal(10)(100), "Expected"))
	t.Run("int NaN", shouldFail(Equal(math.NaN())(10.1), "should.BeNaN"))

	a := 100
	b := 100
	t.Run("pointer inequal", shouldFail(Equal(&a)(&b), "did you want should.Match"))
}

func TestNotEqual(t *testing.T) {
	t.Parallel()

	t.Run("int inequal", shouldPass(NotEqual(10)(100)))

	t.Run("int equal", shouldFail(NotEqual(10)(10), "Actual"))
	t.Run("int NaN", shouldFail(NotEqual(math.NaN())(10.1), "should.BeNaN"))
}
