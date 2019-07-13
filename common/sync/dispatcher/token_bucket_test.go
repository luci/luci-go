// Copyright 2019 The LUCI Authors.
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

package dispatcher

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type testTicker chan time.Time

func (tt testTicker) GetC() <-chan time.Time { return tt }
func (tt testTicker) Stop()                  {}
func (tt testTicker) Poke()                  { tt <- time.Time{} }

func TestQPSBucket(t *testing.T) {
	assertBucketEmpty := func(qb *qpsBucket) {
		select {
		case <-qb.GetC():
			panic("qpsBucket is not empty")
		default:
		}
	}
	popToken := func(qb *qpsBucket) {
		select {
		case <-qb.GetC():
		// can't use 'default' here because the channel may not actually be ready
		// yet; sleeping for a bit lets the other loop catch up.
		case <-time.After(time.Millisecond):
			panic("qpsBucket is empty")
		}
	}

	Convey(`QPSBucket`, t, func(cvctx C) {
		ticker := make(testTicker)
		originalNewticker := newTicker
		newTicker = func(time.Duration) iTicker { return ticker }
		bucket := newQPSBucket(context.Background(), 10, 2)

		defer close(ticker)
		defer func() { newTicker = originalNewticker }()
		defer bucket.Close()

		Convey(`closure wakes readers up`, func() {
			buddyDone := make(chan struct{})
			go func() {
				<-bucket.GetC()
				buddyDone <- struct{}{}
			}()

			ticker.Poke()

			bucket.Close()
			<-buddyDone
		})

		Convey(`closure wakes writer up`, func() {
			for i := 0; i < 10; i++ {
				Println("poke")
				ticker.Poke()
			}

			bucket.Close()
		})

		Convey(`bucket grows over time`, func() {
			assertBucketEmpty(bucket)

			ticker.Poke()
			popToken(bucket)
			assertBucketEmpty(bucket)

			for i := 0; i < 10; i++ {
				ticker.Poke()
			}

			for i := 0; i < 10; i++ {
				popToken(bucket)
			}

			assertBucketEmpty(bucket)
		})

		Convey(`will block on token insertion if full`, func() {
			for i := 0; i < 10; i++ {
				ticker.Poke()
			}
			ticker.Poke()
			time.Sleep(time.Millisecond)
			for i := 0; i < 10; i++ {
				popToken(bucket)
			}
		})

		Convey(`close unblocks token insert`, func() {
			for i := 0; i < 10; i++ {
				ticker.Poke()
			}
			ticker.Poke()
			time.Sleep(time.Millisecond)
			bucket.Close()
		})
	})

	Convey(`QPSBucket infinite`, t, func() {
		// infinite bucket never blocks
		bucket := newInfiniteQPSBucket()
		defer bucket.Close()
		ch := bucket.GetC()
		<-ch
		<-ch
		<-ch
		<-ch
		<-ch
		<-ch
		<-ch
	})
}
