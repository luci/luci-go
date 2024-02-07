// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTryjob(t *testing.T) {
	t.Parallel()

	Convey("Tryjob", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		Convey("Populate RetentionKey", func() {
			epoch := datastore.RoundTime(ct.Clock.Now().UTC())
			tj := &Tryjob{
				ID:               1,
				EntityUpdateTime: epoch,
			}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			tj = &Tryjob{ID: 1}
			So(datastore.Get(ctx, tj), ShouldBeNil)
			So(tj.RetentionKey, ShouldEqual, fmt.Sprintf("01/%010d", epoch.Unix()))
		})
		Convey("Fill in EntityUpdateTime if it's missing", func() {
			tj := &Tryjob{ID: 1}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			tj = &Tryjob{ID: 1}
			So(datastore.Get(ctx, tj), ShouldBeNil)
			So(tj.EntityUpdateTime.IsZero(), ShouldBeFalse)
			So(tj.RetentionKey, ShouldNotBeEmpty)
		})
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	Convey("Delete", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		tj := MustBuildbucketID("bb.example.com", 10).MustCreateIfNotExists(ctx)

		Convey("Works", func() {
			So(CondDelete(ctx, tj.ID, tj.EVersion), ShouldBeNil)
			So(datastore.Get(ctx, &Tryjob{ID: tj.ID}), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, &tryjobMap{ExternalID: tj.ExternalID}), ShouldErrLike, datastore.ErrNoSuchEntity)
			Convey("Can handle deleted entity", func() {
				// delete the same entity again
				So(CondDelete(ctx, tj.ID, tj.EVersion), ShouldBeNil)
			})
		})
		Convey("Works without external ID", func() {
			prevExternalID := tj.ExternalID
			tj.ExternalID = "" // not valid, but just for testing purpose.
			So(datastore.Put(ctx, tj), ShouldBeNil)
			So(CondDelete(ctx, tj.ID, tj.EVersion), ShouldBeNil)
			So(datastore.Get(ctx, &Tryjob{ID: tj.ID}), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, &tryjobMap{ExternalID: prevExternalID}), ShouldBeNil)
		})
		Convey("Returns error for invalid input", func() {
			So(CondDelete(ctx, tj.ID, 0), ShouldErrLike, "expected EVersion must be larger than 0")
		})
		Convey("Returns error if condition mismatch", func() {
			tj.EVersion = 15
			So(datastore.Put(ctx, tj), ShouldBeNil)
			So(CondDelete(ctx, tj.ID, tj.EVersion-1), ShouldErrLike, "request to delete tryjob")
		})
	})
}

func TestCLPatchset(t *testing.T) {
	t.Parallel()

	Convey("CLPatchset works", t, func() {
		var cl common.CLID
		var ps int32
		Convey("with max values", func() {
			cl, ps = 0x7fff_ffff_ffff_ffff, 0x7fff_ffff
		})
		Convey("with min values", func() {
			cl, ps = 0, 0
		})
		clps := MakeCLPatchset(cl, ps)
		parsedCl, parsedPs, err := clps.Parse()
		So(err, ShouldBeNil)
		So(parsedCl, ShouldEqual, cl)
		So(parsedPs, ShouldEqual, ps)
	})
	Convey("CLPatchset fails", t, func() {
		Convey("with bad number of values", func() {
			_, _, err := CLPatchset("1/1").Parse()
			So(err, ShouldErrLike, "CLPatchset in unexpected format")
		})
		Convey("with bad version", func() {
			_, _, err := CLPatchset("8/8/8").Parse()
			So(err, ShouldErrLike, "unsupported version")
		})
		Convey("with bad CLID", func() {
			_, _, err := CLPatchset("1/4d35683b24371b75c5f3fda0d48796638dc0d695/7").Parse()
			So(err, ShouldErrLike, "clid segment in unexpected format")
			_, _, err = CLPatchset("1/gerrit/chromium-review.googlesource.com/3530834/7").Parse()
			So(err, ShouldErrLike, "unexpected format")
		})
		Convey("with bad patchset", func() {
			_, _, err := CLPatchset("1/1/ps1").Parse()
			So(err, ShouldErrLike, "patchset segment in unexpected format")
		})
	})
}
