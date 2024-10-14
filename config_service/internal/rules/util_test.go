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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateAccess(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate access", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		t.Run("not specified", func(t *ftt.Test) {
			validateAccess(vctx, "")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`not specified`))
		})
		t.Run("invalid group", func(t *ftt.Test) {
			validateAccess(vctx, "group:foo^")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid auth group: "foo^"`))
		})
		t.Run("unknown identity", func(t *ftt.Test) {
			validateAccess(vctx, "bad-kind:abc")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid identity "bad-kind:abc"; reason:`))
		})
		t.Run("bad email", func(t *ftt.Test) {
			validateAccess(vctx, "not-a-valid-email")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid email address:`))
		})
	})
}

func TestValidateEmail(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate email", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		t.Run("not specified", func(t *ftt.Test) {
			validateEmail(vctx, "")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`email not specified`))
		})
		t.Run("invalid email", func(t *ftt.Test) {
			validateEmail(vctx, "not-a-valid-email")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid email address:`))
		})
	})
}

func TestValidateSorted(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate sorted", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		type testItem struct {
			id string
		}
		getIDFn := func(ti testItem) string { return ti.id }

		t.Run("handle empty", func(t *ftt.Test) {
			validateSorted[testItem](vctx, []testItem{}, "test_items", getIDFn)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
		t.Run("sorted", func(t *ftt.Test) {
			validateSorted[testItem](vctx, []testItem{{"a"}, {"b"}, {"c"}}, "test_items", getIDFn)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
		t.Run("ignore empty", func(t *ftt.Test) {
			validateSorted[testItem](vctx, []testItem{{"a"}, {""}, {"c"}}, "test_items", getIDFn)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
		t.Run("not sorted", func(t *ftt.Test) {
			validateSorted[testItem](vctx, []testItem{{"a"}, {"c"}, {"b"}, {"d"}}, "test_items", getIDFn)
			verr := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, verr.WithSeverity(validation.Warning), should.ErrLike(`test_items are not sorted by id. First offending id: "b"`))
		})
	})
}

func TestValidateUniqueID(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate unique ID", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		t.Run("empty", func(t *ftt.Test) {
			validateUniqueID(vctx, "", stringset.New(0), nil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`not specified`))
		})

		t.Run("duplicate", func(t *ftt.Test) {
			seen := stringset.New(2)
			validateUniqueID(vctx, "id", seen, nil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
			validateUniqueID(vctx, "id", seen, nil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`duplicate: "id"`))
		})

		t.Run("apply validateIDFn", func(t *ftt.Test) {
			validateUniqueID(vctx, "id", stringset.New(0), func(vctx *validation.Context, id string) {
				vctx.Errorf("bad bad bad")
			})
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`bad bad bad`))
		})
	})
}

func TestValidateURL(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate url", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}

		t.Run("empty", func(t *ftt.Test) {
			validateURL(vctx, "")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`not specified`))
		})
		t.Run("invalid url", func(t *ftt.Test) {
			validateURL(vctx, "https://example.com\\foo")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid url:`))
		})
		t.Run("missing host name", func(t *ftt.Test) {
			validateURL(vctx, "https://")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`hostname must be specified`))
		})
		t.Run("must use https", func(t *ftt.Test) {
			validateURL(vctx, "http://example.com")
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`scheme must be "https"`))
		})
	})
}
