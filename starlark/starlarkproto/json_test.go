// Copyright 2020 The LUCI Authors.
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

package starlarkproto

import (
	"strings"
	"testing"
)

func TestFormatJSON(t *testing.T) {
	t.Parallel()

	successes := []struct {
		in  string
		out string
	}{
		{"", ""},
		{"\n\t ", ""},
		{"  true\n", "true"},
		{"  false\n", "false"},
		{"123\n\t ", "123"},
		{"123456789101112131400000.15\n\t ", "123456789101112131400000.15"},
		{"-123.123\n\t ", "-123.123"},
		{` "abc\n"`, `"abc\n"`},
		{"null", "null"},
		{"[\n] ", "[]"},
		{"[123]", "[\n\t123\n]"},
		{"[null] ", "[\n\tnull\n]"},
		{
			"[1,2]",
			`[
				1,
				2
			]`,
		},
		{
			"[[1], [], [2]]",
			`[
				[
					1
				],
				[],
				[
					2
				]
			]`,
		},
		{"{  }\n", "{}"},
		{
			`{"a":    null}`,
			`{
				"a": null
			}`,
		},
		{
			`{"a": 1, "b": 2}`,
			`{
				"a": 1,
				"b": 2
			}`,
		},
		{
			`{"a": [{"b": [], "c": {}}], "d": 1}`,
			`{
				"a": [
					{
						"b": [],
						"c": {}
					}
				],
				"d": 1
			}`,
		},
	}

	for _, c := range successes {
		out, err := formatJSON([]byte(c.in))
		if err != nil {
			t.Errorf("when formatting %q - %s", c.in, err)
		} else if expected := deindent(c.out, "\t\t\t"); string(out) != expected {
			t.Errorf("format(%q) == %q != %q", c.in, out, expected)
		}
	}

	fails := []struct {
		in  string
		err string
	}{
		{"what is this", "invalid character"},
		{"[", "unexpected EOF"},
		{"{", "unexpected EOF"},
		{"}", "invalid character"},
		{"[}", "invalid character"},
		{"{]", "invalid character"},
		{"[[}]", "invalid character"},
		{`{"a": [}}`, "invalid character"},
		{`"zzz`, "unexpected EOF"},
		{"{1}", "invalid character"},
		{`{"a"}`, "invalid character '}' after object key"},
		{`{"a", "b"}`, "invalid character ',' after object key"},
	}

	for _, c := range fails {
		_, err := formatJSON(([]byte(c.in)))
		if err == nil {
			t.Errorf("formatting of %q unexpectedly successed", c.in)
		} else if !strings.Contains(err.Error(), c.err) {
			t.Errorf("format(%q) expected to fail with %q but failed as %q", c.in, c.err, err)
		}
	}
}

// deindent deindents hardcoded multiline text string used in the test.
func deindent(s, pfx string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, pfx)
	}
	return strings.Join(lines, "\n")
}
