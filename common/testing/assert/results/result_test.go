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

package results

import (
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

// TestResultOk tests whether a result indicates a successful or failed result based on the internals of the Result object.
func TestResultOk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input *OldResult
		ok    bool
	}{
		{
			name:  "empty",
			input: nil,
			ok:    true,
		},
		{
			name:  "nil",
			input: &OldResult{},
			ok:    true,
		},
		{
			name: "failed result",
			input: &OldResult{
				failed: true,
			},
			ok: false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Diff(tt.input.Ok(), tt.ok); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestResultEquals tests comparing two results semantically for equality.
func TestResultEquals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lhs   *OldResult
		rhs   *OldResult
		equal bool
	}{
		{
			name:  "both nil",
			lhs:   nil,
			rhs:   nil,
			equal: true,
		},
		{
			name:  "both nil",
			lhs:   nil,
			rhs:   &OldResult{},
			equal: true,
		},
		{
			name: "different headers",
			lhs: &OldResult{
				failed: true,
				header: resultHeader{
					comparison: "a",
				},
			},
			rhs: &OldResult{
				failed: true,
				header: resultHeader{
					comparison: "b",
				},
			},
			equal: false,
		},
		{
			name: "same headers",
			lhs: &OldResult{
				failed: true,
				header: resultHeader{
					comparison: "a",
				},
			},
			rhs: &OldResult{
				failed: true,
				header: resultHeader{
					comparison: "a",
				},
			},
			equal: true,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Diff(tt.lhs.Equal(tt.rhs), tt.equal); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestResultRender tests rendering a result.
func TestResultRender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result *OldResult
		lines  []string
	}{
		{
			name:   "empty",
			result: nil,
			lines:  nil,
		},
		{
			name: "ok result",
			result: &OldResult{
				failed: false,
			},
			lines: nil,
		},
		{
			name: "simple failure",
			result: &OldResult{
				failed: true,
			},
			lines: []string{
				`Unknown Test FAILED`,
			},
		},
		{
			name: "failure with value",
			result: &OldResult{
				failed: true,
				header: resultHeader{
					comparison: "equal",
					types:      []string{"int", "int"},
				},
				values: []value{
					value{
						name:  "left",
						value: 4,
					},
					value{
						name:  "right",
						value: 7,
					},
				},
			},
			lines: []string{
				`equal FAILED`,
				`  left: 4`,
				`  right: 7`,
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Diff(tt.result.Render(), tt.lines); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
