// Copyright 2015 The LUCI Authors.
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

package clock

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTags(t *testing.T) {
	ftt.Run(`An empty Context`, t, func(t *ftt.Test) {
		c := context.Background()

		t.Run(`Should have nil tags.`, func(t *ftt.Test) {
			assert.Loosely(t, Tags(c), should.BeNil)
		})

		t.Run(`With tag, "A"`, func(t *ftt.Test) {
			c = Tag(c, "A")

			t.Run(`Should have tags {"A"}.`, func(t *ftt.Test) {
				assert.Loosely(t, Tags(c), should.Resemble([]string{"A"}))
			})

			t.Run(`And another tag, "B"`, func(t *ftt.Test) {
				c = Tag(c, "B")

				t.Run(`Should have tags {"A", "B"}.`, func(t *ftt.Test) {
					assert.Loosely(t, Tags(c), should.Resemble([]string{"A", "B"}))
				})
			})
		})
	})
}
