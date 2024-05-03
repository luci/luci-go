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

package comparison

import (
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

func TestHeredocLines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		prefix   string
		title    string
		input    []string
		expected []string
	}{
		{
			name:   "no lines",
			prefix: "  ",
			title:  "Because",
			expected: []string{
				`  Because`,
			},
		},
		{
			name:   "empty oneline",
			prefix: "  ",
			title:  "Because",
			input: []string{
				``,
			},
			expected: []string{
				`  Because`,
			},
		},
		{
			name:   "oneline",
			prefix: "  ",
			title:  "Because",
			input: []string{
				`meepmorp`,
			},
			expected: []string{
				`  Because: meepmorp`,
			},
		},
		{
			name:   "multiline",
			prefix: "  ",
			title:  "Because",
			input: []string{
				`here`,
				`are`,
				`some`,
				`lines`,
			},
			expected: []string{
				`  Because: <<EOF`,
				`here`,
				`are`,
				`some`,
				`lines`,
				`EOF`,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			actual := heredocLines(tt.prefix, tt.title, tt.input)

			if diff := typed.Diff(tt.expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
