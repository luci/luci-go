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

package comparison

import (
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

// TestResultRender tests rendering a failure.
func TestResultRender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		failure *Failure
		lines   []string
	}{
		{
			name:    "nil",
			failure: nil,
			lines:   nil,
		},
		{
			name:    "empty",
			failure: &Failure{},
			lines: []string{
				`UNKNOWN COMPARISON FAILED`,
			},
		},
		{
			name: "simple failure",
			failure: &Failure{
				Comparison: &Failure_ComparisonFunc{
					Name: "testfunc",
				},
			},
			lines: []string{
				`testfunc FAILED`,
			},
		},
		{
			name: "failure with value",
			failure: &Failure{
				Comparison: &Failure_ComparisonFunc{Name: "equal", TypeArguments: []string{"int", "int"}},
				Findings: []*Failure_Finding{
					{Name: "left", Value: "4"},
					{Name: "right", Value: "7"},
				},
			},
			lines: []string{
				`equal[int, int] FAILED`,
				`  left: 4`,
				`  right: 7`,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Diff(RenderCLI(tt.failure), tt.lines); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
