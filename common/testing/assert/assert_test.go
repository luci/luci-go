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

	"go.chromium.org/luci/common/testing/assert/results"
	"go.chromium.org/luci/common/testing/assert/testsupport"
	"go.chromium.org/luci/common/testing/typed"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	isEmpty := func(x string) *results.Result {
		if x == "" {
			return nil
		}
		return results.NewResultBuilder().SetName("string not empty").Result()
	}

	cases := []struct {
		name    string
		input   string
		compare results.Comparison[string]
		ok      bool
	}{
		{
			name:    "empty string",
			input:   "",
			compare: isEmpty,
			ok:      true,
		},
		{
			name:    "empty string",
			input:   "a",
			compare: isEmpty,
			ok:      false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testsupport.ZeroTB{}, tt.input, tt.compare)

			if diff := typed.Diff(got, tt.ok); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
