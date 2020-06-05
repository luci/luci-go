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

package cfgclient

import (
	"context"
	"testing"

	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHelpers(t *testing.T) {
	t.Parallel()

	Convey("ProjectsWithConfig works", t, func() {
		ctx := Use(context.Background(), memory.New(map[config.Set]memory.Files{
			"projects/a": {"f.cfg": ""},
			"projects/b": {},
			"projects/c": {"f.cfg": ""},
			"services/s": {"f.cfg": ""},
		}))
		p, err := ProjectsWithConfig(ctx, "f.cfg")
		So(err, ShouldBeNil)
		So(p, ShouldResemble, []string{"a", "c"})
	})

	Convey("Get works", t, func() {
		ctx := Use(context.Background(), memory.New(map[config.Set]memory.Files{
			"projects/a": {
				"cfg.text":   `blah`,
				"cfg.textpb": `name: "blah"`,
				"cfg.jsonpb": `{"name": "blah"}`,
			},
		}))

		Convey("Bytes", func() {
			dst := []byte(nil)
			meta := config.Meta{}
			So(Get(ctx, "projects/a", "cfg.text", Bytes(&dst), &meta), ShouldBeNil)
			So(dst, ShouldResemble, []byte("blah"))
			So(meta.ContentHash, ShouldNotBeBlank)
		})

		Convey("String", func() {
			dst := ""
			meta := config.Meta{}
			So(Get(ctx, "projects/a", "cfg.text", String(&dst), &meta), ShouldBeNil)
			So(dst, ShouldEqual, "blah")
			So(meta.ContentHash, ShouldNotBeBlank)
		})

		Convey("ProtoText", func() {
			dst := configPB.ProjectCfg{}
			meta := config.Meta{}
			So(Get(ctx, "projects/a", "cfg.textpb", ProtoText(&dst), &meta), ShouldBeNil)
			So(&dst, ShouldResembleProto, &configPB.ProjectCfg{Name: "blah"})
			So(meta.ContentHash, ShouldNotBeBlank)
		})

		Convey("ProtoJSON", func() {
			dst := configPB.ProjectCfg{}
			meta := config.Meta{}
			So(Get(ctx, "projects/a", "cfg.jsonpb", ProtoJSON(&dst), &meta), ShouldBeNil)
			So(&dst, ShouldResembleProto, &configPB.ProjectCfg{Name: "blah"})
			So(meta.ContentHash, ShouldNotBeBlank)
		})

		Convey("Meta only", func() {
			meta := config.Meta{}
			So(Get(ctx, "projects/a", "cfg.text", nil, &meta), ShouldBeNil)
			So(meta.ContentHash, ShouldNotBeBlank)
		})

		Convey("Presence only", func() {
			So(Get(ctx, "projects/a", "cfg.text", nil, nil), ShouldBeNil)
			So(Get(ctx, "projects/a", "missing", nil, nil), ShouldEqual, config.ErrNoConfig)
		})
	})
}
