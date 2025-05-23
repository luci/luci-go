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

	"go.chromium.org/luci/common/testing/internal/typed"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Check that calling NewSummaryBuilder does something remotely reasonable.
func TestNewSummaryBuilderSmokeTest(t *testing.T) {
	t.Parallel()

	res := NewSummaryBuilder("test").Summary
	if diff := typed.Diff(res, &failure.Summary{Comparison: &failure.Comparison{Name: "test"}}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// Check that a SummaryBuilder{} does something remotely reasonable.
func TestSummaryBuilderNil(t *testing.T) {
	t.Parallel()

	res := (&SummaryBuilder{}).Because("reasons").Summary
	if diff := typed.Diff(res, &failure.Summary{Findings: []*failure.Finding{{Name: "Because", Value: []string{"reasons"}}}}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// TestNewSummaryBuilder tests using a SummaryBuilder to build a failure.Summary.
func TestNewSummaryBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		expected *failure.Summary
		actual   *failure.Summary
	}{
		{
			name:     "equal",
			expected: NewSummaryBuilder("equal").Summary,
			actual: &failure.Summary{
				Comparison: &failure.Comparison{
					Name: "equal",
				},
			},
		},
		{
			name:     "equal[int]",
			expected: NewSummaryBuilder("equal", 100).Summary,
			actual: &failure.Summary{
				Comparison: &failure.Comparison{
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

// TestBecause tests setting the because field of a failure.Summary.
func TestBecause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		format  string
		args    []any
		summary *failure.Summary
	}{
		{
			name:   "because",
			format: "%d",
			args:   []any{7},
			summary: &failure.Summary{
				Comparison: &failure.Comparison{Name: "test"},
				Findings: []*failure.Finding{
					{Name: "Because", Value: []string{"7"}},
				},
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expected := tt.summary
			actual := NewSummaryBuilder("test").Because(tt.format, tt.args...).Summary
			if diff := typed.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestActualExpected tests setting the actual/expected fields of a failure.Summary.
func TestActualExpected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		expectedValue any
		actualValue   any
		summary       *failure.Summary
	}{
		{
			name:          "actual/expected",
			expectedValue: 7,
			actualValue:   8,
			summary: &failure.Summary{
				Comparison: &failure.Comparison{Name: "test"},
				Findings: []*failure.Finding{
					{Name: "Expected", Value: []string{"7"}},
					{Name: "Actual", Value: []string{"8"}},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expected := tt.summary
			actual := NewSummaryBuilder("test").Expected(tt.expectedValue).Actual(tt.actualValue).Summary
			if diff := typed.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestAddComparisonArgs(t *testing.T) {
	t.Parallel()

	sb := NewSummaryBuilder("hello")
	if diff := typed.Diff(sb.Summary, &failure.Summary{
		Comparison: &failure.Comparison{
			Name: "hello",
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}

	sb.AddComparisonArgs(1, 2, "hi")
	if diff := typed.Diff(sb.Summary, &failure.Summary{
		Comparison: &failure.Comparison{
			Name:      "hello",
			Arguments: []string{"1", "2", "hi"},
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

func TestGetFailure(t *testing.T) {
	t.Parallel()

	sb := NewSummaryBuilder("hello")
	if diff := typed.Diff(sb.GetFailure(), nil); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}

	sb.AddComparisonArgs(1, 2, "hi")
	if diff := typed.Diff(sb.GetFailure(), nil); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}

	sb.Actual("something")
	if diff := typed.Diff(sb.GetFailure(), &failure.Summary{
		Comparison: &failure.Comparison{
			Name:      "hello",
			Arguments: []string{"1", "2", "hi"},
		},
		Findings: []*failure.Finding{
			{Name: "Actual", Value: []string{`"something"`}},
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

func TestRenameFinding(t *testing.T) {
	sb := NewSummaryBuilder("test")
	sb.Expected("something")
	sb.AddFindingf("Extra", "stuff")

	// Make sure that we have the order Expected, Extra
	if diff := typed.Diff(sb.Summary, &failure.Summary{
		Comparison: &failure.Comparison{Name: "test"},
		Findings: []*failure.Finding{
			{Name: "Expected", Value: []string{`"something"`}},
			{Name: "Extra", Value: []string{"stuff"}},
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}

	// Make sure that we have the order Morples, Extra (renaming doesn't
	// reorder stuff)
	sb.RenameFinding("Expected", "Morples")
	if diff := typed.Diff(sb.Summary, &failure.Summary{
		Comparison: &failure.Comparison{Name: "test"},
		Findings: []*failure.Finding{
			{Name: "Morples", Value: []string{`"something"`}},
			{Name: "Extra", Value: []string{"stuff"}},
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}

	// Make sure that we have the same thing, renaming a finding that doesn't
	// exist is a no-op.
	sb.RenameFinding("NotHere", "Oh No!")
	if diff := typed.Diff(sb.Summary, &failure.Summary{
		Comparison: &failure.Comparison{Name: "test"},
		Findings: []*failure.Finding{
			{Name: "Morples", Value: []string{`"something"`}},
			{Name: "Extra", Value: []string{"stuff"}},
		},
	}); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}
