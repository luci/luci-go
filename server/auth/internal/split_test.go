// Copyright 2023 The LUCI Authors.
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

package internal

import (
	"testing"
)

func TestSplitAuthHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in  string
		typ string
		tok string
	}{
		{"Bearer token", "bearer", "token"},
		{"  Bearer     token   ", "bearer", "token"},
		{"Blah  blErg  ", "blah", "blErg"},
		{"bearer Token", "bearer", "Token"},
		{"Bearer token more  ", "bearer", "token more"},
		{"Token  ", "", "Token"},
		{"  Token  ", "", "Token"},
		{"  ", "", ""},
		{"", "", ""},
	}

	for _, c := range cases {
		gotTyp, gotTok := SplitAuthHeader(c.in)
		if gotTyp != c.typ {
			t.Errorf("%q: got type %q, want %q", c.in, gotTyp, c.typ)
		}
		if gotTok != c.tok {
			t.Errorf("%q: got token %q, want %q", c.in, gotTok, c.tok)
		}
	}
}
