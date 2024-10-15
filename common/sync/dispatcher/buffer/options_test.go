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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
				BatchItemsMax: 99,
				BatchSizeMax:  10,
				BatchAgeMax:   2 * time.Minute,
				FullBehavior:  &DropOldestBatch{MaxLiveItems: 400, MaxLiveSize: -1},
				Retry:         retry.None,
			},
			expected: Options{
				MaxLeases:     2,
				BatchItemsMax: 99,
				BatchSizeMax:  10,
				BatchAgeMax:   2 * time.Minute,
				FullBehavior:  &DropOldestBatch{MaxLiveItems: 400, MaxLiveSize: -1},
				Retry:         retry.None,
			},
		},

		{
			name: "BatchItemsMax == -1 OK",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BatchSizeMax == 0 -> default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BatchSizeMax == 0 w/ BlockNewItems -> default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{MaxItems: 1000, MaxSize: -1},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BatchSizeMax > 0 -> default sizer func",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  10000,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  10000,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  Defaults.FullBehavior,
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "DropOldestBatch default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &DropOldestBatch{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &DropOldestBatch{MaxLiveItems: 1000, MaxLiveSize: -1},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "DropOldestBatch default large BatchItemsMax",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: 2000,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &DropOldestBatch{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: 2000,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &DropOldestBatch{MaxLiveItems: 2000, MaxLiveSize: -1},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BlockNewItems default",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: Defaults.BatchItemsMax,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{MaxItems: 1000, MaxSize: -1},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BlockNewItems size only",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  10000,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: -1,
				BatchSizeMax:  10000,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{MaxItems: -1, MaxSize: 50000},
				Retry:         Defaults.Retry,
			},
		},

		{
			name: "BlockNewItems default large BatchItemsMax",
			options: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: 2000,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{},
				Retry:         Defaults.Retry,
			},
			expected: Options{
				MaxLeases:     Defaults.MaxLeases,
				BatchItemsMax: 2000,
				BatchSizeMax:  Defaults.BatchSizeMax,
				BatchAgeMax:   Defaults.BatchAgeMax,
				FullBehavior:  &BlockNewItems{MaxItems: 2000, MaxSize: -1},
				Retry:         Defaults.Retry,
			},
		},
	}

	ftt.Run(`test good option groups`, t, func(t *ftt.Test) {
		for _, options := range goodOptions {
			t.Run(options.name, func(t *ftt.Test) {
				myOptions := options.options
				expect := options.expected

				assert.Loosely(t, myOptions.normalize(), should.BeNil)

				// ShouldResemble doesn't like function pointers, apparently, so
				// explicitly compare the Retry field.
				assert.Loosely(t, myOptions.Retry, should.Match(expect.Retry))
				myOptions.Retry = nil
				expect.Retry = nil

				assert.Loosely(t, myOptions, should.Resemble(expect))
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
			"BatchItemsMax",
			Options{
				BatchItemsMax: -2,
			},
			"BatchItemsMax must be",
		},

		{
			"BatchSizeMax",
			Options{
				BatchSizeMax: -2,
			},
			"BatchSizeMax must be",
		},

		{
			"BatchAgeMax",
			Options{
				BatchAgeMax: -time.Minute,
			},
			"BatchAgeMax must be",
		},

		{
			"FIFO",
			Options{
				MaxLeases: 10,
				FIFO:      true,
			},
			"FIFO is true, but MaxLeases",
		},

		{
			"DropOldestBatch.MaxLiveItems < -1",
			Options{
				FullBehavior: &DropOldestBatch{MaxLiveItems: -2},
			},
			"DropOldestBatch.MaxLiveItems must be",
		},

		{
			"DropOldestBatch.MaxLiveSize < -1",
			Options{
				FullBehavior: &DropOldestBatch{MaxLiveSize: -2},
			},
			"DropOldestBatch.MaxLiveSize must be",
		},

		{
			"DropOldestBatch.MaxLiveItems == DropOldestBatch.MaxLiveSize == -1",
			Options{
				FullBehavior: &DropOldestBatch{MaxLiveItems: -1, MaxLiveSize: -1},
			},
			"DropOldestBatch must have one of",
		},

		{
			"DropOldestBatch.MaxLiveSize > 0 && BatchSizeMax == -1",
			Options{
				FullBehavior: &DropOldestBatch{MaxLiveSize: 100},
			},
			"DropOldestBatch.MaxLiveSize may only be set",
		},

		{
			"BlockNewItems.MaxItems < -1",
			Options{
				FullBehavior: &BlockNewItems{MaxItems: -2},
			},
			"BlockNewItems.MaxItems must be",
		},

		{
			"BlockNewItems.MaxSize < -1",
			Options{
				FullBehavior: &BlockNewItems{MaxSize: -2},
			},
			"BlockNewItems.MaxSize must be",
		},

		{
			"BlockNewItems.MaxItems == BlockNewItems.MaxSize == -1",
			Options{
				FullBehavior: &BlockNewItems{MaxItems: -1, MaxSize: -1},
			},
			"BlockNewItems must have one of",
		},

		{
			"BlockNewItems.MaxSize > 0 && BatchSizeMax == -1",
			Options{
				FullBehavior: &BlockNewItems{MaxSize: 100},
			},
			"BlockNewItems.MaxSize may only be set",
		},
	}

	ftt.Run(`test bad option groups`, t, func(t *ftt.Test) {
		for _, options := range badOptions {
			t.Run(options.name, func(t *ftt.Test) {
				myOptions := options.options
				assert.Loosely(t, myOptions.normalize(), should.ErrLike(options.expected))
			})
		}
	})
}
