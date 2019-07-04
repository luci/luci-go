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
	. "go.chromium.org/luci/common/testing/assertions"
)

func dummySendFn(ctx context.Context, data *Batch) error {
	return nil
}

func dummyErrorFn(ctx context.Context, failedBatch *Batch, err error) bool {
	return false
}

func TestOptionValidationGood(t *testing.T) {
	t.Parallel()

	fullOptions := Options{
		SendFn:  dummySendFn,
		ErrorFn: dummyErrorFn,
		Concurrency: ConcurrencyOptions{
			MaxSenders: 7,
			MaxQPS:     1337.0,
		},
		Retry: RetryOptions{
			InitialSleep:  1337 * time.Millisecond,
			MaxSleep:      9000 * time.Hour,
			BackoffFactor: 11,
			Limit:         7,
		},
		Batch: BatchOptions{
			MaxSize:     99,
			MaxDuration: 2 * time.Minute,
		},
		Buffer: BufferOptions{
			MaxSize:      12,
			FullBehavior: DropOldestBatch,
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

				ErrorFn:     defaultErrorFn,
				Concurrency: defaultConcurrencyOptions,
				Retry:       defaultRetryOptions,
				Batch:       defaultBatchOptions,
				Buffer:      defaultBufferOptions,
			},
		},

		{
			name:     "full",
			options:  fullOptions,
			expected: fullOptions,
		},

		{
			name: "Batch.MaxSize == -1 OK",
			options: Options{
				SendFn: dummySendFn,
				Batch: BatchOptions{
					MaxSize:     -1,
					MaxDuration: defaultBatchOptions.MaxDuration,
				},
			},
			expected: Options{
				SendFn:  dummySendFn,
				ErrorFn: defaultErrorFn,

				Concurrency: defaultConcurrencyOptions,
				Retry:       defaultRetryOptions,
				Batch: BatchOptions{
					MaxSize:     -1,
					MaxDuration: defaultBatchOptions.MaxDuration,
				},
				Buffer: defaultBufferOptions,
			},
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
			"concurrency: MaxSenders",
			Options{
				SendFn:      dummySendFn,
				Concurrency: ConcurrencyOptions{MaxSenders: -2},
			},
			"MaxSenders must be",
		},

		{
			"concurrency: MaxQPS",
			Options{
				SendFn:      dummySendFn,
				Concurrency: ConcurrencyOptions{MaxQPS: -0.1},
			},
			"MaxQPS must be",
		},

		{
			"retry: InitialSleep",
			Options{
				SendFn: dummySendFn,
				Retry:  RetryOptions{InitialSleep: -time.Millisecond},
			},
			"InitialSleep must be",
		},

		{
			"retry: MaxSleep: smaller than given InitialSleep",
			Options{
				SendFn: dummySendFn,
				Retry: RetryOptions{
					InitialSleep: time.Minute,
					MaxSleep:     400 * time.Millisecond,
				},
			},
			"MaxSleep must be",
		},

		{
			"retry: MaxSleep: smaller than default InitialSleep",
			Options{
				SendFn: dummySendFn,
				Retry:  RetryOptions{MaxSleep: time.Millisecond},
			},
			"MaxSleep must be",
		},

		{
			"retry: BackoffFactor",
			Options{
				SendFn: dummySendFn,
				Retry:  RetryOptions{BackoffFactor: 0.9},
			},
			"BackoffFactor must be",
		},

		{
			"retry: Limit",
			Options{
				SendFn: dummySendFn,
				Retry:  RetryOptions{Limit: -1},
			},
			"Limit must be",
		},

		{
			"batch: MaxSize",
			Options{
				SendFn: dummySendFn,
				Batch:  BatchOptions{MaxSize: -2},
			},
			"MaxSize must be",
		},

		{
			"batch: MaxDuration",
			Options{
				SendFn: dummySendFn,
				Batch:  BatchOptions{MaxDuration: -time.Minute},
			},
			"MaxDuration must be",
		},

		{
			"buffer: MaxSize",
			Options{
				SendFn: dummySendFn,
				Buffer: BufferOptions{MaxSize: -1},
			},
			"MaxSize must be",
		},

		{
			"buffer: FullBehavior",
			Options{
				SendFn: dummySendFn,
				Buffer: BufferOptions{FullBehavior: 20},
			},
			"FullBehavior is unknown",
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
