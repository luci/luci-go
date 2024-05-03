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

// Check that calling NewFailureBuilder does something remotely reasonable.
func TestNewResultBuilderSmokeTest(t *testing.T) {
	t.Parallel()

	res := NewFailureBuilder("test").Failure
	if diff := typed.Diff(res, &Failure{Comparison: &Failure_ComparisonFunc{Name: "test"}}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// Check that a FailureBuilder{} does something remotely reasonable.
func TestResultBuilderNil(t *testing.T) {
	t.Parallel()

	res := (&FailureBuilder{}).Because("reasons").Failure
	if diff := typed.Diff(res, &Failure{Findings: []*Failure_Finding{{Name: "Because", Value: "reasons"}}}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// TestNewResultBuilder tests using a ResultBuilder to build a failure.
func TestNewResultBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		expected *Failure
		actual   *Failure
	}{
		{
			name:     "equal",
			expected: NewFailureBuilder("equal").Failure,
			actual: &Failure{
				Comparison: &Failure_ComparisonFunc{
					Name: "equal",
				},
			},
		},
		{
			name:     "equal[int]",
			expected: NewFailureBuilder("equal", 100).Failure,
			actual: &Failure{
				Comparison: &Failure_ComparisonFunc{
					Name:          "equal",
					TypeArguments: []string{"int"},
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

// TestBecause tests setting the because field of a failure.
func TestBecause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		format  string
		args    []any
		failure *Failure
	}{
		{
			name:   "because",
			format: "%d",
			args:   []any{7},
			failure: &Failure{
				Comparison: &Failure_ComparisonFunc{Name: "test"},
				Findings: []*Failure_Finding{
					{Name: "Because", Value: "7"},
				},
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expected := tt.failure
			actual := NewFailureBuilder("test").Because(tt.format, tt.args...).Failure
			if diff := typed.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestActualExpected tests setting the actual/expected fields of a failure.
func TestActualExpected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		expectedValue any
		actualValue   any
		failure       *Failure
	}{
		{
			name:          "actual/expected",
			expectedValue: 7,
			actualValue:   8,
			failure: &Failure{
				Comparison: &Failure_ComparisonFunc{Name: "test"},
				Findings: []*Failure_Finding{
					{Name: "Expected", Value: "7"},
					{Name: "Actual", Value: "8"},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expected := tt.failure
			actual := NewFailureBuilder("test").Expected(tt.expectedValue).Actual(tt.actualValue).Failure
			if diff := typed.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
