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
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/registry"
)

// SmartCmpDiff does a couple things:
//   - It adds "Actual" and "Expected" findings. If they have long renderings,
//     they will be marked as Verbose.
//   - If their text representations are long, or are identical, this will also
//     add a Diff, using cmp.Diff and the provided Options.
//
// "Long" is defined as a Value with multiple lines or which has > 30 characters
// in one line.
//
// if you want to extend the defaults, take a look at:
// - "go.chromium.org/luci/common/testing/registry"
func (fb *FailureBuilder) SmartCmpDiff(actual, expected any, extraCmpOpts ...cmp.Option) *FailureBuilder {
	const lengthThreshold = 30
	isLong := func(f *Failure_Finding) bool {
		// NOTE: f.Value here will be the newline-split %#v formatting of a Go value
		// - these are USUALLY single line... however just in case someone has
		// a custom GoStringer function which returns a value with non-escaped
		// newlines, we consider all multi-line values here to be 'long'.
		return len(f.Value) > 1 || len(f.Value[0]) > lengthThreshold
	}

	fb = fb.Actual(actual).Expected(expected)

	added := fb.Findings[len(fb.Findings)-2:]
	hasLong := false
	for _, finding := range added {
		if isLong(finding) {
			finding.Level = FindingLogLevel_Warn
			hasLong = true
		}
	}

	if hasLong || slices.Equal(added[0].Value, added[1].Value) {
		cmpOpts := registry.GetCmpOptions()
		cmpOpts = append(cmpOpts, extraCmpOpts...)

		splitted := strings.Split(cmp.Diff(actual, expected, cmpOpts...), "\n")
		if splitted[len(splitted)-1] == "" {
			splitted = splitted[:len(splitted)-1]
		}
		fb.Findings = append(fb.Findings, &Failure_Finding{
			Name:  "Diff",
			Value: splitted,
			Type:  FindingTypeHint_CmpDiff,
		})
	}

	return fb
}
