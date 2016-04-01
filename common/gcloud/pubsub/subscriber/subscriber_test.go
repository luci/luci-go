// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package subscriber

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type event struct {
	msg *pubsub.Message
	err error
}

type testSource struct {
	sub    pubsub.Subscription
	eventC chan event
}

func (s *testSource) Pull(c context.Context, batch int) ([]*pubsub.Message, error) {
	var e event

	select {
	case <-c.Done():
		// Enforce determinism, preferring events.
		select {
		case e = <-s.eventC:
			break
		default:
			return nil, c.Err()
		}

	case e = <-s.eventC:
		break
	}

	switch {
	case e.err != nil:
		return nil, e.err

	case e.msg != nil:
		return []*pubsub.Message{e.msg}, nil

	default:
		return nil, nil
	}
}

func (s *testSource) message(id ...string) {
	for _, v := range id {
		if v != "" {
			s.eventC <- event{msg: &pubsub.Message{
				ID:    v,
				AckID: v,
				Data:  []byte(v),
			}}
		} else {
			s.eventC <- event{}
		}
	}
}

func (s *testSource) error(err error) {
	s.eventC <- event{err: err}
}

type testACK struct {
	sync.Mutex

	acks map[string]struct{}
}

func (a *testACK) Ack(id string) {
	a.Lock()
	defer a.Unlock()

	if a.acks == nil {
		a.acks = make(map[string]struct{})
	}
	a.acks[id] = struct{}{}
}

func (a *testACK) getACKs() []string {
	a.Lock()
	defer a.Unlock()
	return dumpStringSet(a.acks)
}

func dumpStringSet(s map[string]struct{}) []string {
	v := make([]string, 0, len(s))
	for a := range s {
		v = append(v, a)
	}
	sort.Strings(v)
	return v
}

func TestSubscriber(t *testing.T) {
	t.Parallel()

	Convey(`A Subscriber configuration using a testing Pub/Sub`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)

		c, cancelFunc := context.WithCancel(c)
		defer cancelFunc()

		src := &testSource{
			eventC: make(chan event),
		}
		ack := &testACK{}
		s := Subscriber{
			S:       src,
			A:       ack,
			Workers: 8,
		}

		// If not nil, processing goroutines will block on reads from this
		// channel, one per message.
		var pullC chan string

		var seenMu sync.Mutex
		seen := map[string]struct{}{}
		blacklist := map[string]struct{}{}
		runWith := func(cb func()) {
			doneC := make(chan struct{})
			go func() {
				defer close(doneC)
				s.Run(c, func(msg *pubsub.Message) bool {
					if pullC != nil {
						pullC <- msg.ID
					}

					seenMu.Lock()
					defer seenMu.Unlock()
					seen[msg.ID] = struct{}{}

					_, ok := blacklist[msg.ID]
					return !ok
				})
			}()

			cb()
			cancelFunc()
			<-doneC
		}

		Convey(`A Subscriber can pull and ACK messages.`, func() {
			var msgs []string
			pullC = make(chan string)
			runWith(func() {
				for i := 0; i < 1024; i++ {
					v := fmt.Sprintf("%08x", i)
					msgs = append(msgs, v)
					src.message(v)

					<-pullC
				}
			})

			So(dumpStringSet(seen), ShouldResemble, msgs)
			So(ack.getACKs(), ShouldResemble, msgs)
		})

		Convey(`A Subscriber that encounters an empty message set will sleep and try again.`, func() {
			var count int32
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				if atomic.AddInt32(&count, 1) > 1 {
					panic("should not have this many callbacks")
				}
				tc.Add(d)
			})

			runWith(func() {
				src.message("a", "b", "", "c", "d")
			})

			So(dumpStringSet(seen), ShouldResemble, []string{"a", "b", "c", "d"})
			So(ack.getACKs(), ShouldResemble, []string{"a", "b", "c", "d"})
		})

		Convey(`A Subscriber that encounters a Source error will sleep and try again.`, func() {
			var count int32
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				if atomic.AddInt32(&count, 1) > 1 {
					panic("should not have this many callbacks")
				}
				tc.Add(d)
			})

			runWith(func() {
				src.message("a", "b")
				src.error(errors.New("test error"))
				src.message("c", "d")
			})

			So(dumpStringSet(seen), ShouldResemble, []string{"a", "b", "c", "d"})
			So(ack.getACKs(), ShouldResemble, []string{"a", "b", "c", "d"})
		})
	})
}
