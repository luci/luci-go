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

package backend

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	computealpha "google.golang.org/api/compute/v0.alpha"
	compute "google.golang.org/api/compute/v1"
)

// TestGetErrors tests getting the errors out of a combined stable+alpha Operation type.
func TestGetErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  Operation
		output []CommonOpError
	}{
		{
			name:   "empty operation",
			input:  Operation{},
			output: nil,
		},
		{
			name: "stable operation",
			input: Operation{
				Stable: &compute.Operation{
					Error: &compute.OperationError{
						Errors: []*compute.OperationErrorErrors{
							{
								Code:    "hi",
								Message: "bye",
							},
						},
					},
				},
			},
			output: []CommonOpError{
				{
					Code:    "hi",
					Message: "bye",
				},
			},
		},
		{
			name: "alpha operation",
			input: Operation{
				Alpha: &computealpha.Operation{
					Error: &computealpha.OperationError{
						Errors: []*computealpha.OperationErrorErrors{
							{
								Code:    "hi",
								Message: "bye",
							},
						},
					},
				},
			},
			output: []CommonOpError{
				{
					Code:    "hi",
					Message: "bye",
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expected := tt.output
			actual := tt.input.GetErrors()
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got) %s", diff)
			}
		})
	}
}
