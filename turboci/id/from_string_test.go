// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package id

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
)

func ExampleFromString() {
	// A simple workplan ID
	wp, err := FromString("Lmy-workplan")
	if err != nil {
		panic(err)
	}
	fmt.Println(wp.GetWorkPlan().GetId())

	// A check on that workplan
	chk, err := FromString("Lmy-workplan:Cmy-check")
	if err != nil {
		panic(err)
	}
	fmt.Println(chk.GetCheck().GetId())

	// A stage on that workplan
	st, err := FromString("Lmy-workplan:Smy-stage")
	if err != nil {
		panic(err)
	}
	fmt.Println(st.GetStage().GetId())

	// An invalid string
	_, err = FromString("Lmy-workplan:Xwhatever")
	fmt.Println(err)

	// Output:
	// my-workplan
	// my-check
	// my-stage
	// token 1 in "Lmy-workplan:Xwhatever": unexpected key 'X'
}

func TestFromStringErrorConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectedErr string
	}{
		{
			name:        "Empty input string",
			input:       "",
			expectedErr: "",
		},
		{
			name:        "Empty token in middle",
			input:       "Lwp::Cchk",
			expectedErr: `token 1 in "Lwp::Cchk" was empty?`,
		},
		{
			name:        "Empty token at end",
			input:       "Lwp:Cchk:",
			expectedErr: `token 2 in "Lwp:Cchk:" was empty?`,
		},
		{
			name:        "Invalid idx - non-integer",
			input:       "Lwp:Cchk:Rabc",
			expectedErr: `token 2 in "Lwp:Cchk:Rabc": bad idx: strconv.ParseInt: parsing "abc": invalid syntax`,
		},
		{
			name:        "Invalid idx - negative",
			input:       "Lwp:Cchk:R-1",
			expectedErr: `token 2 in "Lwp:Cchk:R-1": bad idx: must be > 0`,
		},
		{
			name:        "Invalid timestamp - malformed",
			input:       "Lwp:Cchk:Vbadts",
			expectedErr: `token 2 in "Lwp:Cchk:Vbadts": bad timestamp: needed seconds/nanos`,
		},
		{
			name:        "Invalid timestamp - non-integer seconds",
			input:       "Lwp:Cchk:Vabc/123",
			expectedErr: `token 2 in "Lwp:Cchk:Vabc/123": bad timestamp: parsing seconds "abc"`,
		},
		{
			name:        "Invalid timestamp - non-integer nanos",
			input:       "Lwp:Cchk:V123/abc",
			expectedErr: `token 2 in "Lwp:Cchk:V123/abc": bad timestamp: parsing nanos "abc"`,
		},
		{
			name:        "Missing required previous ID part - R without C",
			input:       "Lwp:R1",
			expectedErr: `token 1 in "Lwp:R1": expected "Check", got "WorkPlan"`,
		},
		{
			name:        "Unexpected key for current ID type - L after L",
			input:       "Lwp1:Lwp2",
			expectedErr: `token 1 in "Lwp1:Lwp2": unexpected key 'L'`,
		},
		{
			name:        "Unexpected key for current ID type - C after C",
			input:       "Lwp:Cchk1:Cchk2",
			expectedErr: `token 2 in "Lwp:Cchk1:Cchk2": expected "WorkPlan", got "Check"`,
		},
		{
			name:        "WorkPlan with only key, no trimmed part - L",
			input:       "L",
			expectedErr: "",
		},
		{
			name:        "Token with only key, no trimmed part - C",
			input:       "Lwp:C",
			expectedErr: `token 1 in "Lwp:C": unexpected empty token`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FromString(test.input)
			if test.expectedErr == "" {
				assert.NoErr(t, err)
			} else {
				assert.ErrIsLike(t, err, test.expectedErr)
			}
		})
	}
}
