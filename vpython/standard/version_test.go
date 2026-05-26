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

package standard

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestVersionParsingAndMatching(tT *testing.T) {
	tT.Parallel()

	ftt.Run("Version Parsing", tT, func(t *ftt.Test) {
		t.Run("Parses exact single, double, and triple versions", func(t *ftt.Test) {
			v, err := ParseVersion("3")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Match(Version{Major: 3, Minor: 0, Patch: 0, Segments: 1, Raw: "3"}))

			v, err = ParseVersion("3.11")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Match(Version{Major: 3, Minor: 11, Patch: 0, Segments: 2, Raw: "3.11"}))

			v, err = ParseVersion("v3.8.10")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Match(Version{Major: 3, Minor: 8, Patch: 10, Segments: 3, Raw: "3.8.10"}))

			// Verify trailing build/prerelease suffixes are safely allowed and stripped
			v, err = ParseVersion("3.8.10.chromium.31")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Match(Version{Major: 3, Minor: 8, Patch: 10, Segments: 3, Raw: "3.8.10.chromium.31"}))

			v, err = ParseVersion("3.11-rc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Match(Version{Major: 3, Minor: 11, Patch: 0, Segments: 2, Raw: "3.11-rc"}))
		})

		t.Run("Fails on invalid formats", func(t *ftt.Test) {
			_, err := ParseVersion("abc")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseVersion("3..5")
			assert.Loosely(t, err, should.NotBeNil) // Double dots are blocked!
		})
	})

	ftt.Run("Interpreter Selection Rules", tT, func(t *ftt.Test) {
		available := []string{"3.8.10", "3.11.2", "3.11.8"}

		t.Run("Selects highest available when no constraint is provided", func(t *ftt.Test) {
			matched, err := MatchInterpreter("", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))
		})

		t.Run("Selects correct versions under strict exact match rules (==)", func(t *ftt.Test) {
			// Two-segment exact match acts as a prefix-wildcard in PEP 440 (allows patch variations)
			matched, err := MatchInterpreter("==3.8", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.8.10"))

			// Exact match wildcard "==3.11" matches both 3.11.2 and 3.11.8, selecting the highest!
			matched, err = MatchInterpreter("==3.11", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))

			// Three-segment exact match strictly requires a patch version match
			_, err = MatchInterpreter("==3.8.0", available)
			assert.Loosely(t, err, should.NotBeNil) // Mismatch (available is 3.8.10, not 3.8.0!)
		})

		t.Run("Protects against patch down-grading errors (>=)", func(t *ftt.Test) {
			// >=3.11.4 should correctly skip 3.11.2 and select 3.11.8!
			matched, err := MatchInterpreter(">=3.11.4", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))

			// If the requested patch is higher than all available, throw an error
			_, err = MatchInterpreter(">=3.11.9", available)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Enforces correct PEP 440 compatible release boundaries (~=)", func(t *ftt.Test) {
			// ~=3.8.0 allows >=3.8.0, <3.9.0 (excl. 3.11!)
			matched, err := MatchInterpreter("~=3.8.0", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.8.10"))

			// ~=3.8 allows >=3.8, <4.0 (incl. 3.11, selecting highest candidate)
			matched, err = MatchInterpreter("~=3.8", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))
		})

		t.Run("Fails with error when no matching targets exist", func(t *ftt.Test) {
			_, err := MatchInterpreter(">=3.12", available)
			assert.Loosely(t, err, should.NotBeNil)

			_, err = MatchInterpreter("==3.9.0", available)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Fails with error on malformed constraint syntax", func(t *ftt.Test) {
			_, err := MatchInterpreter(">=abc", available)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Fails with error when the list of available interpreters is empty", func(t *ftt.Test) {
			_, err := MatchInterpreter("", []string{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Fails with error when the available list only contains invalid versions", func(t *ftt.Test) {
			_, err := MatchInterpreter("", []string{"abc", "def"})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Evaluates boundary operators like <= and >", func(t *ftt.Test) {
			// <= operator: select highest version <= 3.11.2 (picks 3.11.2, skips 3.11.8)
			matched, err := MatchInterpreter("<=3.11.2", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.2"))

			// > operator: select highest version > 3.11.2 (picks 3.11.8)
			matched, err = MatchInterpreter(">3.11.2", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))
		})

		t.Run("Evaluates inequality operators like !=", func(t *ftt.Test) {
			// Two-segment inequality: acts as a wildcard exclusion (excludes all 3.11.x, selects 3.8.10)
			matched, err := MatchInterpreter("!=3.11", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.8.10"))

			// Three-segment inequality: strict patch exclusion (excludes only 3.11.2, selects highest 3.11.8!)
			matched, err = MatchInterpreter("!=3.11.2", available)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.11.8"))
		})

		t.Run("Evaluates arbitrary equality operators like ===", func(t *ftt.Test) {
			customAvailable := []string{"3.8.10.chromium.31", "3.8.10"}

			// === requires an exact, character-for-character match (after trimming spaces/leading 'v')
			matched, err := MatchInterpreter("===3.8.10.chromium.31", customAvailable)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.8.10.chromium.31"))

			matched, err = MatchInterpreter("===3.8.10", customAvailable)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.Equal("3.8.10"))

			// If no target string matches exactly, throw error
			_, err = MatchInterpreter("===3.8.10.chromium.32", customAvailable)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Handles unrecognized operators safely by returning false (Default Case)", func(t *ftt.Test) {
			// Manually construct a private subConstraint with an invalid operator
			invalidSub := subConstraint{
				op:  "invalid_op",
				ver: Version{Major: 3, Minor: 11, Patch: 0, Segments: 2},
			}
			satisfied := invalidSub.isSatisfied(Version{Major: 3, Minor: 11, Patch: 0, Segments: 2})
			assert.Loosely(t, satisfied, should.BeFalse)
		})
	})
}
