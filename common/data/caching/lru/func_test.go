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

package lru

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCachedFunc(t *testing.T) {
	t.Parallel()

	Convey(`An LRU cache`, t, func() {
		ctx := context.Background()

		cache := New(0)
		getCache := func(context.Context) *Cache { return cache }

		makes := 0
		var makeErr error
		cv := Cached(getCache, "foo", func(context.Context) (interface{}, time.Duration, error) {
			if makeErr != nil {
				return nil, 0, makeErr
			}
			makes++
			return "bar", 0, nil
		})

		Convey(`Can create and retrieve a cached value.`, func() {
			v, err := cv(ctx)
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")
			So(makes, ShouldEqual, 1)

			// Again (cached)
			v, err = cv(ctx)
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")
			So(makes, ShouldEqual, 1)
		})

		Convey(`Will forward an error.`, func() {
			makeErr = errors.New("FAKE ERROR")
			v, err := cv(ctx)
			So(err, ShouldEqual, makeErr)

			v, ok := cache.Get(ctx, "foo")
			So(ok, ShouldBeFalse)

			// Again (success).
			makeErr = nil
			v, err = cv(ctx)
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")
			So(makes, ShouldEqual, 1)
		})
	})
}
