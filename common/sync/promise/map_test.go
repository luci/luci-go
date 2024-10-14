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

package promise

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMap(t *testing.T) {
	t.Parallel()

	ftt.Run(`Works`, t, func(t *ftt.Test) {
		c := context.Background()
		m := Map[int, string]{}

		p1 := m.Get(c, 1, func(context.Context) (string, error) {
			return "hello", nil
		})
		p2 := m.Get(c, 1, func(context.Context) (string, error) {
			panic("must not be called")
		})
		assert.Loosely(t, p1, should.Equal(p2))

		res, err := p1.Get(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.Equal("hello"))
	})
}
