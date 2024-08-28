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

package lucicfg

import (
	"flag"
	"sort"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMeta(t *testing.T) {
	t.Parallel()

	ftt.Run("Starlark setters", t, func(t *ftt.Test) {
		m := Meta{}

		t.Run("Success", func(t *ftt.Test) {
			// String.
			assert.Loosely(t, m.setField("config_service_host", starlark.String("boo")), should.BeNil)
			assert.Loosely(t, m.ConfigServiceHost, should.Equal("boo"))

			// Bool.
			assert.Loosely(t, m.setField("fail_on_warnings", starlark.Bool(true)), should.BeNil)
			assert.Loosely(t, m.FailOnWarnings, should.BeTrue)

			// []string.
			assert.Loosely(t, m.setField("tracked_files", starlark.NewList([]starlark.Value{
				starlark.String("t1"), starlark.String("t2"),
			})), should.BeNil)
			assert.Loosely(t, m.TrackedFiles, should.Resemble([]string{"t1", "t2"}))

			// List of touched fields was updated.
			assert.Loosely(t, touched(&m), should.Resemble([]string{
				"config_service_host",
				"fail_on_warnings",
				"tracked_files",
			}))
		})

		t.Run("Errors", func(t *ftt.Test) {
			assert.Loosely(t, m.setField("unknown", starlark.None), should.ErrLike(`set_meta: no such meta key "unknown"`))
			assert.Loosely(t, m.setField("config_service_host", starlark.None), should.ErrLike(`set_meta: got NoneType, expecting string`))
			assert.Loosely(t, m.setField("fail_on_warnings", starlark.None), should.ErrLike(`set_meta: got NoneType, expecting bool`))
			assert.Loosely(t, m.setField("tracked_files", starlark.None), should.ErrLike(`set_meta: got NoneType, expecting an iterable`))
			assert.Loosely(t, m.setField("tracked_files", starlark.NewList([]starlark.Value{starlark.None})), should.ErrLike(`set_meta: got NoneType, expecting string`))
		})
	})

	ftt.Run("Flag setters", t, func(t *ftt.Test) {
		fs := flag.FlagSet{}
		m := Meta{}

		m.AddFlags(&fs)

		assert.Loosely(t, fs.Parse([]string{
			"-config-service-host", "boo",
			"-tracked-files", "a,b,c",
			"-fail-on-warnings",
		}), should.BeNil)

		assert.Loosely(t, m.ConfigServiceHost, should.Equal("boo"))
		assert.Loosely(t, m.TrackedFiles, should.Resemble([]string{"a", "b", "c"}))
		assert.Loosely(t, m.FailOnWarnings, should.BeTrue)

		assert.Loosely(t, touched(&m), should.Resemble([]string{
			"config_service_host",
			"fail_on_warnings",
			"tracked_files",
		}))
	})

	ftt.Run("Merging", t, func(t *ftt.Test) {
		l := Meta{
			ConfigServiceHost: "l1",
			FailOnWarnings:    true,
			TrackedFiles:      []string{"l3"},
		}
		r := Meta{
			ConfigServiceHost: "r1",
			FailOnWarnings:    false,
			TrackedFiles:      []string{"r3"},
		}

		r.touch(&r.FailOnWarnings)
		r.touch(&r.TrackedFiles)

		l.PopulateFromTouchedIn(&r)

		assert.Loosely(t, l, should.Resemble(Meta{
			ConfigServiceHost: "l1", // wasn't touched in r
			FailOnWarnings:    false,
			TrackedFiles:      []string{"r3"},
		}))
	})
}

func touched(m *Meta) (touched []string) {
	for k := range m.fieldsMap() {
		if m.WasTouched(k) {
			touched = append(touched, k)
		}
	}
	sort.Strings(touched)
	return
}
