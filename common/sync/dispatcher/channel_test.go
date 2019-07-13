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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func dummySendFn(*buffer.Batch) error { return nil }

func noDrop(dropped *buffer.Batch) { panic(fmt.Sprintf("dropping %+v", dropped)) }

func TestChannelConstruction(t *testing.T) {
	t.Parallel()

	Convey(`Channel`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		Convey(`construction`, func() {

			Convey(`success`, func() {
				ch, err := NewChannel(ctx, nil, dummySendFn)
				So(err, ShouldBeNil)
				ch.Close(ctx)
			})

			Convey(`failure`, func() {
				Convey(`bad SendFn`, func() {
					_, err := NewChannel(ctx, nil, nil)
					So(err, ShouldErrLike, "send is required")
				})

				Convey(`bad Options`, func() {
					_, err := NewChannel(ctx, &Options{
						MaxSenders: -1,
					}, dummySendFn)
					So(err, ShouldErrLike, "normalizing dispatcher.Options")
				})

				Convey(`bad Options.Buffer`, func() {
					_, err := NewChannel(ctx, &Options{
						Buffer: buffer.Options{
							MaxItems: -1,
						},
					}, dummySendFn)
					So(err, ShouldErrLike, "allocating Buffer")
				})
			})

		})

	})

}

func TestSerialSenderWithoutDrops(t *testing.T) {
	Convey(`serial world-state sender without drops`, t, func(cvctx C) {
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

		sentBatches := []string{}
		enableThisError := false

		ch, err := NewChannel(ctx, &Options{
			DropFn:     noDrop,
			MaxSenders: 1,
			MaxQPS:     -1,
			Buffer: buffer.Options{
				BatchSize:    1,
				FullBehavior: buffer.BlockNewItems,
			},
		}, func(batch *buffer.Batch) (err error) {
			cvctx.So(batch.Data, ShouldHaveLength, 1)
			str := batch.Data[0].(string)
			if enableThisError && str == "This" {
				enableThisError = false
				return errors.New("narp", transient.Tag)
			}
			sentBatches = append(sentBatches, str)
			if str == "test." {
				// advance clock by a minute; in the retry case this will let "This"
				// be retried.
				tclock.Set(tclock.Now().Add(time.Minute))
			}
			return nil
		})
		So(err, ShouldBeNil)
		defer ch.Close(ctx)

		Convey(`no errors`, func() {
			ch.Chan() <- "Hello"
			ch.Chan() <- "World!"
			ch.Chan() <- "This"
			ch.Chan() <- "is"
			ch.Chan() <- "a"
			ch.Chan() <- "test."
			ch.Close(ctx)

			So(sentBatches, ShouldResemble, []string{
				"Hello", "World!",
				"This", "is", "a", "test.",
			})
		})

		Convey(`error and retry`, func() {
			enableThisError = true

			ch.Chan() <- "Hello"
			ch.Chan() <- "World!"
			ch.Chan() <- "This"
			ch.Chan() <- "is"
			ch.Chan() <- "a"
			ch.Chan() <- "test."
			ch.Close(ctx)

			So(sentBatches, ShouldResemble, []string{
				"Hello", "World!",
				"is", "a", "test.", "This",
			})
		})

	})
}
