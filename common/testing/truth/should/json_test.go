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

package should

import (
	"regexp"
	"testing"

	"go.chromium.org/luci/common/testing/internal/typed"
)

func TestEqualJSON_Equal(t *testing.T) {
	t.Parallel()

	res := EqualJSON([]byte(`{"a": 4}`))([]byte(`{"a":    4}`))
	if res != nil {
		t.Errorf("unexpected result: %s", res.String())
	}
}

func TestEqualJSON_Unequal(t *testing.T) {
	t.Parallel()

	res := EqualJSON([]byte(`{"a": 4}`))([]byte(`{"a": 3}`))
	if res == nil {
		t.Error(`EqualJSON should have produced a nil result but didn't`)
	}
	if diff := typed.Got(res.GetComparison().GetName()).Want(`should.EqualJSON`).Diff(); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

func TestCanonicalizeInput(t *testing.T) {
	t.Parallel()

	input := `[1, 2, 3, 4, 5]`
	output := `[
  1,
  2,
  3,
  4,
  5
]`

	actual, err := canonicalizeJSON([]byte(input))
	if err != nil {
		t.Error(err)
	}

	if diff := typed.Got(string(actual)).Want(output).Diff(); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

// TestEqualJSON_GoodIndentation tests that the indentation is good.
//
// I don't like this test. Really, we should have a good stable way of
// rendering a failure.Summary as a string that resembles what we do in
// the command line, but is, you know, stable.
//
// That way we can actually write tests that check to see if the output
// of a failed test is reasonable to a human reader.
//
// In the absence of a way to do that, enjoy this subpar test.
func TestEqualJSON_GoodIndentation(t *testing.T) {
	t.Parallel()

	lhs := `[1, 2, 3, 4, 5, 6, 7, 8, 9]`
	rhs := `[11, 2, 3, 4, 5, 6, 7, 8, 9]`

	res := EqualJSON([]byte(lhs))([]byte(rhs))

	diffFinding := res.GetFindings()[2]
	// Check that we got the right finding by looking at its name.
	if diff := typed.Got(diffFinding.GetName()).Want("Diff").Diff(); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
	// Check that we're removing a line containing "11," by itself in the
	// output, preceded by some whitespace. In the cmp.Diff library, which
	// we are ultimately using through at least one wrapper, is using a
	// random combination of tabs and non-breaking spaces and other
	// whitespace as its strategy for defending itself against people
	// unit-testing its output.
	finding := diffFinding.GetValue()[3]
	// This regexp is the most reliable thing I found to check that the
	// line looks right. \u00a0 is a non-breaking space. \t is a tab, but
	// escaped at the string literal level, not at the regexp library
	// level. \s does not appear to match \u00a0 and I don't want to use
	// some \p{...} thing which is even more obscure.
	expectedPattern := regexp.MustCompile("^-[ \t\u00a0]*11,$")
	if !expectedPattern.MatchString(finding) {
		t.Errorf("bad line: %q", finding)
	}
}
