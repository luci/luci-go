// Copyright 2022 The LUCI Authors.
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

package pagination

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPageSizeLimiter(t *testing.T) {
	t.Parallel()

	ftt.Run(`PageSizeLimiter`, t, func(t *ftt.Test) {
		psl := PageSizeLimiter{
			Max:     1000,
			Default: 10,
		}

		t.Run(`Adjust works`, func(t *ftt.Test) {
			assert.Loosely(t, psl.Adjust(0), should.Equal(10))
			assert.Loosely(t, psl.Adjust(10000), should.Equal(1000))
			assert.Loosely(t, psl.Adjust(500), should.Equal(500))
			assert.Loosely(t, psl.Adjust(5), should.Equal(5))
		})
	})
}

func TestValidatePageSize(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidatePageSize`, t, func(t *ftt.Test) {
		t.Run(`Positive`, func(t *ftt.Test) {
			assert.Loosely(t, ValidatePageSize(10), should.BeNil)
		})
		t.Run(`Zero`, func(t *ftt.Test) {
			assert.Loosely(t, ValidatePageSize(0), should.BeNil)
		})
		t.Run(`Negative`, func(t *ftt.Test) {
			assert.Loosely(t, ValidatePageSize(-10), should.ErrLike("negative"))
		})
	})
}
