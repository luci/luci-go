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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQPSBucket(t *testing.T) {
	assertBucketEmpty := func(qb *qpsBucket) {
		select {
		case <-qb.ch:
			panic("qpsBucket is not empty")
		default:
		}
	}
	popToken := func(qb *qpsBucket) {
		select {
		case <-qb.ch:
		case <-time.After(time.Millisecond): // let filler routine run
			panic("qpsBucket is empty")
		}
	}

	Convey(`QPSBucket`, t, func(cvctx C) {
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		tick := make(chan struct{})
		defer close(tick)

		stepClock := func() {
			tick <- struct{}{}
		}

		tclock.SetTimerCallback(func(amount time.Duration, timer clock.Timer) {
			if testclock.HasTags(timer, "qpsBucket") {
				<-tick
				tclock.Add(amount + time.Millisecond)
			}
		})

		bucket := newQPSBucket(ctx, 10, 2)
		defer bucket.Close()
		assertBucketEmpty(bucket)

		stepClock()
		popToken(bucket)
		assertBucketEmpty(bucket)

		for i := 0; i < 10; i++ { // can sleep more than the bucket size
			stepClock()
		}

		for i := 0; i < 10; i++ {
			popToken(bucket)
		}

		assertBucketEmpty(bucket)
	})

	Convey(`QPSBucket infinite`, t, func() {
		// infinite bucket never blocks
		bucket := newInfiniteQPSBucket()
		defer bucket.Close()
		bucket.GetToken()
		bucket.GetToken()
		bucket.GetToken()
		bucket.GetToken()
		bucket.GetToken()
		bucket.GetToken()
	})
}
