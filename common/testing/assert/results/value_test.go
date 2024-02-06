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

func TestRender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string // name of the *test*
		valName string // name of the *value*
		value   any
		output  []string
	}{
		{
			name:    "empty",
			valName: "empty",
			output: []string{
				`  empty: <nil>`,
			},
		},
		{
			name:    "integer",
			valName: "a",
			value:   72,
			output: []string{
				`  a: 72`,
			},
		},
		{
			name:    "string",
			valName: "a",
			value:   "b",
			output: []string{
				`  a: "b"`,
			},
		},
		{
			name:    "verbatim string",
			valName: "a",
			value:   verbatimString("zz"),
			output: []string{
				`  a: zz`,
			},
		},
		{
			name:    "newlines",
			valName: "a",
			value:   verbatimString("z\nz"),
			output: []string{
				`  a:`,
				`  | z`,
				`  | z`,
			},
		},
		{
			name:    "newlines",
			valName: "a",
			value:   verbatimString("\n"),
			output: []string{
				`  a: <empty>`,
			},
		},
		{
			name:    "two newlines",
			valName: "a",
			value:   verbatimString("\n\n"),
			output: []string{
				`  a: <empty>`,
			},
		},
		{
			name:    "newline-delimited thing",
			valName: "a",
			value:   verbatimString("a\nb"),
			output: []string{
				`  a:`,
				`  | a`,
				`  | b`,
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := (&value{tt.valName, tt.value}).render()
			if diff := typed.Diff(got, tt.output); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestBlockLines tests that blocklines formats output in roughly the format below:
//
//	@@TITLE
//	@@| line1
//	@@| line2
//
// where `@@` is the prefix.
func TestBlockLines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		prefix string
		title  string
		lines  []string
		output []string
	}{
		{
			name:   "empty",
			prefix: "",
			title:  "",
			lines:  nil,
			output: nil,
		},
		{
			name:   "one line",
			prefix: "@@",
			title:  "TITLE",
			lines: []string{
				`a`,
			},
			output: []string{
				`@@TITLE: a`,
			},
		},
		{
			name:   "two lines",
			prefix: "@@",
			title:  "TITLE",
			lines: []string{
				`A`,
				`B`,
			},
			output: []string{
				`@@TITLE:`,
				`@@| A`,
				`@@| B`,
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := blockLines(tt.prefix, tt.title, tt.lines)

			if diff := typed.Diff(got, tt.output); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
