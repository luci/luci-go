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

package cachingtest

import (
	"errors"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		c := context.Background()
		b := BlobCache{LRU: lru.New(0)}

		res, err := b.Get(c, "key")
		So(res, ShouldBeNil)
		So(err, ShouldEqual, caching.ErrCacheMiss)

		So(b.Set(c, "key", []byte("blah"), 0), ShouldBeNil)

		res, err = b.Get(c, "key")
		So(res, ShouldResemble, []byte("blah"))
		So(err, ShouldBeNil)
	})

	Convey("Errors", t, func() {
		fail := errors.New("fail")

		c := context.Background()
		b := BlobCache{LRU: lru.New(0), Err: fail}

		_, err := b.Get(c, "key")
		So(err, ShouldEqual, fail)

		So(b.Set(c, "key", nil, 0), ShouldEqual, fail)
	})
}
