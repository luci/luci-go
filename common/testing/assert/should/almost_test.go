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

func TestAlmostEqual(t *testing.T) {
	t.Parallel()

	t.Run("basic", shouldPass(AlmostEqual(100.0)(100.0000000000000000000000001)))
	t.Run("basic w/ threshold", shouldPass(AlmostEqual(100.0, 21.0)(121.0000000000000000000000001)))

	t.Run("float64 default epsilon", shouldFail(
		AlmostEqual(100.0)(100.00000000000001),
		"Because", "1.4210854715202004e-14 off of target",
		"Expected", "100 ± 2.220446049250313e-16",
	))

	t.Run("float32 default epsilon", shouldFail(
		// (note: float32 is substantially lower fidelity than float64).
		AlmostEqual[float32](100.0)(100.00001),
		"Because", "7.6293945e-06 off of target",
		"Expected", "100 ± 1.1920929e-07",
	))

	t.Run("float64 epsilon fail", shouldFail(
		AlmostEqual(100.0, 21)(121.00000000000001),
		"Because", "21.000000000000014 off of target",
		"Expected", "100 ± 21",
	))

	t.Run("float64 epsilon fail low", shouldFail(
		AlmostEqual(100.0, 21)(100-21.00000000000001),
		"Actual", "78.99999999999999",
		"Because", "21.000000000000014 off of target",
		"Expected", "100 ± 21",
	))

	t.Run("float32 epsilon fail low", shouldFail(
		AlmostEqual[float32](100.0, 21)(121.00001),
		"Because", "21.000008 off of target",
		"Expected", "100 ± 21",
	))

	t.Run("check float32 with float64 math", shouldFail(
		// float64(float32(...)) emulates using AssertL with this comparison.
		AlmostEqual(100.0, 21)(float64(float32(121.00001))),
		"Because", "21.00000762939453 off of target",
		"Expected", "100 ± 21",
	))

	t.Run("negative epsilon", shouldFail(AlmostEqual(100.0, -21)(100), "is negative"))
	t.Run("double epsilons", shouldFail(AlmostEqual(100.0, -21, 200)(100), "single optional value", "got 2"))
}
