// Copyright 2023 The LUCI Authors.
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

package pbutil

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestBaselineID(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateBaselineID`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateBaselineID("try:linux-rel"), should.BeNil)
			assert.Loosely(t, ValidateBaselineID("try:linux asan"), should.BeNil)
		})

		t.Run(`Invalid`, func(t *ftt.Test) {
			t.Run(`Empty`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateBaselineID(""), should.ErrLike(`unspecified`))
			})
			t.Run(`Unsupported Symbol`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateBaselineID("try/linux-rel"), should.ErrLike(`does not match`))
				assert.Loosely(t, ValidateBaselineID("try :rel"), should.ErrLike(`does not match`))
			})
		})
	})
}

func TestBaselineName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ParseBaselineName`, t, func(t *ftt.Test) {
		t.Run(`Parse`, func(t *ftt.Test) {
			proj, baseline, err := ParseBaselineName("projects/foo/baselines/bar:baz")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, proj, should.Equal("foo"))
			assert.Loosely(t, baseline, should.Equal("bar:baz"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, _, err := ParseBaselineName("projects/-/baselines/b!z")
			assert.Loosely(t, err, should.ErrLike(`does not match`))
		})
	})
}
