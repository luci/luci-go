// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package typed

import "testing"

// TestDiff tests whether two things are equal or not.
//
// TestDiff is implemented using cmp.Diff and thus the only part of the API
// that's stable is whether the result of cmp.Diff(...) is an empty string or
// not. cmp.Diff will insert random spaces at the ends of lines to make it
// impossible to test against its output. The reason that they do that,
// though, is to give them the freedom to display diffs more informatively in
// the future without breaking people's tests. That seems reasonable to me.
func TestDiff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		got  string
		ok   bool
	}{
		{
			name: "empty",
			want: "",
			got:  "",
			ok:   true,
		},
		{
			name: "empty",
			want: "a",
			got:  "",
			ok:   false,
		},
	}

	for _, tt := range cases {
		res := (Diff(tt.want, tt.got) == "")
		switch {
		case res && !tt.ok:
			t.Errorf("case %s unexpectedly true", tt.name)
		case !res && tt.ok:
			t.Errorf("case %s unexpectedly false", tt.name)
		}
	}
}

// Test that the got-before-want syntax does what we intend it to.
func TestGot(t *testing.T) {
	t.Parallel()

	diff := Got(4).Want(7).Diff()

	if diff == "" {
		t.Error("expected diff to be empty")
	}
}
