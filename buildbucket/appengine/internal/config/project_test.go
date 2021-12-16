// Copyright 2021 The LUCI Authors.
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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProject(t *testing.T) {
	t.Parallel()

	Convey("validate buildbucket cfg", t, func() {
		vctx := &validation.Context{
			Context: memory.Use(context.Background()),
		}
		configSet := "projects/test"
		path := "cr-buildbucket.cfg"
		settingsCfg := &pb.SettingsCfg{}
		So(SetTestSettingsCfg(vctx.Context, settingsCfg), ShouldBeNil)

		Convey("OK", func() {
			var okCfg = `
				buckets {
					name: "good.name"
					acls {
						role: WRITER
						group: "writers"
					}
				}
				buckets {
					name: "good.name2"
					acls {
						role: READER
						identity: "a@a.com"
					}
					acls {
						role: READER
						identity: "user:b@a.com"
					}
				}
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("bad proto", func() {
			content := []byte(` bad: "bad" `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "invalid BuildbucketCfg proto message")
		})

		Convey("empty cr-buildbucket.cfg", func() {
			content := []byte(` `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("fail", func() {
			var badCfg = `
				acl_sets { name: "a" }
				buckets {
					name: "a"
					acls {
						role: READER
						group: "writers"
						identity: "a@a.com"
					}
					acls {
						role: READER
					}
				}
				buckets {
					name: "a"
					acl_sets: "a"
					acls {
						role: READER
						identity: "ldap"
					}
					acls {
						role: READER
						group: ";%:"
					}
				}
				buckets {}
				buckets { name: "luci.x" }
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			So(len(ve.Errors), ShouldEqual, 9)
			So(ve.Errors[0].Error(), ShouldContainSubstring, "acl_sets is not allowed any more, use go/lucicfg")
			So(ve.Errors[1].Error(), ShouldContainSubstring, "(buckets #0 - a / acls #0): either group or identity must be set, not both")
			So(ve.Errors[2].Error(), ShouldContainSubstring, "(buckets #0 - a / acls #1): group or identity must be set")
			So(ve.Errors[3].Error(), ShouldContainSubstring, "(buckets #1 - a): duplicate bucket name \"a\"")
			So(ve.Errors[4].Error(), ShouldContainSubstring, "(buckets #1 - a / acls #0): \"ldap\" invalid: auth: bad value \"ldap\" for identity kind \"user\"")
			So(ve.Errors[5].Error(), ShouldContainSubstring, "(buckets #1 - a / acls #1): invalid group: ;%:")
			So(ve.Errors[6].Error(), ShouldContainSubstring, "(buckets #1 - a): acl_sets is not allowed any more, use go/lucicfg")
			So(ve.Errors[7].Error(), ShouldContainSubstring, "(buckets #2 - ): invalid name \"\": bucket name is not specified")
			So(ve.Errors[8].Error(), ShouldContainSubstring, "(buckets #3 - luci.x): invalid name \"luci.x\": must start with 'luci.test.' because it starts with 'luci.' and is defined in the \"test\" project")
			})


		Convey("buckets unsorted", func() {
			badCfg := `
				buckets { name: "c" }
				buckets { name: "b" }
				buckets { name: "a" }
			`
			So(validateProjectCfg(vctx, configSet, path, []byte(badCfg)), ShouldBeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			So(ok, ShouldEqual, true)
			warnings := ve.WithSeverity(validation.Warning).(errors.MultiError)
			So(warnings[0].Error(), ShouldContainSubstring, "bucket \"b\" out of order")
			So(warnings[1].Error(), ShouldContainSubstring, "bucket \"a\" out of order")
		})
	})

	Convey("validate project_config.Swarming", t, func() {
		// TODO (yuanjunh): fill in tests after validateBuilderCfg is done.
	})
}
