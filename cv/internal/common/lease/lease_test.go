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
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"

	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLease(t *testing.T) {
	t.Parallel()

	Convey("Apply", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(1)))

		rid := ResourceID("foo/1")
		now := clock.Now(ctx).UTC().Truncate(time.Second)
		application := Application{
			ResourceID: rid,
			Holder:     "holder",
			Payload:    []byte("Hey!"),
			ExpireTime: now.Add(1 * time.Minute),
		}
		Convey("Works for new lease", func() {
			actual, err := Apply(ctx, application)
			So(err, ShouldBeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				Payload:    []byte("Hey!"),
				ExpireTime: now.Add(1 * time.Minute),
				Token:      []byte{82, 253, 252, 7, 33, 130, 101, 79},
			}
			So(actual, ShouldResemble, expected)
			So(mustLoadLease(ctx, rid), ShouldResemble, expected)

			Convey("Returns AlreadyInLeaseErr if existing lease is still active", func() {
				ct.Clock.Add(30 * time.Second) // lease expires after 1 minute
				_, err := Apply(ctx, application)
				So(err, ShouldErrLike, &AlreadyInLeaseErr{
					ExpireTime: now.Add(1 * time.Minute), // original lease expiry time
					Holder:     "holder",
					ResourceID: rid,
				})
			})

			Convey("Succeed if existing lease has expired", func() {
				ct.Clock.Add(2 * time.Minute) // lease expires after 1 minute
				now := clock.Now(ctx).UTC().Truncate(time.Second)
				application := Application{
					ResourceID: rid,
					Holder:     "holder2",
					Payload:    []byte("Heyo!"),
					ExpireTime: now.Add(1 * time.Minute),
				}
				actual, err := Apply(ctx, application)
				So(err, ShouldBeNil)
				expected := &Lease{
					ResourceID: rid,
					Holder:     "holder2",
					Payload:    []byte("Heyo!"),
					ExpireTime: now.Add(1 * time.Minute),
					Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				}
				So(actual, ShouldResemble, expected)
				So(mustLoadLease(ctx, rid), ShouldResemble, expected)
			})
		})

		Convey("Truncates to millisecond", func() {
			application.ExpireTime = now.Add(1 * time.Minute).Add(3141593 * time.Nanosecond) // 3.141593 ms
			actual, err := Apply(ctx, application)
			So(err, ShouldBeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				Payload:    []byte("Hey!"),
				ExpireTime: now.Add(1 * time.Minute).Add(3 * time.Millisecond),
				Token:      []byte{82, 253, 252, 7, 33, 130, 101, 79},
			}
			So(actual, ShouldResemble, expected)
		})
	})

	Convey("Terminate", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		rid := ResourceID("foo/1")
		now := clock.Now(ctx).UTC().Truncate(time.Second)
		l, err := Apply(ctx, Application{
			ResourceID: rid,
			Holder:     "holder",
			ExpireTime: now.Add(1 * time.Minute),
		})
		So(err, ShouldBeNil)

		Convey("Works", func() {
			err := l.Terminate(ctx)
			So(err, ShouldBeNil)
			So(mustLoadLease(ctx, rid), ShouldBeNil)

			Convey("No-op if lease is already terminated", func() {
				err := l.Terminate(ctx)
				So(err, ShouldBeNil)
				So(mustLoadLease(ctx, rid), ShouldBeNil)
			})
		})

		Convey("Errors if lease is not current", func() {
			ct.Clock.Add(2 * time.Minute)
			now := clock.Now(ctx).UTC().Truncate(time.Second)
			_, err := Apply(ctx, Application{
				ResourceID: rid,
				Holder:     "holder2",
				ExpireTime: now.Add(1 * time.Minute),
			})
			So(err, ShouldBeNil)
			So(l.Terminate(ctx), ShouldResemble, &AlreadyInLeaseErr{
				ExpireTime: now.Add(1 * time.Minute), // original lease expiry time
				Holder:     "holder2",
				ResourceID: rid,
			})
		})
	})

	Convey("Extend", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(1)))

		rid := ResourceID("foo/1")
		now := clock.Now(ctx).UTC().Truncate(time.Second)
		l, err := Apply(ctx, Application{
			ResourceID: rid,
			Holder:     "holder",
			ExpireTime: now.Add(1 * time.Minute),
			Payload:    []byte("stuff"),
		})
		So(err, ShouldBeNil)

		Convey("Works", func() {
			err := l.Extend(ctx, 1*time.Minute)
			So(err, ShouldBeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				ExpireTime: now.Add(2 * time.Minute),
				Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				Payload:    []byte("stuff"),
			}
			So(l, ShouldResemble, expected)
			So(mustLoadLease(ctx, rid), ShouldResemble, expected)
		})

		Convey("Truncates to millisecond", func() {
			err := l.Extend(ctx, 3141593*time.Nanosecond) // 3.141593 ms
			So(err, ShouldBeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				ExpireTime: now.Add(1 * time.Minute).Add(3 * time.Millisecond),
				Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				Payload:    []byte("stuff"),
			}
			So(l, ShouldResemble, expected)
		})

		Convey("Errors if lease has expired", func() {
			ct.Clock.Add(2 * time.Minute)
			err := l.Extend(ctx, 1*time.Minute)
			So(err, ShouldErrLike, "can't extend an expired lease")
		})

		Convey("Errors if lease doesn't exist in Datastore", func() {
			ct.Clock.Add(30 * time.Second)
			So(l.Terminate(ctx), ShouldBeNil)
			err := l.Extend(ctx, 1*time.Minute)
			So(err, ShouldErrLike, "target lease doesn't exist in datastore")
		})

		Convey("Errors if lease is not current in Datastore", func() {
			ct.Clock.Add(30 * time.Second)
			So(l.Terminate(ctx), ShouldBeNil)
			_, err := Apply(ctx, Application{
				ResourceID: rid,
				Holder:     "holder2",
				ExpireTime: now.UTC().Add(1 * time.Minute),
			})
			So(err, ShouldBeNil)
			err = l.Extend(ctx, 1*time.Minute)
			So(err, ShouldResemble, &AlreadyInLeaseErr{
				ExpireTime: now.Add(1 * time.Minute),
				Holder:     "holder2",
				ResourceID: rid,
			})
		})
	})
}

func mustLoadLease(ctx context.Context, rid ResourceID) *Lease {
	ret, err := Load(ctx, rid)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestRetryIfLeased(t *testing.T) {
	t.Parallel()

	Convey("RetryIfLeased", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		innerDelay, leaseDur := 2*time.Minute, 3*time.Minute
		innerPolicy := func() retry.Iterator {
			return &retry.Limited{
				Delay:   innerDelay,
				Retries: 1,
			}
		}
		expire := clock.Now(ctx).UTC().Truncate(time.Second).Add(leaseDur)
		leaseErr := &AlreadyInLeaseErr{ExpireTime: expire}
		notLeaseErr := errors.New("successful error")

		Convey("with nil inner", func() {
			it := RetryIfLeased(nil)()

			Convey("returns stop, if not AlreadyInLeaseErr", func() {
				So(it.Next(ctx, notLeaseErr), ShouldEqual, retry.Stop)
			})
			Convey("returns the lease expiration, if AlreadyInLeaseErr", func() {
				So(it.Next(ctx, leaseErr).Truncate(time.Second), ShouldEqual, leaseDur)
			})
		})

		Convey("with limited inner", func() {
			it := RetryIfLeased(innerPolicy)()

			Convey("returns inner.next(), if not AlreadyInLeaseErr", func() {
				So(it.Next(ctx, notLeaseErr), ShouldEqual, innerDelay)
				So(it.Next(ctx, notLeaseErr), ShouldEqual, retry.Stop)
			})
			Convey("returns whichever comes earlier", func() {
				Convey("inner.Delay < expiration", func() {
					So(it.Next(ctx, leaseErr), ShouldEqual, innerDelay)
				})
				Convey("inner.Delay > expiration", func() {
					it.(*retryIfLeasedIterator).inner.(*retry.Limited).Delay *= 2
					So(it.Next(ctx, leaseErr), ShouldEqual, leaseDur)
				})
				Convey("inner.Delay > 0 but inner returns stop", func() {
					So(it.Next(ctx, notLeaseErr), ShouldEqual, innerDelay)
					So(it.Next(ctx, leaseErr), ShouldEqual, retry.Stop)
				})
			})
		})
	})
}
