// Copyright 2020 The LUCI Authors.
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

package rts

// Affectedness is how much a test is affected by the code change.
// The zero value means a test is affected and unranked.
type Affectedness struct {
	// Distance is a non-negative number, where 0.0 means the code change is
	// extremely likely to affect the test, and +inf means extremely unlikely.
	// If a test's distance is less or equal than a given MaxDistance threshold,
	// then the test is selected.
	// A selection strategy doesn't have to use +inf as the upper boundary if the
	// threshold uses the same scale.
	Distance float64

	// Rank orders all tests in the repository by Distance, with rank 1 assigned
	// to the test with the lowest distance.
	// Iff two tests have the same distance, then they may, but don't have to,
	// have same the same rank.
	// Zero rank means the rank is unknown, e.g. if the selection strategy is not
	// aware of other tests.
	//
	// Example:
	// There are three tests exists in the codebase: T1, T2 and T3.
	// Their distances are 1, 2 and 3, respectively.
	// Input.TestVariants has only one element and it refers to T2.
	// Then Output.TestVariantAffectedness[0].Rank should be 2.
	Rank int
}
