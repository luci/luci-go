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

func TestBeNaN(t *testing.T) {
	t.Parallel()

	t.Run("non-NaN equal", shouldPass(BeNaN(math.NaN())))
	t.Run("non-NaN equal (32)", shouldPass(BeNaN(float32(math.NaN()))))

	t.Run("not NaN", shouldFail(BeNaN(10.1), "Actual"))
}

func TestNotBeNan(t *testing.T) {
	t.Parallel()

	t.Run("7.0 is not NaN", shouldPass(NotBeNaN(7.0)))
	t.Run("NaN, however, is NaN", shouldFail(NotBeNaN(math.NaN())))
}
