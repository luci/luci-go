// Copyright 2020 The LUCI Authors.
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

package txndefer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleFilterRDS() {
	ctx := FilterRDS(memory.Use(context.Background()))

	datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		Defer(ctx, func(context.Context) { fmt.Println("1") })
		Defer(ctx, func(context.Context) { fmt.Println("2") })
		return nil
	}, nil)

	// Output:
	// 2
	// 1
}

func TestFilter(t *testing.T) {
	t.Parallel()

	Convey("With filter", t, func() {
		ctx := FilterRDS(memory.Use(context.Background()))

		Convey("Successful txn", func() {
			ctx := context.WithValue(ctx, "123", "random extra value")
			called := false

			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				Defer(ctx, func(ctx context.Context) {
					So(datastore.CurrentTransaction(ctx), ShouldBeNil)
					So(ctx.Value("123"), ShouldEqual, "random extra value")
					called = true
				})
				return nil
			}, nil)

			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
		})

		Convey("Fatal txn error", func() {
			called := false

			datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				Defer(ctx, func(context.Context) { called = true })
				return errors.New("boom")
			}, nil)

			So(called, ShouldBeFalse)
		})

		Convey("Txn retries", func() {
			attempt := 0
			calls := 0

			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				attempt++
				Defer(ctx, func(context.Context) { calls++ })
				if attempt < 3 {
					return datastore.ErrConcurrentTransaction
				}
				return nil
			}, nil)

			So(err, ShouldBeNil)
			So(attempt, ShouldEqual, 3)
			So(calls, ShouldEqual, 1)
		})
	})
}
