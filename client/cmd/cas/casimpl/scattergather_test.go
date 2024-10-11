// Copyright 2017 The LUCI Authors.
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

package casimpl

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"path/filepath"
	"testing"
)

func TestScatterGatherAdd(t *testing.T) {
	t.Parallel()

	wd1 := filepath.Join("tmp", "go")
	wd2 := filepath.Join("tmp", "stop")
	rp1 := "ha"
	rp2 := filepath.Join("hah", "bah")
	rp3 := filepath.Join("hah", "nah")
	rp3unclean := rp3 + "/"

	ftt.Run(`Test that Add works in a good case.`, t, func(t *ftt.Test) {
		sc := scatterGather{}
		assert.Loosely(t, sc.Add(wd1, rp1), should.BeNil)
		assert.Loosely(t, sc.Add(wd1, rp2), should.BeNil)
		assert.Loosely(t, sc.Add(wd2, rp3unclean), should.BeNil)

		assert.Loosely(t, sc, should.Resemble(scatterGather{
			rp1: wd1,
			rp2: wd1,
			rp3: wd2,
		}))
	})

	ftt.Run(`Test that Add fails in a bad case.`, t, func(t *ftt.Test) {
		sc := scatterGather{}
		assert.Loosely(t, sc.Add(wd1, rp1), should.BeNil)
		assert.Loosely(t, sc.Add(wd1, rp1), should.NotBeNil)
		assert.Loosely(t, sc.Add(wd2, rp1), should.NotBeNil)
		assert.Loosely(t, sc.Add(wd2, rp3), should.BeNil)
		assert.Loosely(t, sc.Add(wd1, rp3unclean), should.NotBeNil)
	})
}

func TestScatterGatherSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Test that Set works in a good case.", t, func(t *ftt.Test) {
		sc := scatterGather{}
		assert.Loosely(t, sc.Set("C:\\windir:dir"), should.BeNil)
		assert.Loosely(t, sc.String(), should.Equal("map[C:\\windir:[dir]]"))
	})
}
