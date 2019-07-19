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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOptionValidationGood(t *testing.T) {
	fullOptions := Options{
		ErrorFn:  func(*buffer.Batch, error) bool { return false },
		DropFn:   func(*buffer.Batch) {},
		QPSLimit: rate.NewLimiter(rate.Inf, 0),
	}

	var goodOptions = []struct {
		name     string
		options  Options
		expected Options
	}{
		{
			name:    "minimal",
			options: Options{},
			expected: Options{
				QPSLimit: rate.NewLimiter(1, 1),
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

				So(myOptions.normalize(ctx), ShouldBeNil)

				// Directly compare function pointers; ShouldResemble doesn't compare
				// them sensibly.
				if expect.ErrorFn == nil {
					So(myOptions.ErrorFn, ShouldNotBeNil)
				} else {
					So(myOptions.ErrorFn, ShouldEqual, expect.ErrorFn)
					expect.ErrorFn = nil
				}
				myOptions.ErrorFn = nil

				if expect.DropFn == nil {
					So(myOptions.DropFn, ShouldNotBeNil)
				} else {
					So(myOptions.DropFn, ShouldEqual, expect.DropFn)
					expect.DropFn = nil
				}
				myOptions.DropFn = nil

				So(myOptions, ShouldResemble, expect)
			})
		}
	})
}

func TestOptionValidationBad(t *testing.T) {
	Convey(`bad QPSLimit check`, t, func() {
		ctx := context.Background()
		opts := Options{QPSLimit: rate.NewLimiter(100, 0)}
		So(opts.normalize(ctx), ShouldErrLike, "QPSLimit has burst size < 1")
	})
}
