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

package projectconfig

import (
	"testing"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		Convey("Validation tests", func() {
			ctx := &validation.Context{
				Context: c,
			}
			configSet := "projects/foobar"
			path := "${appid}.cfg"
			Convey("Load a bad config", func() {
				content := []byte(badCfg)
				validateProjectCfg(ctx, configSet, path, content)
				So(ctx.Finalize().Error(), ShouldResemble, "in <unspecified file>: line 4: unknown field name \"\" in projectconfigpb.Header")
			})
			Convey("Load another bad config", func() {
				content := []byte(badCfg2)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 14)
				So(ve.Errors[0], ShouldErrLike, "duplicate header id")
				So(ve.Errors[1], ShouldErrLike, "missing id")
				So(ve.Errors[2], ShouldErrLike, "missing manifest name")
				So(ve.Errors[3], ShouldErrLike, "missing repo url")
				So(ve.Errors[4], ShouldErrLike, "missing ref")
				So(ve.Errors[5], ShouldErrLike, "header non-existent not defined")
			})
			Convey("Load yet another bad config", func() {
				content := []byte(badCfg3)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 1)
				So(ve.Errors[0], ShouldErrLike, "id can not contain '/'")
			})
			Convey("Load a bad config due to malformed external consoles", func() {
				content := []byte(badCfg4)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 7)
				So(ve.Errors[0], ShouldErrLike, "missing external project")
				So(ve.Errors[1], ShouldErrLike, "missing external console id")
				So(ve.Errors[2], ShouldErrLike, "repo url found in external console")
				So(ve.Errors[3], ShouldErrLike, "refs found in external console")
				So(ve.Errors[4], ShouldErrLike, "manifest name found in external console")
				So(ve.Errors[5], ShouldErrLike, "builders found in external console")
				So(ve.Errors[6], ShouldErrLike, "header found in external console")
			})
			Convey("Load bad config due to console builder's definitions", func() {
				content := []byte(badConsoleCfg)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 2)
				So(ve.Errors[0], ShouldErrLike, "name must be non-empty")
				So(ve.Errors[1], ShouldErrLike, "the string is not a valid legacy builder ID")
			})
			Convey("Load bad config due to metadata config", func() {
				content := []byte(badCfg5)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 2)
				So(ve.Errors[0], ShouldErrLike, "schema): does not match")
				So(ve.Errors[1], ShouldErrLike, "path): unspecified")
			})
			Convey("Load a good config", func() {
				content := []byte(fooCfg)
				validateProjectCfg(ctx, configSet, path, content)
				So(ctx.Finalize(), ShouldBeNil)
			})
		})
	})
}

var badCfg = `
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
`

var badCfg2 = `
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
}
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
}
consoles {
	header_id: "non-existent"
}
consoles {
	id: "foo"
}
consoles {
	id: "foo"
}
logo_url: "badurl"
`

var badCfg3 = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "with/slash"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "regexp:refs/heads/also-ok"
	manifest_name: "REVISION"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
`

var badCfg4 = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "missing-external-proj"
	external_id: "console"
}
consoles: {
	id: "missing-external-id"
	external_project: "proj"
}
consoles: {
	id: "external-console-extra-fields"
	external_project: "proj"
	external_id: "console"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "regexp:refs/heads/also-ok"
	manifest_name: "REVISION"
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
`

var badCfg5 = `
metadata_config: {
	test_metadata_properties: {
		schema: "package"
		display_items: {
			display_name: "owners"
			path: ""
		}
	}
}`
