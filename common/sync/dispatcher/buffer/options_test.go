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

package buffer

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBatchSizeGuessing(t *testing.T) {
	t.Parallel()

	Convey(`Options.BatchSizeGuess`, t, func() {
		Convey(`default BatchSize`, func() {
			o := Options{}
			So(o.normalize(), ShouldBeNil)
			So(o.BatchSizeGuess(), ShouldEqual, 20)
		})

		Convey(`guess on zero BatchSize`, func() {
			o := Options{BatchSize: -1}
			So(o.normalize(), ShouldBeNil)
			So(o.BatchSizeGuess(), ShouldEqual, 10)
		})

		Convey(`guess on different BatchSize`, func() {
			o := Options{BatchSize: 93}
			So(o.normalize(), ShouldBeNil)
			So(o.BatchSizeGuess(), ShouldEqual, 93)
		})
	})
}

func TestOptionValidationGood(t *testing.T) {
	t.Parallel()

	var goodOptions = []struct {
		name     string
		options  Options
		expected Options
	}{
		{
			name:     "minimal",
			options:  Options{},
			expected: Defaults,
		},

		{
			name: "full",
			options: Options{
				BatchSize:     99,
				BatchDuration: 2 * time.Minute,
				MaxItems:      100,
				FullBehavior:  DropOldestBatch,
				Retry:         retry.None,
			},
			expected: Options{
				BatchSize:     99,
				BatchDuration: 2 * time.Minute,
				MaxItems:      100,
				FullBehavior:  DropOldestBatch,
				Retry:         retry.None,
			},
		},

		{
			name: "Batch.BatchSize == -1 OK",
			options: Options{
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				MaxItems:      Defaults.MaxItems,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
			expected: Options{
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				MaxItems:      Defaults.MaxItems,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
		},
	}

	Convey(`test good option groups`, t, func() {
		for _, options := range goodOptions {
			Convey(options.name, func() {
				myOptions := options.options
				expect := options.expected

				So(myOptions.normalize(), ShouldBeNil)

				// ShouldResemble doesn't like function pointers, apparently, so
				// explicitly compare Retry field.
				So(myOptions.Retry, ShouldEqual, expect.Retry)
				myOptions.Retry = nil
				expect.Retry = nil

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
			"BatchSize",
			Options{
				BatchSize: -2,
			},
			"BatchSize must be",
		},

		{
			"BatchDuration",
			Options{
				BatchDuration: -time.Minute,
			},
			"BatchDuration must be",
		},

		{
			"buffer: MaxItems",
			Options{
				MaxItems: -1,
			},
			"MaxItems must be",
		},

		{
			"buffer: FullBehavior",
			Options{
				FullBehavior: 20,
			},
			"FullBehavior is unknown",
		},
	}

	Convey(`test bad option groups`, t, func() {
		for _, options := range badOptions {
			Convey(options.name, func() {
				myOptions := options.options
				So(myOptions.normalize(), ShouldErrLike, options.expected)
			})
		}
	})
}
