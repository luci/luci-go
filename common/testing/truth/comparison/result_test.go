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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/internal/typed"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// TestResultRender tests rendering a failure.
func TestResultRender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prefix  string
		render  RenderCLI
		failure *failure.Summary
		lines   []string
	}{
		{
			name:    "nil",
			failure: nil,
			lines:   nil,
		},
		{
			name:    "empty",
			failure: &failure.Summary{},
			lines: []string{
				`UNKNOWN COMPARISON FAILED`,
			},
		},
		{
			name: "simple failure",
			failure: &failure.Summary{
				Comparison: &failure.Comparison{
					Name: "testfunc",
				},
			},
			lines: []string{
				`testfunc FAILED`,
			},
		},
		{
			name: "failure with value",
			failure: &failure.Summary{
				Comparison: &failure.Comparison{Name: "equal", TypeArguments: []string{"int", "int"}},
				Findings: []*failure.Finding{
					{Name: "left", Value: []string{"4"}},
					{Name: "right", Value: []string{"7"}},
				},
			},
			lines: []string{
				`equal[int, int] FAILED`,
				`left: 4`,
				`right: 7`,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Diff(tt.render.Summary(tt.prefix, tt.failure), strings.Join(tt.lines, "\n")); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
