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

package eval

import "fmt"

// Result is the result of evaluation.
type Result struct {
	AnalyzedPatchSets int
	Safety            Safety
	Precision         Precision
}

// Safety is evaluation of safety of an RTS algorithm.
// A safe algorithm does not let bad CLs pass CQ.
type Safety struct {
	// EligablePatchSets is the number of patchsets eligable for safety
	// evaluations. All of these patchsets failed.
	EligablePatchSets int

	// Rejected is the number of eligable patchsets that would be rejected by CQ
	// the RTS algorithm was used. Ideally this equals EligablePatchSets.
	Rejected int
}

// Precision is evaluation of precision of an RTS algorithm.
// A precise algorithm does not run unnecessary tests, thus reducing the cost
// of testing.
type Precision struct {
}

// Print prints the results to stdout.
func (r *Result) Print() {
	switch {
	case r.AnalyzedPatchSets == 0:
		fmt.Printf("Evaluation failed: patchsets not found\n")
		return

	case r.Safety.EligablePatchSets == 0:
		fmt.Printf("Evaluation failed: all %d patchsets are ineligable.\n", r.AnalyzedPatchSets)
		return
	}

	fmt.Printf("Total analyzed patchsets: %d\n", r.AnalyzedPatchSets)
	fmt.Printf(
		"Safety score: %.2f (%d/%d)\n",
		float64(r.Safety.Rejected)/float64(r.Safety.EligablePatchSets),
		r.Safety.Rejected,
		r.Safety.EligablePatchSets,
	)
}
