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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestLease(t *testing.T) {
	t.Parallel()

	ftt.Run("Apply", t, func(t *ftt.Test) {
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
		t.Run("Works for new lease", func(t *ftt.Test) {
			actual, err := Apply(ctx, application)
			assert.Loosely(t, err, should.BeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				Payload:    []byte("Hey!"),
				ExpireTime: now.Add(1 * time.Minute),
				Token:      []byte{82, 253, 252, 7, 33, 130, 101, 79},
			}
			assert.Loosely(t, actual, should.Resemble(expected))
			assert.Loosely(t, mustLoadLease(ctx, rid), should.Resemble(expected))

			t.Run("Returns AlreadyInLeaseErr if existing lease is still active", func(t *ftt.Test) {
				ct.Clock.Add(30 * time.Second) // lease expires after 1 minute
				_, err := Apply(ctx, application)
				assert.Loosely(t, err, should.Match(&AlreadyInLeaseErr{
					ExpireTime: now.Add(1 * time.Minute), // original lease expiry time
					Holder:     "holder",
					ResourceID: rid,
				}))
			})

			t.Run("Succeed if existing lease has expired", func(t *ftt.Test) {
				ct.Clock.Add(2 * time.Minute) // lease expires after 1 minute
				now := clock.Now(ctx).UTC().Truncate(time.Second)
				application := Application{
					ResourceID: rid,
					Holder:     "holder2",
					Payload:    []byte("Heyo!"),
					ExpireTime: now.Add(1 * time.Minute),
				}
				actual, err := Apply(ctx, application)
				assert.Loosely(t, err, should.BeNil)
				expected := &Lease{
					ResourceID: rid,
					Holder:     "holder2",
					Payload:    []byte("Heyo!"),
					ExpireTime: now.Add(1 * time.Minute),
					Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				}
				assert.Loosely(t, actual, should.Resemble(expected))
				assert.Loosely(t, mustLoadLease(ctx, rid), should.Resemble(expected))
			})
		})

		t.Run("Truncates to millisecond", func(t *ftt.Test) {
			application.ExpireTime = now.Add(1 * time.Minute).Add(3141593 * time.Nanosecond) // 3.141593 ms
			actual, err := Apply(ctx, application)
			assert.Loosely(t, err, should.BeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				Payload:    []byte("Hey!"),
				ExpireTime: now.Add(1 * time.Minute).Add(3 * time.Millisecond),
				Token:      []byte{82, 253, 252, 7, 33, 130, 101, 79},
			}
			assert.Loosely(t, actual, should.Resemble(expected))
		})
	})

	ftt.Run("Terminate", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		rid := ResourceID("foo/1")
		now := clock.Now(ctx).UTC().Truncate(time.Second)
		l, err := Apply(ctx, Application{
			ResourceID: rid,
			Holder:     "holder",
			ExpireTime: now.Add(1 * time.Minute),
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("Works", func(t *ftt.Test) {
			err := l.Terminate(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, mustLoadLease(ctx, rid), should.BeNil)

			t.Run("No-op if lease is already terminated", func(t *ftt.Test) {
				err := l.Terminate(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mustLoadLease(ctx, rid), should.BeNil)
			})
		})

		t.Run("Errors if lease is not current", func(t *ftt.Test) {
			ct.Clock.Add(2 * time.Minute)
			now := clock.Now(ctx).UTC().Truncate(time.Second)
			_, err := Apply(ctx, Application{
				ResourceID: rid,
				Holder:     "holder2",
				ExpireTime: now.Add(1 * time.Minute),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Terminate(ctx), should.Match(&AlreadyInLeaseErr{
				ExpireTime: now.Add(1 * time.Minute), // original lease expiry time
				Holder:     "holder2",
				ResourceID: rid,
			}))
		})
	})

	ftt.Run("Extend", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		t.Run("Works", func(t *ftt.Test) {
			err := l.Extend(ctx, 1*time.Minute)
			assert.Loosely(t, err, should.BeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				ExpireTime: now.Add(2 * time.Minute),
				Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				Payload:    []byte("stuff"),
			}
			assert.Loosely(t, l, should.Resemble(expected))
			assert.Loosely(t, mustLoadLease(ctx, rid), should.Resemble(expected))
		})

		t.Run("Truncates to millisecond", func(t *ftt.Test) {
			err := l.Extend(ctx, 3141593*time.Nanosecond) // 3.141593 ms
			assert.Loosely(t, err, should.BeNil)
			expected := &Lease{
				ResourceID: rid,
				Holder:     "holder",
				ExpireTime: now.Add(1 * time.Minute).Add(3 * time.Millisecond),
				Token:      []byte{22, 63, 95, 15, 154, 98, 29, 114},
				Payload:    []byte("stuff"),
			}
			assert.Loosely(t, l, should.Resemble(expected))
		})

		t.Run("Errors if lease has expired", func(t *ftt.Test) {
			ct.Clock.Add(2 * time.Minute)
			err := l.Extend(ctx, 1*time.Minute)
			assert.Loosely(t, err, should.ErrLike("can't extend an expired lease"))
		})

		t.Run("Errors if lease doesn't exist in Datastore", func(t *ftt.Test) {
			ct.Clock.Add(30 * time.Second)
			assert.Loosely(t, l.Terminate(ctx), should.BeNil)
			err := l.Extend(ctx, 1*time.Minute)
			assert.Loosely(t, err, should.ErrLike("target lease doesn't exist in datastore"))
		})

		t.Run("Errors if lease is not current in Datastore", func(t *ftt.Test) {
			ct.Clock.Add(30 * time.Second)
			assert.Loosely(t, l.Terminate(ctx), should.BeNil)
			_, err := Apply(ctx, Application{
				ResourceID: rid,
				Holder:     "holder2",
				ExpireTime: now.UTC().Add(1 * time.Minute),
			})
			assert.Loosely(t, err, should.BeNil)
			err = l.Extend(ctx, 1*time.Minute)
			assert.Loosely(t, err, should.Match(&AlreadyInLeaseErr{
				ExpireTime: now.Add(1 * time.Minute),
				Holder:     "holder2",
				ResourceID: rid,
			}))
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

	ftt.Run("RetryIfLeased", t, func(t *ftt.Test) {
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

		t.Run("with nil inner", func(t *ftt.Test) {
			it := RetryIfLeased(nil)()

			t.Run("returns stop, if not AlreadyInLeaseErr", func(t *ftt.Test) {
				assert.Loosely(t, it.Next(ctx, notLeaseErr), should.Equal(retry.Stop))
			})
			t.Run("returns the lease expiration, if AlreadyInLeaseErr", func(t *ftt.Test) {
				assert.Loosely(t, it.Next(ctx, leaseErr).Truncate(time.Second), should.Equal(leaseDur))
			})
		})

		t.Run("with limited inner", func(t *ftt.Test) {
			it := RetryIfLeased(innerPolicy)()

			t.Run("returns inner.next(), if not AlreadyInLeaseErr", func(t *ftt.Test) {
				assert.Loosely(t, it.Next(ctx, notLeaseErr), should.Equal(innerDelay))
				assert.Loosely(t, it.Next(ctx, notLeaseErr), should.Equal(retry.Stop))
			})
			t.Run("returns whichever comes earlier", func(t *ftt.Test) {
				t.Run("inner.Delay < expiration", func(t *ftt.Test) {
					assert.Loosely(t, it.Next(ctx, leaseErr), should.Equal(innerDelay))
				})
				t.Run("inner.Delay > expiration", func(t *ftt.Test) {
					it.(*retryIfLeasedIterator).inner.(*retry.Limited).Delay *= 2
					assert.Loosely(t, it.Next(ctx, leaseErr), should.Equal(leaseDur))
				})
				t.Run("inner.Delay > 0 but inner returns stop", func(t *ftt.Test) {
					assert.Loosely(t, it.Next(ctx, notLeaseErr), should.Equal(innerDelay))
					assert.Loosely(t, it.Next(ctx, leaseErr), should.Equal(retry.Stop))
				})
			})
		})
	})
}
