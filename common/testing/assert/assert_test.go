// Copyright 2023 The LUCI Authors.
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
package assert

import (
	"testing"

	"go.chromium.org/luci/common/testing/assert/interfaces"
	"go.chromium.org/luci/common/testing/assert/results"
	"go.chromium.org/luci/common/testing/typed"
)

func isEmptyCmp(x string) *results.OldResult {
	if x == "" {
		return nil
	}
	return results.NewResultBuilder().SetName("string not empty").Result()
}

func TestCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  any
		expect interfaces.MockTB
		ok     bool
	}{
		{
			name:  "empty string",
			input: "",
			ok:    true,
		},
		{
			name:  "non-empty string",
			input: "a",
			expect: interfaces.MockTB{
				HelperCalls: 2,
				LogCalls:    [][]any{{"string not empty FAILED"}},
				FailCalls:   1,
			},
			ok: false,
		},
		{
			name:  "bad type match",
			input: 100,
			expect: interfaces.MockTB{
				HelperCalls: 2,
				LogCalls:    [][]any{{"builtin.LosslessConvertTo FAILED"}},
				FailCalls:   1,
			},
			ok: false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			zt := &interfaces.MockTB{}
			got := Check(zt, tt.input, isEmptyCmp)

			if diff := typed.Diff(zt, &tt.expect); diff != "" {
				t.Errorf("unexpected diff in TB calls (-want +got): %s", diff)
			}
			if diff := typed.Diff(got, tt.ok); diff != "" {
				t.Errorf("unexpected diff in Check return value (-want +got): %s", diff)
			}
		})
	}
}

func TestAssert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  string
		expect interfaces.MockTB
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "non-empty string",
			input: "a",
			expect: interfaces.MockTB{
				HelperCalls:  2,
				LogCalls:     [][]any{{"string not empty FAILED"}},
				FailNowCalls: 1,
			},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			zt := &interfaces.MockTB{}
			Assert(zt, tt.input, isEmptyCmp)

			if diff := typed.Diff(zt, &tt.expect); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
