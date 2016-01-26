// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ackbuffer

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type testACK struct {
	sync.Mutex

	err       error
	acks      stringset.Set
	batchSize int
}

func (ps *testACK) Ack(c context.Context, acks ...string) error {
	ps.Lock()
	defer ps.Unlock()

	if ps.err != nil {
		return ps.err
	}

	if ps.acks == nil {
		ps.acks = stringset.New(0)
	}
	for _, ack := range acks {
		ps.acks.Add(ack)
	}
	return nil
}

func (ps *testACK) AckBatchSize() int {
	size := ps.batchSize
	if size <= 0 {
		size = 4
	}
	return size
}

func (ps *testACK) ackIDs() []string {
	ps.Lock()
	defer ps.Unlock()

	v := make([]string, 0, ps.acks.Len())
	ps.acks.Iter(func(s string) bool {
		v = append(v, s)
		return true
	})
	sort.Strings(v)
	return v
}

func TestAckBuffer(t *testing.T) {
	t.Parallel()

	Convey(`An AckBuffer configuration using a testing Pub/Sub`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)
		ps := &testACK{}

		var discarded []string
		var discardedMu sync.Mutex

		cfg := Config{
			Ack: ps,
			DiscardCallback: func(acks []string) {
				discardedMu.Lock()
				defer discardedMu.Unlock()

				discarded = append(discarded, acks...)
			},
		}

		Convey(`Can instantiate an AckBuffer`, func() {
			ab := New(c, cfg)
			So(ab, ShouldNotBeNil)

			// Our tests will close/flush the buffer to synchronize. However, if they
			// don't, make sure we do so we don't spawn a bunch of floating
			// goroutines.
			closed := false
			closeOnce := func() {
				if !closed {
					closed = true
					ab.CloseAndFlush()
				}
			}
			defer closeOnce()

			Convey(`Can send ACKs.`, func() {
				acks := []string{"foo", "bar", "baz"}
				for _, v := range acks {
					ab.Ack(v)
				}
				tc.Add(DefaultMaxBufferTime)

				closeOnce()
				sort.Strings(acks)
				So(ps.ackIDs(), ShouldResemble, acks)
				So(discarded, ShouldBeNil)
			})

			Convey(`Will retry on transient Pub/Sub error`, func() {
				tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					tc.Add(d)
				})

				ps.err = errors.WrapTransient(errors.New("test error"))
				acks := []string{"foo", "bar", "baz"}
				for _, v := range acks {
					ab.Ack(v)
				}

				Convey(`And eventually discard the ACK.`, func() {
					closeOnce()

					sort.Strings(acks)
					sort.Strings(discarded)
					So(discarded, ShouldResemble, acks)
				})
			})
		})
	})
}
