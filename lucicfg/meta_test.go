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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMeta(t *testing.T) {
	t.Parallel()

	Convey("Starlark setters", t, func() {
		m := Meta{}

		Convey("Success", func() {
			// String.
			So(m.setField("config_service_host", starlark.String("boo")), ShouldBeNil)
			So(m.ConfigServiceHost, ShouldEqual, "boo")

			// Bool.
			So(m.setField("fail_on_warnings", starlark.Bool(true)), ShouldBeNil)
			So(m.FailOnWarnings, ShouldBeTrue)

			// []string.
			So(m.setField("tracked_files", starlark.NewList([]starlark.Value{
				starlark.String("t1"), starlark.String("t2"),
			})), ShouldBeNil)
			So(m.TrackedFiles, ShouldResemble, []string{"t1", "t2"})

			// List of touched fields was updated.
			So(touched(&m), ShouldResemble, []string{
				"config_service_host",
				"fail_on_warnings",
				"tracked_files",
			})
		})

		Convey("Errors", func() {
			So(m.setField("unknown", starlark.None), ShouldErrLike, `set_meta: no such meta key "unknown"`)
			So(m.setField("config_service_host", starlark.None), ShouldErrLike, `set_meta: got NoneType, expecting string`)
			So(m.setField("fail_on_warnings", starlark.None), ShouldErrLike, `set_meta: got NoneType, expecting bool`)
			So(m.setField("tracked_files", starlark.None), ShouldErrLike, `set_meta: got NoneType, expecting an iterable`)
			So(m.setField("tracked_files", starlark.NewList([]starlark.Value{starlark.None})), ShouldErrLike, `set_meta: got NoneType, expecting string`)
		})
	})

	Convey("Flag setters", t, func() {
		fs := flag.FlagSet{}
		m := Meta{}

		m.AddFlags(&fs)

		So(fs.Parse([]string{
			"-config-service-host", "boo",
			"-tracked-files", "a,b,c",
			"-fail-on-warnings",
		}), ShouldBeNil)

		So(m.ConfigServiceHost, ShouldEqual, "boo")
		So(m.TrackedFiles, ShouldResemble, []string{"a", "b", "c"})
		So(m.FailOnWarnings, ShouldBeTrue)

		So(touched(&m), ShouldResemble, []string{
			"config_service_host",
			"fail_on_warnings",
			"tracked_files",
		})
	})

	Convey("Merging", t, func() {
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

		So(l, ShouldResemble, Meta{
			ConfigServiceHost: "l1", // wasn't touched in r
			FailOnWarnings:    false,
			TrackedFiles:      []string{"r3"},
		})
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
