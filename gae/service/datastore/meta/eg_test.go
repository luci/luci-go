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
	"errors"
	"testing"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetEntityGroupVersion(t *testing.T) {
	t.Parallel()

	Convey("GetEntityGroupVersion", t, func() {
		c := memory.Use(context.Background())
		c, fb := featureBreaker.FilterRDS(c, errors.New("INTERNAL_ERROR"))

		pm := ds.PropertyMap{
			"$key": ds.MkPropertyNI(ds.MakeKey(c, "A", "")),
			"Val":  ds.MkProperty(10),
		}
		So(ds.Put(c, pm), ShouldBeNil)
		aKey := ds.KeyForObj(c, pm)
		So(aKey, ShouldNotBeNil)

		v, err := GetEntityGroupVersion(c, aKey)
		So(err, ShouldBeNil)
		So(v, ShouldEqual, 1)

		So(ds.Delete(c, aKey), ShouldBeNil)

		v, err = GetEntityGroupVersion(c, ds.NewKey(c, "madeUp", "thing", 0, aKey))
		So(err, ShouldBeNil)
		So(v, ShouldEqual, 2)

		v, err = GetEntityGroupVersion(c, ds.NewKey(c, "madeUp", "thing", 0, nil))
		So(err, ShouldBeNil)
		So(v, ShouldEqual, 0)

		fb.BreakFeatures(nil, "GetMulti")

		v, err = GetEntityGroupVersion(c, aKey)
		So(err.Error(), ShouldContainSubstring, "INTERNAL_ERROR")
	})
}
