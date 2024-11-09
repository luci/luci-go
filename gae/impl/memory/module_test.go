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

package memory

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/module"
)

func TestModule(t *testing.T) {
	ftt.Run("NumInstances", t, func(t *ftt.Test) {
		c := Use(context.Background())

		i, err := module.NumInstances(c, "foo", "bar")
		assert.Loosely(t, i, should.Equal(1))
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, module.SetNumInstances(c, "foo", "bar", 42), should.BeNil)
		i, err = module.NumInstances(c, "foo", "bar")
		assert.Loosely(t, i, should.Equal(42))
		assert.Loosely(t, err, should.BeNil)

		i, err = module.NumInstances(c, "foo", "baz")
		assert.Loosely(t, i, should.Equal(1))
		assert.Loosely(t, err, should.BeNil)
	})
}
