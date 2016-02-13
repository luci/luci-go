// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ackbuffer

import (
	"fmt"
	"sync"
	"testing"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type testACK struct {
	sync.Mutex

	err       error
	ackC      chan []string
	acks      []string
	batchSize int
}

func (ps *testACK) Ack(c context.Context, acks ...string) error {
	if ps.ackC != nil {
		ps.ackC <- acks
	}

	ps.Lock()
	defer ps.Unlock()

	if ps.err != nil {
		return ps.err
	}

	for _, ack := range acks {
		ps.acks = append(ps.acks, ack)
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

	v := make([]string, len(ps.acks))
	copy(v, ps.acks)
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

			Convey(`Will buffer ACKs until enough are sent.`, func() {
				ps.ackC = make(chan []string)
				acks := make([]string, ps.AckBatchSize())

				// Fill up the entire batch, which will cause an automatic dump.
				for i := range acks {
					acks[i] = fmt.Sprintf("%d", i)
					ab.Ack(acks[i])
				}
				<-ps.ackC

				So(ps.ackIDs(), ShouldResemble, acks)
				So(discarded, ShouldBeNil)
			})

			Convey(`Will buffer ACKs and send if time has expired.`, func() {
				ps.ackC = make(chan []string)
				ab.ackReceivedC = make(chan string)

				acks := []string{"foo", "bar", "baz"}
				for _, v := range acks {
					ab.Ack(v)

					// Acknoweldge that all ACKs have been received before we advance
					// our timer. This will ensure that the timer triggers AFTER the ACKs
					// are buffered.
					<-ab.ackReceivedC
				}
				tc.Add(DefaultMaxBufferTime)

				<-ps.ackC
				So(ps.ackIDs(), ShouldResemble, acks)
				So(discarded, ShouldBeNil)
			})

			Convey(`Will flush any remaining ACKs on close.`, func() {
				acks := []string{"foo", "bar", "baz"}
				for _, v := range acks {
					ab.Ack(v)
				}
				closeOnce()

				So(ps.ackIDs(), ShouldResemble, acks)
				So(discarded, ShouldBeNil)
			})

			Convey(`Will discard the ACK if it could not be sent`, func() {
				ps.err = errors.WrapTransient(errors.New("test error"))
				acks := []string{"foo", "bar", "baz"}
				for _, v := range acks {
					ab.Ack(v)
				}
				closeOnce()

				So(discarded, ShouldResemble, acks)
			})
		})
	})
}
