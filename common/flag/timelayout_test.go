// Copyright 2020 The LUCI Authors.
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

package flag

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTimeLayoutFlag(t *testing.T) {
	t.Parallel()

	ftt.Run(`TimeLayoutFlag`, t, func(t *ftt.Test) {
		t.Run(`Works`, func(t *ftt.Test) {
			var ts time.Time
			f := Date(&ts)

			assert.Loosely(t, f.Set("2020-01-24"), should.BeNil)
			assert.Loosely(t, ts, should.Match(time.Date(2020, 1, 24, 0, 0, 0, 0, time.UTC)))

			assert.Loosely(t, f.String(), should.Equal("2020-01-24"))
			assert.Loosely(t, f.Get(), should.Match(ts))
		})

		t.Run(`zero value`, func(t *ftt.Test) {
			var tf timeLayoutFlag
			assert.Loosely(t, tf.String(), should.BeEmpty)
		})
	})
}
