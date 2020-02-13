// Copyright 2020 The LUCI Authors.
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

package retry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock/testclock"
)

var (
	t1 = errors.BoolTag{Key: errors.NewTagKey("t1")}
	t2 = errors.BoolTag{Key: errors.NewTagKey("t2")}
	t3 = errors.BoolTag{Key: errors.NewTagKey("t3")}
)

type retryOnTag struct {
	errTag errors.BoolTag
	delay time.Duration
}

// Next implements a simplified, unlimited version of exponential backoff, and
// only retries on errors with errTag.
func (r *retryOnTag) Next(ctx context.Context, err error) time.Duration {
	if !r.errTag.In(err) {
		return Stop
	}
	delay := r.delay
	r.delay *= 2
	return delay
}

func retryOnTagFactory(errTag errors.BoolTag, delay time.Duration) Factory {
	return func() Iterator {
		return &retryOnTag{
			errTag: errTag,
			delay:  delay,
		}
	}
}

func TestChain(t *testing.T) {
	t.Parallel()

	Convey(`chainIterator`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		ci := ChainFactories(
			retryOnTagFactory(t1, time.Second),
			retryOnTagFactory(t2, 3*time.Second))()

		Convey(`All errors are the same, retries as exponential backoff.`, func() {
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, time.Second)
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, 2*time.Second)
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, 4*time.Second)
		})

		Convey(`Can also retry when encounter mixed errors.`, func() {
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, time.Second)
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, 2*time.Second)
			So(ci.Next(ctx, t2.Apply(fmt.Errorf("err"))), ShouldEqual, 3*time.Second)
			So(ci.Next(ctx, t2.Apply(fmt.Errorf("err"))), ShouldEqual, 6*time.Second)
			// Iterator for t1 is reset.
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, time.Second)
			So(ci.Next(ctx, t1.Apply(fmt.Errorf("err"))), ShouldEqual, 2*time.Second)
			So(ci.Next(ctx, t3.Apply(fmt.Errorf("err"))), ShouldEqual, Stop)
		})
	})
}