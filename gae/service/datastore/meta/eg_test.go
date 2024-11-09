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

package meta

import (
	"context"
	"errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
)

func TestGetEntityGroupVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("GetEntityGroupVersion", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c, fb := featureBreaker.FilterRDS(c, errors.New("INTERNAL_ERROR"))

		pm := ds.PropertyMap{
			"$key": ds.MkPropertyNI(ds.MakeKey(c, "A", "")),
			"Val":  ds.MkProperty(10),
		}
		assert.Loosely(t, ds.Put(c, pm), should.BeNil)
		aKey := ds.KeyForObj(c, pm)
		assert.Loosely(t, aKey, should.NotBeNil)

		v, err := GetEntityGroupVersion(c, aKey)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, v, should.Equal(1))

		assert.Loosely(t, ds.Delete(c, aKey), should.BeNil)

		v, err = GetEntityGroupVersion(c, ds.NewKey(c, "madeUp", "thing", 0, aKey))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, v, should.Equal(2))

		v, err = GetEntityGroupVersion(c, ds.NewKey(c, "madeUp", "thing", 0, nil))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, v, should.BeZero)

		fb.BreakFeatures(nil, "GetMulti")

		v, err = GetEntityGroupVersion(c, aKey)
		assert.Loosely(t, err.Error(), should.ContainSubstring("INTERNAL_ERROR"))
	})
}
