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

package structuraldiff

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sergi/go-diff/diffmatchpatch"

	"go.chromium.org/luci/common/testing/typed"
)

// TestDebugCompare compares two values and checks to see if the result is what we expect.
//
// This test is sensitive to small changes in how the Result is represented.
func TestDebugCompare(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		left   any
		right  any
		result *Result
	}{
		{
			name:   "equal",
			left:   10,
			right:  10,
			result: nil,
		},
		{
			name:  "different",
			left:  10,
			right: 27,
			result: &Result{
				diffs: []diffmatchpatch.Diff{
					{Type: diffmatchpatch.DiffDelete, Text: "10"},
					{Type: diffmatchpatch.DiffInsert, Text: "27"},
				},
			},
		},
		{
			name:  "different types but similar string representation.",
			left:  10,
			right: "10",
			result: &Result{
				diffs: []diffmatchpatch.Diff{
					{Type: diffmatchpatch.DiffInsert, Text: `"`},
					{Text: "10"},
					{Type: diffmatchpatch.DiffInsert, Text: `"`},
				},
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := DebugCompare[any](tt.left, tt.right)
			want := tt.result

			if diff := typed.Got(got).Want(want).Options(cmp.AllowUnexported(Result{})).Diff(); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestToString tests rendering a result as a string
func TestToString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result *Result
		output string
	}{
		{
			name:   "empty",
			result: nil,
			output: "",
		},
		{
			name:   "simple",
			result: DebugCompare[string]("a", "b"),
			output: strings.Join([]string{
				"Delete a\n",
				"Insert b\n",
			}, ""),
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := typed.Got(tt.result.String()).Want(tt.output).Diff(); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
