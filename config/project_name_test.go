// Copyright 2016 The LUCI Authors.
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

package config

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing valid project names`, t, func(t *ftt.Test) {
		for _, testCase := range []string{
			"a",
			"foo_bar-baz-059",
		} {
			t.Run(fmt.Sprintf(`Project name %q is valid`, testCase), func(t *ftt.Test) {
				assert.Loosely(t, ValidateProjectName(testCase), should.BeNil)
			})
		}
	})

	ftt.Run(`Testing invalid project names`, t, func(t *ftt.Test) {
		for _, testCase := range []struct {
			v          string
			errorsLike string
		}{
			{"", "cannot have empty name"},
			{"foo/bar", "invalid character"},
			{"_name", "must begin with a letter"},
			{"1eet", "must begin with a letter"},
		} {
			t.Run(fmt.Sprintf(`Project name %q fails with error %q`, testCase.v, testCase.errorsLike), func(t *ftt.Test) {
				assert.Loosely(t, ValidateProjectName(testCase.v), should.ErrLike(testCase.errorsLike))
			})
		}
	})
}
