// Copyright 2023 The LUCI Authors.
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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskRequest(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := memory.Use(context.Background())

		Convey("Good task ID: TaskResultSummary", func() {
			key, err := TaskRequestKey(ctx, "60b2ed0a43023110")
			So(err, ShouldBeNil)
			So(key.IntID(), ShouldEqual, 8787878774240697582)
		})

		Convey("Good task ID: TaskRunResult", func() {
			key, err := TaskRequestKey(ctx, "60b2ed0a43023111")
			So(err, ShouldBeNil)
			So(key.IntID(), ShouldEqual, 8787878774240697582)
		})

		Convey("Bad hex", func() {
			_, err := TaskRequestKey(ctx, "60b2ed0a4302311z")
			So(err, ShouldErrLike, "bad task ID: bad lowercase hex string")
		})

		Convey("Empty", func() {
			_, err := TaskRequestKey(ctx, "")
			So(err, ShouldErrLike, "bad task ID: too small")
		})

		Convey("Overflow", func() {
			_, err := TaskRequestKey(ctx, "ff60b2ed0a4302311f")
			So(err, ShouldErrLike, "value out of range")
		})
	})
}
