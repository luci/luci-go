// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAddress(t *testing.T) {
	t.Parallel()

	ftt.Run("FormatBuildAddress", t, func(c *ftt.Test) {
		c.Run("Number", func(c *ftt.Test) {
			assert.Loosely(c, FormatBuildAddress(1, "luci.chromium.try", "linux-rel", 2), should.Equal("luci.chromium.try/linux-rel/2"))
		})
		c.Run("ID", func(c *ftt.Test) {
			assert.Loosely(c, FormatBuildAddress(1, "luci.chromium.try", "linux-rel", 0), should.Equal("1"))
		})
	})
	ftt.Run("ParseBuildAddress", t, func(c *ftt.Test) {
		c.Run("Number", func(c *ftt.Test) {
			id, project, bucket, builder, number, err := ParseBuildAddress("luci.chromium.try/linux-rel/2")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, id, should.BeZero)
			assert.Loosely(c, project, should.Equal("chromium"))
			assert.Loosely(c, bucket, should.Equal("luci.chromium.try"))
			assert.Loosely(c, builder, should.Equal("linux-rel"))
			assert.Loosely(c, number, should.Equal(2))
		})
		c.Run("ID", func(c *ftt.Test) {
			id, project, bucket, builder, number, err := ParseBuildAddress("1")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, id, should.Equal(1))
			assert.Loosely(c, project, should.BeEmpty)
			assert.Loosely(c, bucket, should.BeEmpty)
			assert.Loosely(c, builder, should.BeEmpty)
			assert.Loosely(c, number, should.BeZero)
		})
		c.Run("Unrecognized", func(c *ftt.Test) {
			_, _, _, _, _, err := ParseBuildAddress("a/b")
			assert.Loosely(c, err, should.NotBeNil)
		})
	})
}
