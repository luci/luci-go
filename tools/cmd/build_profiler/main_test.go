// Copyright 2022 The LUCI Authors.
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
package main

import "testing"

func TestGetProcessKind(t *testing.T) {
	for _, test := range []struct {
		in   []string
		want string
	}{
		{
			in:   []string{"clang++", "a.c"},
			want: "clang++",
		},
		{
			in:   []string{"python3", "generate_bindings.py"},
			want: "python3 generate_bindings.py",
		},
		{
			in:   []string{"java", "-jar", "compiler.jar"},
			want: "java compiler.jar",
		},
	} {
		if got := getProcessKind(test.in); got != test.want {
			t.Errorf("failed with %s: got %s, want %s", test.in, got, test.want)
		}
	}
}
