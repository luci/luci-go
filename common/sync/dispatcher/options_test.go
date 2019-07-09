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

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func dummySendFn(ctx context.Context, data *buffer.Batch) error {
	return nil
}

func dummyErrorFn(ctx context.Context, failedBatch *buffer.Batch, err error) bool {
	return false
}

func TestOptionValidationGood(t *testing.T) {
	t.Parallel()

	fullOptions := Options{
		SendFn:     dummySendFn,
		ErrorFn:    dummyErrorFn,
		MaxSenders: 7,
		MaxQPS:     1337.0,

		Buffer: buffer.Options{
			BatchSize:     99,
			BatchDuration: 2 * time.Minute,
			MaxItems:      12,
			FullBehavior:  buffer.DropOldestBatch,
			Retry:         retry.None,
		},
	}

	var goodOptions = []struct {
		name     string
		options  Options
		expected Options
	}{
		{
			name: "minimal",
			options: Options{
				SendFn: dummySendFn,
			},
			expected: Options{
				SendFn: dummySendFn,

				ErrorFn:    Defaults.ErrorFn,
				MaxSenders: Defaults.MaxSenders,
				MaxQPS:     Defaults.MaxQPS,
				Buffer:     buffer.Defaults,
			},
		},

		{
			name:     "full",
			options:  fullOptions,
			expected: fullOptions,
		},
	}

	Convey(`test good option groups`, t, func() {
		for _, options := range goodOptions {
			Convey(options.name, func() {
				myOptions := options.options
				expect := options.expected

				So(myOptions.Normalize(), ShouldBeNil)

				// ShouldResemble has issues with function pointers, so compare them
				// explicitly.
				So(myOptions.SendFn, ShouldEqual, expect.SendFn)
				So(myOptions.ErrorFn, ShouldEqual, expect.ErrorFn)

				myOptions.SendFn = nil
				myOptions.ErrorFn = nil
				expect.SendFn = nil
				expect.ErrorFn = nil
				myOptions.Buffer.Retry = nil
				expect.Buffer.Retry = nil

				So(myOptions, ShouldResemble, expect)
			})
		}
	})
}

func TestOptionValidationBad(t *testing.T) {
	t.Parallel()

	var badOptions = []struct {
		name     string
		options  Options
		expected string
	}{
		{
			"no SendFn",
			Options{},
			"SendFn is required",
		},

		{
			"MaxSenders",
			Options{
				SendFn:     dummySendFn,
				MaxSenders: -2,
			},
			"MaxSenders must be",
		},

		{
			"MaxQPS",
			Options{
				SendFn: dummySendFn,
				MaxQPS: -0.1,
			},
			"MaxQPS must be",
		},
	}

	Convey(`test bad option groups`, t, func() {
		for _, options := range badOptions {
			Convey(options.name, func() {
				myOptions := options.options
				So(myOptions.Normalize(), ShouldErrLike, options.expected)
			})
		}
	})
}
