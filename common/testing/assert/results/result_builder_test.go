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
	"reflect"
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

// Check that calling NewResultBuilder does something remotely reasonable.
func TestNewResultBuilderSmokeTest(t *testing.T) {
	t.Parallel()

	res := NewResultBuilder().Result()
	if diff := typed.Diff(res, nil); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// TestNewResultBuilder tests using a ResultBuilder to build a result.
func TestNewResultBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		expected *Result
		actual   *Result
	}{
		{
			name:     "equal",
			expected: NewResultBuilder().SetName("equal").Result(),
			actual: &Result{
				failed: true,
				header: resultHeader{
					comparison: "equal",
				},
			},
		},
		{
			name:     "equal[int]",
			expected: NewResultBuilder().SetName("equal", reflect.TypeOf(0)).Result(),
			actual: &Result{
				failed: true,
				header: resultHeader{
					comparison: "equal",
					types:      []string{"int"},
				},
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := typed.Diff(tt.expected, tt.actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
