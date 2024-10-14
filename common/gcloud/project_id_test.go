// Copyright 2021 The LUCI Authors.
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

package gcloud

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateProjectID(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test ValidateProjectID`, t, func(t *ftt.Test) {
		t.Run(`With valid project IDs`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateProjectID(`project-id-foo`), should.BeNil)
		})

		t.Run(`W/o a starting, lowercase ASCII letters`, func(t *ftt.Test) {
			e := `must start with a lowercase ASCII letter`
			assert.Loosely(t, ValidateProjectID(`0123456`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`ProjectID`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`-project-id`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`ö-project-id`), should.ErrLike(e))
		})

		t.Run(`With a trailing hyphen`, func(t *ftt.Test) {
			e := `must not have a trailing hyphen`
			assert.Loosely(t, ValidateProjectID(`project-id-`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`project--id--`), should.ErrLike(e))
		})

		t.Run(`With len() < 6 or > 30`, func(t *ftt.Test) {
			e := `must contain 6 to 30 ASCII letters, digits, or hyphens`
			assert.Loosely(t, ValidateProjectID(``), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`pro`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`project-id-1234567890-1234567890`), should.ErrLike(e))
		})

		t.Run(`With non-ascii letters`, func(t *ftt.Test) {
			e := `invalid letter`
			assert.Loosely(t, ValidateProjectID(`project-✅-id`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`project-✅-👀`), should.ErrLike(e))
			assert.Loosely(t, ValidateProjectID(`project-id-🍀🍀🍀🍀s`), should.ErrLike(e))
		})
	})
}
