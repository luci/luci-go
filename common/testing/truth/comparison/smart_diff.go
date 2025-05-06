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

	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/internal/typed"
)

// AddCmpDiff adds a 'Diff' finding which is type hinted to be the output of
// cmp.Diff.
//
// The diff is split into multiple lines, but is otherwise untouched.
func (sb *SummaryBuilder) AddCmpDiff(diff string) *SummaryBuilder {
	sb.fixNilFailure()
	sb.Findings = append(sb.Findings, &failure.Finding{
		Name:  "Diff",
		Value: strings.Split(diff, "\n"),
		Type:  failure.FindingTypeHint_CmpDiff,
	})
	return sb
}

// SmartCmpDiff does a couple things:
//   - It adds "Actual" and "Expected" findings. If they have long renderings,
//     they will be marked as Level=Warn.
//   - If either text representation is long, or they are identical, this will
//     also add a Diff, using cmp.Diff and the provided Options.
//
// "Long" is defined as a Value with multiple lines or which has > 30 characters
// in one line.
//
// The default cmp.Options include a Transformer to handle protobufs. If you
// want to extend the default Options see
// `go.chromium.org/luci/common/testing/registry`.
func (sb *SummaryBuilder) SmartCmpDiff(actual, expected any, extraCmpOpts ...cmp.Option) *SummaryBuilder {
	sb.fixNilFailure()

	sb = sb.Actual(actual).WarnIfLong().
		Expected(expected).WarnIfLong()

	added := sb.Findings[len(sb.Findings)-2:]
	hasLong := false
	for _, finding := range added {
		if finding.Level == failure.FindingLogLevel_Warn {
			hasLong = true
			break
		}
	}

	if hasLong || slices.Equal(added[0].Value, added[1].Value) {
		diff, ok := typed.DiffSafe(actual, expected, extraCmpOpts...)
		if !ok {
			sb.AddFindingf("typed.Diff error", "%s", diff)
		} else {
			sb.AddCmpDiff(diff)
		}
	}

	return sb
}
