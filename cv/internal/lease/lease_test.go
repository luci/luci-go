// Copyright 2021 The LUCI Authors.
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

package lease

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLease(t *testing.T) {
	t.Parallel()

	Convey("Apply", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		aid := AssetID("foo/1")
		lease := &Lease{
			AssetID: aid,
			Leasee:  "leasee",
			Action:  []byte("This is an action"),
		}
		Convey("Works for new lease", func() {
			now := ct.Clock.Now().UTC()
			err := lease.Apply(ctx, 1*time.Minute)
			So(err, ShouldBeNil)
			So(loadLease(ctx, aid), ShouldResemble, Lease{
				AssetID:        aid,
				Leasee:         "leasee",
				Action:         []byte("This is an action"),
				ExpirationTime: now.Add(1 * time.Minute),
				Number:         1,
			})

			Convey("Returns ErrConflict if existing lease is still active", func() {
				ct.Clock.Add(30 * time.Second) // lease expires after 1 minute
				anotherLease := &Lease{
					AssetID: aid,
					Leasee:  "anotherLeasee",
					Action:  []byte("This is another action"),
				}
				err := anotherLease.Apply(ctx, 1*time.Minute)
				So(err, ShouldErrLike, ErrConflict)
			})

			Convey("Succeed if existing lease has expired", func() {
				ct.Clock.Add(2 * time.Minute) // lease expires after 1 minute
				now := ct.Clock.Now().UTC()
				anotherLease := &Lease{
					AssetID: aid,
					Leasee:  "anotherLeasee",
					Action:  []byte("This is another action"),
				}
				err := anotherLease.Apply(ctx, 1*time.Minute)
				So(err, ShouldBeNil)
				So(loadLease(ctx, aid), ShouldResemble, Lease{
					AssetID:        aid,
					Leasee:         "anotherLeasee",
					Action:         []byte("This is another action"),
					ExpirationTime: now.Add(1 * time.Minute),
					Number:         2,
				})
			})
		})
	})

	Convey("Terminate", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		aid := AssetID("foo/1")
		lease := &Lease{
			AssetID: aid,
			Leasee:  "leasee",
			Action:  []byte("This is an action"),
		}

		Convey("Works", func() {
			So(lease.Apply(ctx, 1*time.Minute), ShouldBeNil)
			Terminate(ctx, aid, 1)
			So(loadLease(ctx, aid), ShouldResemble, Lease{
				AssetID: aid,
				Number:  1,
			})
		})

		Convey("Noop if lease is not current", func() {
			So(lease.Apply(ctx, 1*time.Minute), ShouldBeNil)
			ct.Clock.Add(2 * time.Minute)
			So(lease.Apply(ctx, 1*time.Minute), ShouldBeNil)
			Terminate(ctx, aid, 1)
			So(loadLease(ctx, aid).Number, ShouldEqual, 2)
		})

		Convey("Noop if lease is current but has expired", func() {
			now := ct.Clock.Now().UTC()
			So(lease.Apply(ctx, 1*time.Minute), ShouldBeNil)
			ct.Clock.Add(2 * time.Minute)
			Terminate(ctx, aid, 1)
			So(loadLease(ctx, aid), ShouldResemble, Lease{
				AssetID:        aid,
				Leasee:         "leasee",
				Action:         []byte("This is an action"),
				ExpirationTime: now.Add(1 * time.Minute),
				Number:         1,
			})
		})
	})
}

func loadLease(ctx context.Context, aid AssetID) Lease {
	ret := Lease{AssetID: aid}
	So(datastore.Get(ctx, &ret), ShouldBeNil)
	return ret
}
