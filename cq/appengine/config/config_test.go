// Copyright 2018 The LUCI Authors.
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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidationRules(t *testing.T) {
	t.Parallel()

	Convey("Validation Rules", t, func() {
		patterns, err := validation.Rules.ConfigPatterns(context.Background())
		So(err, ShouldBeNil)
		So(len(patterns), ShouldEqual, 2)
		Convey("project-scope cq.cfg", func() {
			So(patterns[0].ConfigSet.Match("projects/xyz"), ShouldBeTrue)
			So(patterns[0].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeFalse)
			So(patterns[0].Path.Match("cq.cfg"), ShouldBeTrue)
		})
		Convey("legacy ref-scope cq.cfg", func() {
			So(patterns[1].ConfigSet.Match("projects/xyz"), ShouldBeFalse)
			So(patterns[1].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeTrue)
			So(patterns[1].Path.Match("cq.cfg"), ShouldBeTrue)
		})
	})
}

func TestValidationLegacy(t *testing.T) {
	t.Parallel()

	Convey("Validate Legacy Config", t, func() {
		c := gaetesting.TestingContext()
		vctx := &validation.Context{Context: c}
		configSet := "projects/foo/refs/heads/master"
		path := "cq.cfg"
		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateRefCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})
		Convey("Loading OK proto", func() {
			content := []byte(` version: 2 `)
			So(validateRefCfg(vctx, configSet, path, content), ShouldErrLike, "not implemented")
		})
	})
}

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("Validate Config", t, func() {
		c := gaetesting.TestingContext()
		vctx := &validation.Context{Context: c}
		configSet := "projects/foo"
		path := "cq.cfg"
		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})
		Convey("Loading OK proto", func() {
			content := []byte(` draining_start_time: "2017-12-23T15:47:58Z" `)
			So(validateProjectCfg(vctx, configSet, path, content), ShouldErrLike, "not implemented")
		})
	})
}
