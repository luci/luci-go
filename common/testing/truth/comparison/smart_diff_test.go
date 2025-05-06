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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/internal/typed"
)

func TestSmartCmpDiff(t *testing.T) {
	t.Parallel()

	type caseT struct {
		actualIn   any
		expectedIn any
		extraOpts  []cmp.Option

		expectedFailure *failure.Summary

		// cmp.Diff has unstable output - for any Finding with Type of CmpDiff,
		// assert that it contains all of these strings and then redact the value of
		// that finding.
		diffContains []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()

			got := NewSummaryBuilder("test").
				SmartCmpDiff(tt.actualIn, tt.expectedIn, tt.extraOpts...).
				Summary

			for _, finding := range got.Findings {
				if finding.Type == failure.FindingTypeHint_CmpDiff {
					needles := stringset.NewFromSlice(tt.diffContains...)
					for _, candidate := range finding.Value {
						found := stringset.New(needles.Len())
						needles.Iter(func(needle string) bool {
							if strings.Contains(candidate, needle) {
								found.Add(needle)
							}
							return true
						})
						needles = needles.Difference(found)
						if needles.Len() == 0 {
							break
						}
					}
					if needles.Len() != 0 {
						t.Logf("failed to find the following in the diff typed entry %q", finding.Name)
						needles.Iter(func(needle string) bool {
							t.Log("  ", needle)
							return true
						})
						t.Log("Value was:")
						for _, line := range finding.Value {
							t.Log("  ", line)
						}
						t.FailNow()
					}

					finding.Value = []string{"[REMOVED]"}
				}
			}

			if diff := typed.Diff(tt.expectedFailure, got); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("simple", testCase(caseT{
		actualIn:   10,
		expectedIn: 20,
		expectedFailure: &failure.Summary{
			Comparison: &failure.Comparison{
				Name: "test",
			},
			Findings: []*failure.Finding{
				{Name: "Actual", Value: []string{"10"}},
				{Name: "Expected", Value: []string{"20"}},
			},
		},
	}))

	type myCustomInt int

	t.Run("simple mismatched types", testCase(caseT{
		actualIn:   10,
		expectedIn: myCustomInt(10),
		expectedFailure: &failure.Summary{
			Comparison: &failure.Comparison{
				Name: "test",
			},
			Findings: []*failure.Finding{
				{Name: "Actual", Value: []string{"10"}},
				{Name: "Expected", Value: []string{"10"}},
				{Name: "Diff", Value: []string{"[REMOVED]"}, Type: failure.FindingTypeHint_CmpDiff},
			},
		},
		diffContains: []string{
			"int(10)",
			"comparison.myCustomInt(10)",
		},
	}))

	t.Run("long value", testCase(caseT{
		actualIn:   10,
		expectedIn: strings.Repeat("hi", 16),
		expectedFailure: &failure.Summary{
			Comparison: &failure.Comparison{
				Name: "test",
			},
			Findings: []*failure.Finding{
				{Name: "Actual", Value: []string{"10"}},
				{
					Name:  "Expected",
					Value: []string{`"hihihihihihihihihihihihihihihihi"`},
					Level: failure.FindingLogLevel_Warn,
				},
				{Name: "Diff", Value: []string{"[REMOVED]"}, Type: failure.FindingTypeHint_CmpDiff},
			},
		},
		diffContains: []string{"10", "hihihihihi"},
	}))
}
