// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package subscriber

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gcps"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"

	. "github.com/smartystreets/goconvey/convey"
)

type testPubSub struct {
	c     context.Context
	errCB func() error
	sub   gcps.Subscription
	msgC  chan *pubsub.Message
}

func (ps *testPubSub) Pull(s gcps.Subscription, amount int) ([]*pubsub.Message, error) {
	if ps.errCB != nil {
		if err := ps.errCB(); err != nil {
			return nil, err
		}
	}
	if ps.sub != s {
		return nil, fmt.Errorf("unknown subscription %q", s)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("invalid amount: %d", amount)
	}

	select {
	case <-ps.c.Done():
		return nil, ps.c.Err()

	case msg := <-ps.msgC:
		return []*pubsub.Message{msg}, nil
	}
}

func (ps *testPubSub) send(s ...string) {
	for _, v := range s {
		ps.msgC <- &pubsub.Message{
			ID:   v,
			Data: []byte(v),
		}
	}
}

func TestSubscriber(t *testing.T) {
	t.Parallel()

	Convey(`A Subscriber configuration using a testing Pub/Sub`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)

		c, cancelFunc := context.WithCancel(c)
		defer cancelFunc()

		ps := &testPubSub{
			c:    c,
			sub:  gcps.Subscription("testsub"),
			msgC: make(chan *pubsub.Message),
		}
		s := Subscriber{
			PubSub:       ps,
			Subscription: ps.sub,
		}

		var received []string
		var receivedMu sync.Mutex
		cb := func(msg *pubsub.Message) {
			receivedMu.Lock()
			defer receivedMu.Unlock()
			received = append(received, msg.ID)
		}

		// If a subscriber goroutine is sleeping, advance to wake it.
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		Convey(`A Subscriber can pull messages.`, func() {
			doneC := make(chan struct{})
			go func() {
				defer close(doneC)
				s.Run(c, cb)
			}()

			var msgs []string
			for i := 0; i < 1024; i++ {
				msgs = append(msgs, fmt.Sprintf("%08x", i))
			}
			ps.send(msgs...)

			cancelFunc()
			<-doneC

			sort.Strings(msgs)
			sort.Strings(received)
			So(received, ShouldResemble, msgs)
		})

		Convey(`A Subscriber will continue retrying if there is a Pub/Sub error.`, func() {
			var errMu sync.Mutex
			count := 0
			ps.errCB = func() error {
				errMu.Lock()
				defer errMu.Unlock()

				if count < 1024 {
					count++
					return errors.WrapTransient(errors.New("test error"))
				}
				return nil
			}

			doneC := make(chan struct{})
			go func() {
				defer close(doneC)
				s.Run(c, cb)
			}()

			var msgs []string
			for i := 0; i < 1024; i++ {
				msgs = append(msgs, fmt.Sprintf("%08x", i))
			}
			ps.send(msgs...)

			cancelFunc()
			<-doneC

			sort.Strings(msgs)
			sort.Strings(received)
			So(received, ShouldResemble, msgs)
		})
	})
}
