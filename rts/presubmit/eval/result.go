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

import (
	"fmt"
	"io"
)

// Result is the result of evaluation.
type Result struct {
	AnalyzedPatchSets int
	Safety            Safety
	// TODO(crbug.com/1112125): add Precision.
}

// Safety is evaluation of safety of an RTS algorithm.
// A safe algorithm does not let bad CLs pass CQ.
type Safety struct {
	// EligiblePatchSets is the number of patchsets eligible for safety
	// evaluation. All of these patchsets were rejected.
	EligiblePatchSets int

	// Rejected is the number of eligible patchsets that would be rejected
	// if the RTS algorithm in question was used.
	// Ideally this equals EligiblePatchSets.
	Rejected int
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer) (err error) {
	switch {
	case r.AnalyzedPatchSets == 0:
		_, err = fmt.Printf("Evaluation failed: patchsets not found\n")

	case r.Safety.EligiblePatchSets == 0:
		_, err = fmt.Printf("Evaluation failed: all %d patchsets are ineligible.\n", r.AnalyzedPatchSets)

	default:
		fmt.Printf("Total analyzed patchsets: %d\n", r.AnalyzedPatchSets)
		_, err = fmt.Printf("Safety score: %.2f (%d/%d)\n",
			float64(r.Safety.Rejected)/float64(r.Safety.EligiblePatchSets),
			r.Safety.Rejected,
			r.Safety.EligiblePatchSets,
		)
	}
	return
}
