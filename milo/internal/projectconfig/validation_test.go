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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Environment", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		t.Run("Validation tests", func(t *ftt.Test) {
			ctx := &validation.Context{
				Context: c,
			}
			configSet := "projects/foobar"
			path := "${appid}.cfg"
			t.Run("Load a bad config", func(t *ftt.Test) {
				content := []byte(badCfg)
				validateProjectCfg(ctx, configSet, path, content)
				assert.Loosely(t, ctx.Finalize().Error(), should.Match("in <unspecified file>: line 4: unknown field name \"\" in projectconfigpb.Header"))
			})
			t.Run("Load another bad config", func(t *ftt.Test) {
				content := []byte(badCfg2)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(14))
				assert.Loosely(t, ve.Errors[0], should.ErrLike("duplicate header id"))
				assert.Loosely(t, ve.Errors[1], should.ErrLike("missing id"))
				assert.Loosely(t, ve.Errors[2], should.ErrLike("missing manifest name"))
				assert.Loosely(t, ve.Errors[3], should.ErrLike("missing repo url"))
				assert.Loosely(t, ve.Errors[4], should.ErrLike("missing ref"))
				assert.Loosely(t, ve.Errors[5], should.ErrLike("header non-existent not defined"))
			})
			t.Run("Load yet another bad config", func(t *ftt.Test) {
				content := []byte(badCfg3)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0], should.ErrLike("id can not contain '/'"))
			})
			t.Run("Load a bad config due to malformed external consoles", func(t *ftt.Test) {
				content := []byte(badCfg4)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(7))
				assert.Loosely(t, ve.Errors[0], should.ErrLike("missing external project"))
				assert.Loosely(t, ve.Errors[1], should.ErrLike("missing external console id"))
				assert.Loosely(t, ve.Errors[2], should.ErrLike("repo url found in external console"))
				assert.Loosely(t, ve.Errors[3], should.ErrLike("refs found in external console"))
				assert.Loosely(t, ve.Errors[4], should.ErrLike("manifest name found in external console"))
				assert.Loosely(t, ve.Errors[5], should.ErrLike("builders found in external console"))
				assert.Loosely(t, ve.Errors[6], should.ErrLike("header found in external console"))
			})
			t.Run("Load bad config due to console builder's definitions", func(t *ftt.Test) {
				content := []byte(badConsoleCfg)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(3))
				assert.Loosely(t, ve.Errors[0], should.ErrLike("name must be non-empty or id must be set"))
				assert.Loosely(t, ve.Errors[1], should.ErrLike("the string is not a valid legacy builder ID"))
				assert.Loosely(t, ve.Errors[2], should.ErrLike("id: project must match"))
			})
			t.Run("Load bad config due to metadata config", func(t *ftt.Test) {
				content := []byte(badCfg5)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(2))
				assert.Loosely(t, ve.Errors[0], should.ErrLike("schema): does not match"))
				assert.Loosely(t, ve.Errors[1], should.ErrLike("path): unspecified"))
			})
			t.Run("Load a good config", func(t *ftt.Test) {
				content := []byte(fooCfg)
				validateProjectCfg(ctx, configSet, path, content)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
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
