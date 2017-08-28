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

package gaesecrets

import (
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	Convey("gaesecrets.Store works", t, func() {
		c := Use(memory.Use(context.Background()), nil)
		c = caching.WithProcessCache(c, lru.New(0))

		// Autogenerates one.
		s1, err := secrets.GetSecret(c, "key1")
		So(err, ShouldBeNil)
		So(len(s1.Current.ID), ShouldEqual, 8)
		So(len(s1.Current.Blob), ShouldEqual, 32)

		// Returns same one.
		s2, err := secrets.GetSecret(c, "key1")
		So(err, ShouldBeNil)
		So(s2, ShouldResemble, s1)
	})
}
