// Copyright 2026 The LUCI Authors.
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

package failurereason

import (
	"regexp"
	"testing"
	"unicode/utf8"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// FuzzReasonMaskRuleMatchesExample verifies that for any valid
// ReasonMaskPattern (a regular expression with a single capturing group, as
// enforced by validateReasonMaskPattern) and any non-empty UTF-8 failure reason
// message, FailureAssociationRule produces a rule that parses cleanly via
// lang.Parse and evaluates to true on the example failure that generated it.
//
// Fuzzing both the masking regexp and the failure reason exercises the
// interaction between LIKE-escaping (\, %, _) and regexp submatch masking in
// clusterLike and applyMask, including empty submatches, optional capture
// groups in alternations, and masks that land on LIKE escape boundaries.
func FuzzReasonMaskRuleMatchesExample(f *testing.F) {
	f.Add(`(?:^\[Fixture failure\] )([a-zA-Z0-9_]+):`, `[Fixture failure] my_fix: err`)
	f.Add(`^\[rosetta\] (.*)$`, `[rosetta] crash_1`)
	f.Add(`id=([0-9a-f]+)`, `id=deadbeef`)
	f.Add(`()`, `tf1zZbC}i tF_[i[e+FSl`)
	f.Add(`()`, `_`)
	f.Add(`(a)|b`, `b`)
	f.Add(`(a)?_`, `_`)
	f.Add(`\\(.)`, `\a`)
	// Adjacent captures that meet inside the escape sequence `\%`. The capture
	// of `\` is widened to cover all of `\%`, so the next capture is either
	// skipped (`%`) or trimmed (`%a` to `a`).
	f.Add(`(.)`, `%`)
	f.Add(`(\\|%a)`, `%a`)

	alg := &Algorithm{}

	f.Fuzz(func(t *testing.T, maskPattern string, msg string) {
		if maskPattern == "" || msg == "" || !utf8.ValidString(maskPattern) || !utf8.ValidString(msg) {
			return
		}
		re, err := regexp.Compile(maskPattern)
		if err != nil || re.NumSubexp() != 1 {
			return
		}

		cfg := &compiledcfg.ProjectConfig{
			ReasonMaskPatterns: []*regexp.Regexp{re},
		}
		rule := alg.FailureAssociationRule(cfg, &clustering.Failure{
			Reason: &pb.FailureReason{PrimaryErrorMessage: msg},
		})
		expr, err := lang.Parse(rule)
		if err != nil {
			t.Fatalf("maskPattern=%q msg=%q: lang.Parse(%q) failed: %v", maskPattern, msg, rule, err)
		}
		if !expr.Evaluate(lang.Failure{Reason: msg}) {
			t.Fatalf("maskPattern=%q msg=%q: rule %s did not match example failure", maskPattern, msg, rule)
		}
	})
}
