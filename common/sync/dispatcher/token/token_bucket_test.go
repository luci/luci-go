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

package token

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testTicker chan time.Time

func (tt testTicker) GetC() <-chan time.Time { return tt }
func (tt testTicker) Stop()                  {}
func (tt testTicker) Poke()                  { tt <- time.Time{} }

var globalTicker = make(testTicker)

func init() {
	newTicker = func(time.Duration) iTicker { return globalTicker }
}

func TestBucket(t *testing.T) {
	assertBucketEmpty := func(tb *Bucket) {
		select {
		case <-tb.C:
			panic("Bucket is not empty")
		default:
		}
	}
	popToken := func(tb *Bucket) {
		select {
		case <-tb.C:
		// can't use 'default' here because the channel may not actually be ready
		// yet; sleeping for a bit lets the other loop catch up.
		case <-time.After(time.Millisecond):
			panic("Bucket is empty")
		}
	}

	Convey(`Bucket`, t, func(cvctx C) {
		bucket := NewBucket(context.Background(), 10, 2)
		defer bucket.Close()

		Convey(`depth must be positive`, func() {
			So(
				func() { NewBucket(context.Background(), -1, 100) },
				ShouldPanicLike, "depth must also be positive")
		})

		Convey(`closure wakes readers up`, func() {
			buddyDone := make(chan struct{})
			go func() {
				<-bucket.C
				buddyDone <- struct{}{}
			}()

			globalTicker.Poke()

			bucket.Close()
			<-buddyDone
		})

		Convey(`closure wakes writer up`, func() {
			for i := 0; i < 10; i++ {
				globalTicker.Poke()
			}

			bucket.Close()
		})

		Convey(`bucket grows over time`, func() {
			assertBucketEmpty(bucket)

			globalTicker.Poke()
			popToken(bucket)
			assertBucketEmpty(bucket)

			for i := 0; i < 10; i++ {
				globalTicker.Poke()
			}

			for i := 0; i < 10; i++ {
				popToken(bucket)
			}

			assertBucketEmpty(bucket)
		})

		Convey(`will block on token insertion if full`, func() {
			for i := 0; i < 10; i++ {
				globalTicker.Poke()
			}
			globalTicker.Poke()
			time.Sleep(time.Millisecond)
			for i := 0; i < 10; i++ {
				popToken(bucket)
			}
		})

		Convey(`close unblocks token insert`, func() {
			for i := 0; i < 10; i++ {
				globalTicker.Poke()
			}
			globalTicker.Poke()
			time.Sleep(time.Millisecond)
			bucket.Close()
		})
	})

	Convey(`Bucket infinite`, t, func() {
		// infinite bucket never blocks
		bucket := NewBucket(context.Background(), 0, 0)
		defer bucket.Close()
		<-bucket.C
		<-bucket.C
		<-bucket.C
		<-bucket.C
		<-bucket.C
		<-bucket.C
		<-bucket.C
		<-bucket.C
	})
}
