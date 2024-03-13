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

	"go.chromium.org/luci/common/testing/typed"
)

// TestEqualInt checks whether two ints are equal.
func TestEqualInt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		lhs  int
		rhs  int
		ok   bool
	}{
		{
			name: "not equal",
			lhs:  9,
			rhs:  8,
			ok:   false,
		},
		{
			name: "equal",
			lhs:  7,
			rhs:  7,
			ok:   true,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Equal(tt.lhs)(tt.rhs) == nil
			want := tt.ok
			if diff := typed.Got(got).Want(want).Diff(); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestEqualFloat checks whether two ints are equal.
func TestEqualFloat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		lhs  float64
		rhs  float64
		ok   bool
	}{
		{
			name: "NaNs are equal",
			lhs:  math.NaN(),
			rhs:  math.NaN(),
			ok:   true,
		},
		{
			name: "NaN isn't equal to any non-NaN value",
			lhs:  math.NaN(),
			rhs:  7.0,
			ok:   false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Equal(tt.lhs)(tt.rhs) == nil
			want := tt.ok
			if diff := typed.Got(got).Want(want).Diff(); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
