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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequence(t *testing.T) {
	t.Parallel()

	Convey("GenerateSequenceNumbers", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("not found", func() {
			Convey("zero", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 0)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 1)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 0)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 1)
			})

			Convey("one", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 1)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 1)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 1)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 2)
			})

			Convey("many", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 10)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 1)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 10)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 11)
			})
		})

		Convey("found", func() {
			So(datastore.Put(ctx, &NumberSequence{
				ID:   "seq",
				Next: 2,
			}), ShouldBeNil)

			Convey("zero", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 0)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 2)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 0)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 2)
			})

			Convey("one", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 1)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 2)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 1)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 3)
			})

			Convey("many", func() {
				seq, err := GenerateSequenceNumbers(ctx, "seq", 10)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 2)

				seq, err = GenerateSequenceNumbers(ctx, "seq", 10)
				So(err, ShouldBeNil)
				So(seq, ShouldEqual, 12)
			})
		})
	})
}
