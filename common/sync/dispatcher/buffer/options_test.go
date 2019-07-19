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

func TestOptionValidationGood(t *testing.T) {
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
				MaxLeases:     2,
				BatchSize:     99,
				BatchDuration: 2 * time.Minute,
				FullBehavior:  &DropOldestBatch{400},
				Retry:         retry.None,
			},
			expected: Options{
				MaxLeases:     2,
				BatchSize:     99,
				BatchDuration: 2 * time.Minute,
				FullBehavior:  &DropOldestBatch{400},
				Retry:         retry.None,
			},
		},

		{
			name: "BatchSize == -1 OK",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "DropOldestBatch default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &DropOldestBatch{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &DropOldestBatch{1000},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "DropOldestBatch default large BatchSize",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     2000,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &DropOldestBatch{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     2000,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &DropOldestBatch{2000},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BlockNewItems default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     -1,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &BlockNewItems{1000},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BlockNewItems default large BatchSize",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     2000,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchSize:     2000,
				BatchDuration: Defaults.BatchDuration,
				FullBehavior:  &BlockNewItems{2000},
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
	var badOptions = []struct {
		name     string
		options  Options
		expected string
	}{
		{
			"MaxLeases",
			Options{
				MaxLeases: -1,
			},
			"MaxLeases must be",
		},

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
			"DropOldestBatch.MaxLiveItems < -1",
			Options{
				FullBehavior: &DropOldestBatch{-2},
			},
			"DropOldestBatch.MaxLiveItems must be",
		},

		{
			"BlockNewItems.MaxItems < -1",
			Options{
				FullBehavior: &BlockNewItems{-2},
			},
			"BlockNewItems.MaxItems must be",
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
