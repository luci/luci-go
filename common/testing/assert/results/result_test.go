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

func TestResultOk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input *Result
		ok    bool
	}{
		{
			name:  "empty",
			input: nil,
			ok:    true,
		},
		{
			name:  "nil",
			input: &Result{},
			ok:    true,
		},
		{
			name: "failed result",
			input: &Result{
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

func TestResultEquals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lhs   *Result
		rhs   *Result
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
			rhs:   &Result{},
			equal: true,
		},
		{
			name: "different headers",
			lhs: &Result{
				failed: true,
				header: resultHeader{
					comparison: "a",
				},
			},
			rhs: &Result{
				failed: true,
				header: resultHeader{
					comparison: "b",
				},
			},
			equal: false,
		},
		{
			name: "same headers",
			lhs: &Result{
				failed: true,
				header: resultHeader{
					comparison: "a",
				},
			},
			rhs: &Result{
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
