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

package shell

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

// TestQuoteUnix tests the unix quoter.
func TestQuoteUnix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in  []string
		out string
	}{
		{
			in:  []string{``},
			out: `""`,
		},
		{
			in:  []string{`$`},
			out: `"\$"`,
		},
		{
			in:  []string{`a`},
			out: `"a"`,
		},
		{
			in:  []string{`"`},
			out: `"\""`,
		},
		{
			in:  []string{`\`},
			out: `"\\"`,
		},
		{
			in:  []string{"`"},
			out: "\"\\`\"",
		},
		{
			in:  []string{"a", "b"},
			out: `"a" "b"`,
		},
	}

	for _, tt := range cases {
		t.Run(strings.Join(tt.in, " "), func(t *testing.T) {
			t.Parallel()
			expected := tt.out
			actual := QuoteUnix(tt.in...)
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestQuoteUnixUnicode tests the property that the input is valid UTF8 if and only if the output is UTF8 when the input is a single string.
func TestQuoteUnixUnicode(t *testing.T) {
	t.Parallel()
	f := func(in []byte) bool {
		inputIsUTF8 := utf8.Valid(in)
		out := []byte(QuoteUnix(string(in)))
		outputIsUTF8 := utf8.Valid(out)
		return inputIsUTF8 == outputIsUTF8
	}
	if err := quick.Check(f, &quick.Config{
		MaxCount: 10000,
	}); err != nil {
		t.Error(err)
	}
}
