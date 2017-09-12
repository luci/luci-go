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

package engine

import (
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunTxn(t *testing.T) {
	Convey("With mock context", t, func(C) {
		c := memory.Use(context.Background())
		c = clock.Set(c, testclock.New(epoch))

		Convey("Happy path", func() {
			calls := 0
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				So(inner, ShouldBeNil)
				return nil
			})
			So(err, ShouldBeNil)
			So(calls, ShouldEqual, 1) // one successful attempt

			// Committed.
			job := Job{JobID: "123"}
			So(datastore.Get(c, &job), ShouldBeNil)
			So(job.Revision, ShouldEqual, "abc")
		})

		Convey("Transient error", func() {
			calls := 0
			transient := errors.New("transient error", transient.Tag)
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				So(inner, ShouldBeNil)
				return transient
			})
			So(err, ShouldEqual, transient)
			So(calls, ShouldEqual, defaultTransactionOptions.Attempts) // all attempts

			// Not committed.
			job := Job{JobID: "123"}
			So(datastore.Get(c, &job), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("Fatal error", func() {
			calls := 0
			fatal := errors.New("fatal error")
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				So(inner, ShouldBeNil)
				return fatal
			})
			So(err, ShouldEqual, fatal)
			So(calls, ShouldEqual, 1) // one failed attempt

			// Not committed.
			job := Job{JobID: "123"}
			So(datastore.Get(c, &job), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("Transient error, but marked as abortTransaction", func() {
			calls := 0
			transient := errors.New("transient error", transient.Tag, abortTransaction)
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				So(inner, ShouldBeNil)
				return transient
			})
			So(err, ShouldEqual, transient)
			So(calls, ShouldEqual, 1) // one failed attempt

			// Not committed.
			job := Job{JobID: "123"}
			So(datastore.Get(c, &job), ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
}
