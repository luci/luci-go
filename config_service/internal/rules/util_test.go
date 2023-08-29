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

package rules

import (
	"testing"

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/data/stringset"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateAccess(t *testing.T) {
	t.Parallel()

	Convey("Validate access", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		Convey("not specified", func() {
			validateAccess(vctx, "")
			So(vctx.Finalize(), ShouldErrLike, `not specified`)
		})
		Convey("invalid group", func() {
			validateAccess(vctx, "group:foo^")
			So(vctx.Finalize(), ShouldErrLike, `invalid auth group: "foo^"`)
		})
		Convey("unknown identity", func() {
			validateAccess(vctx, "bad-kind:abc")
			So(vctx.Finalize(), ShouldErrLike, `invalid identity "bad-kind:abc"; reason:`)
		})
		Convey("bad email", func() {
			validateAccess(vctx, "not-a-valid-email")
			So(vctx.Finalize(), ShouldErrLike, `invalid email address:`)
		})
	})
}

func TestValidateEmail(t *testing.T) {
	t.Parallel()

	Convey("Validate email", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		Convey("not specified", func() {
			validateEmail(vctx, "")
			So(vctx.Finalize(), ShouldErrLike, `email not specified`)
		})
		Convey("invalid email", func() {
			validateEmail(vctx, "not-a-valid-email")
			So(vctx.Finalize(), ShouldErrLike, `invalid email address:`)
		})
	})
}

func TestValidateSorted(t *testing.T) {
	t.Parallel()

	Convey("Validate sorted", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		type testItem struct {
			id string
		}
		getIDFn := func(ti testItem) string { return ti.id }

		Convey("handle empty", func() {
			validateSorted[testItem](vctx, []testItem{}, "test_items", getIDFn)
			So(vctx.Finalize(), ShouldBeNil)
		})
		Convey("sorted", func() {
			validateSorted[testItem](vctx, []testItem{{"a"}, {"b"}, {"c"}}, "test_items", getIDFn)
			So(vctx.Finalize(), ShouldBeNil)
		})
		Convey("ignore empty", func() {
			validateSorted[testItem](vctx, []testItem{{"a"}, {""}, {"c"}}, "test_items", getIDFn)
			So(vctx.Finalize(), ShouldBeNil)
		})
		Convey("not sorted", func() {
			validateSorted[testItem](vctx, []testItem{{"a"}, {"c"}, {"b"}, {"d"}}, "test_items", getIDFn)
			verr := vctx.Finalize().(*validation.Error)
			So(verr.WithSeverity(validation.Warning), ShouldErrLike, `test_items are not sorted by id. First offending id: "b"`)
		})
	})
}

func TestValidateUniqueID(t *testing.T) {
	t.Parallel()

	Convey("Validate unique ID", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		Convey("empty", func() {
			validateUniqueID(vctx, "", stringset.New(0), nil)
			So(vctx.Finalize(), ShouldErrLike, `not specified`)
		})

		Convey("duplicate", func() {
			seen := stringset.New(2)
			validateUniqueID(vctx, "id", seen, nil)
			So(vctx.Finalize(), ShouldBeNil)
			validateUniqueID(vctx, "id", seen, nil)
			So(vctx.Finalize(), ShouldErrLike, `duplicate: "id"`)
		})

		Convey("apply validateIDFn", func() {
			validateUniqueID(vctx, "id", stringset.New(0), func(vctx *validation.Context, id string) {
				vctx.Errorf("bad bad bad")
			})
			So(vctx.Finalize(), ShouldErrLike, `bad bad bad`)
		})
	})
}

func TestValidateURL(t *testing.T) {
	t.Parallel()

	Convey("Validate url", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		Convey("empty", func() {
			validateURL(vctx, "")
			So(vctx.Finalize(), ShouldErrLike, `not specified`)
		})
		Convey("invalid url", func() {
			validateURL(vctx, "https://example.com\\foo")
			So(vctx.Finalize(), ShouldErrLike, `invalid url:`)
		})
		Convey("missing host name", func() {
			validateURL(vctx, "https://")
			So(vctx.Finalize(), ShouldErrLike, `hostname must be specified`)
		})
		Convey("must use https", func() {
			validateURL(vctx, "http://example.com")
			So(vctx.Finalize(), ShouldErrLike, `scheme must be "https"`)
		})
	})
}
