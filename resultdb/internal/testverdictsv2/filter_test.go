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

package testverdictsv2

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateFilter(t *testing.T) {
	ftt.Run("ValidateFilter", t, func(t *ftt.Test) {
		t.Run("Invalid syntax", func(t *ftt.Test) {
			err := ValidateFilter("status =")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("expected arg after ="))
		})

		t.Run("Invalid column", func(t *ftt.Test) {
			err := ValidateFilter("invalid_column = 1")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike(`no filterable field "invalid_column"`))
		})

		t.Run("Valid", func(t *ftt.Test) {
			err := ValidateFilter("(status_override = NOT_OVERRIDDEN AND (status = FAILED OR status = EXECUTION_ERRORED)) OR status_override = EXONERATED")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
