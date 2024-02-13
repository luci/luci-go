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
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

// TestEqual checks whether two things are equal.
func TestEqual(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		lhs  interface{}
		rhs  interface{}
		ok   bool
	}{
		{
			name: "empty",
			lhs:  nil,
			rhs:  nil,
			ok:   true,
		},
		{
			name: "nil vs non-nil",
			lhs:  nil,
			rhs:  7,
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
