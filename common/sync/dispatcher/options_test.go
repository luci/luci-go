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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOptionValidationGood(t *testing.T) {
	fullOptions := Options[string]{
		ErrorFn:  func(*buffer.Batch[string], error) bool { return false },
		DropFn:   DropFnQuiet[string],
		QPSLimit: rate.NewLimiter(rate.Inf, 0),
		Buffer:   buffer.Defaults,
	}

	var goodOptions = []struct {
		name     string
		options  Options[string]
		expected Options[string]
	}{
		{
			name: "minimal",
			options: Options[string]{
				Buffer: buffer.Defaults,
			},
			expected: Options[string]{
				QPSLimit: rate.NewLimiter(rate.Inf, 0),
				Buffer:   buffer.Defaults,
			},
		},

		{
			name:     "full",
			options:  fullOptions,
			expected: fullOptions,
		},
	}

	Convey(`test good option groups`, t, func() {
		ctx := context.Background()
		for _, options := range goodOptions {
			Convey(options.name, func() {
				myOptions := options.options
				expect := options.expected

				// ShouldResemble has issues with function pointers; don't care about
				// testing buffer options in this test.
				myOptions.Buffer.Retry = nil
				expect.Buffer.Retry = nil

				So(myOptions.normalize(ctx), ShouldBeNil)

				// Directly compare function pointers; ShouldResemble doesn't compare
				// them sensibly.
				if expect.ErrorFn == nil {
					So(myOptions.ErrorFn, ShouldNotBeNil) // default is non-nil
				} else {
					So(myOptions.ErrorFn, ShouldEqual, expect.ErrorFn)
					expect.ErrorFn = nil
				}
				myOptions.ErrorFn = nil

				if expect.DropFn == nil {
					So(myOptions.DropFn, ShouldNotBeNil) // default is non-nil
				} else {
					So(myOptions.DropFn, ShouldEqual, expect.DropFn)
					expect.DropFn = nil
				}
				myOptions.DropFn = nil

				if expect.ItemSizeFunc == nil {
					So(myOptions.ItemSizeFunc, ShouldBeNil)
				} else {
					So(myOptions.ItemSizeFunc, ShouldEqual, expect.ItemSizeFunc)
					expect.ItemSizeFunc = nil
				}
				myOptions.ItemSizeFunc = nil

				So(myOptions, ShouldResemble, expect)
			})
		}
	})
}

func TestOptionValidationBad(t *testing.T) {
	Convey(`bad option validation`, t, func() {
		ctx := context.Background()

		Convey(`QPSLimit`, func() {
			opts := Options[string]{QPSLimit: rate.NewLimiter(100, 0), Buffer: buffer.Defaults}
			So(opts.normalize(ctx), ShouldErrLike, "QPSLimit has burst size < 1")
		})

		Convey(`ItemSizeFunc == nil`, func() {
			Convey(`BatchSizeMax > 0`, func() {
				opts := Options[string]{Buffer: buffer.Defaults}
				opts.Buffer.BatchSizeMax = 1000
				So(opts.normalize(ctx), ShouldErrLike, "Buffer.BatchSizeMax > 0")
			})
		})

		Convey(`MinQPS`, func() {
			Convey(`MinQPS == rate.Inf`, func() {
				opts := Options[string]{
					MinQPS: rate.Inf,
					Buffer: buffer.Defaults}
				So(opts.normalize(ctx), ShouldErrLike, "MinQPS cannot be infinite")
			})
			Convey(`MinQPS greater than QPSLimit`, func() {
				opts := Options[string]{
					QPSLimit: rate.NewLimiter(100, 1),
					MinQPS:   rate.Every(time.Millisecond),
					Buffer:   buffer.Defaults}
				So(opts.normalize(ctx), ShouldErrLike, "MinQPS: 1000.000000 is greater than QPSLimit: 100.000000")
			})
		})
	})
}
