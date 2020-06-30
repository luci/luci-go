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

package ttq

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOptions(t *testing.T) {
	t.Parallel()

	Convey("Validate", t, func() {
		valid := Options{
			BaseURL: "https://example.com/internal/ttq",
			Queue:   "projects/example-project/locations/us-central1/queues/ttq",
		}
		Convey("Valid", func() {
			So(valid.Validate(), ShouldBeNil)
			So(valid.ScanInterval, ShouldEqual, time.Minute)
			So(valid.Shards, ShouldEqual, 16)
		})
		Convey("Allow non default", func() {
			valid.Shards = 17
			So(valid.Validate(), ShouldBeNil)
			So(valid.Shards, ShouldEqual, 17)
		})

		Convey("BaseURL", func() {
			opts := valid
			opts.BaseURL = ""
			So(opts.Validate(), ShouldErrLike, "BaseURL is required")
			opts.BaseURL = "https://example.com/internal/ttq/"
			So(opts.Validate(), ShouldNotBeNil)
			opts.BaseURL = "https://example.com/internal/ttq?blah=true"
			So(opts.Validate(), ShouldNotBeNil)
			opts.BaseURL = "https://example.com"
			So(opts.Validate(), ShouldNotBeNil)
		})

		Convey("Queue", func() {
			opts := valid
			opts.Queue = ""
			So(opts.Validate(), ShouldErrLike, "Queue is required")
			opts.Queue = "short-gae-classic-name"
			So(opts.Validate(), ShouldNotBeNil)
			// But can't catch all errors.
			opts.Queue = "projects/404/locations/moon/queues/invalidChars"
			So(opts.Validate(), ShouldBeNil)
		})

		Convey("ScanInterval", func() {
			opts := valid
			opts.ScanInterval = time.Minute - time.Second
			So(opts.Validate(), ShouldErrLike, "must be at least 1 Minute")

			opts.ScanInterval = 5*time.Minute + 7*time.Second
			So(opts.Validate(), ShouldBeNil)
			So(opts.ScanInterval, ShouldEqual, 5*time.Minute)
		})
	})
}
