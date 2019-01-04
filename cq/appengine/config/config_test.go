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

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config/validation"
	v2 "go.chromium.org/luci/cq/api/config/v2"

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
			So(validateRef(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})
		Convey("Loading OK config", func() {
			content := []byte(`
				version: 1
				gerrit {}
				git_repo_url: "https://x.googlesource.com/re/po.git"
				verifiers {
					gerrit_cq_ability { committer_list: "blah" }
				}
			`)
			So(validateRef(vctx, configSet, path, content), ShouldBeNil)
			err := vctx.Finalize()
			So(err, ShouldBeNil)
		})
		// The rest of legacy config validation tests are in legacy_test.go.
	})
}

const validConfigTextPB = `
  draining_start_time: "2017-12-23T15:47:58Z"
  cq_status_host: "example.com"
  submit_options {
    max_burst: 2
  	burst_delay { seconds: 120 }
  }
  config_groups {
    # TODO(tandrii): finish valid config.
  }
`

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("Validate Config", t, func() {
		c := gaetesting.TestingContext()
		vctx := &validation.Context{Context: c}
		configSet := "projects/foo"
		path := "cq.cfg"

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateProject(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		// It's easier to manipulate Go struct than text.
		cfg := v2.Config{}
		So(proto.UnmarshalText(validConfigTextPB, &cfg), ShouldBeNil)

		Convey("OK", func() {
			Convey("good proto, good config", func() {
				So(validateProject(vctx, configSet, path, []byte(validConfigTextPB)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("good config", func() {
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Top-level config", func() {
			Convey("Top level opts can be omitted", func() {
				cfg.DrainingStartTime = ""
				cfg.CqStatusHost = ""
				cfg.SubmitOptions = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("Bad draining time", func() {
				cfg.DrainingStartTime = "meh"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "failed to parse draining_start_time \"meh\" as RFC3339 format")
			})
			Convey("Bad cq_status_host", func() {
				cfg.CqStatusHost = "h://@test:123//not//://@adsfhost."
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("cq_status_host not just host", func() {
				cfg.CqStatusHost = "example.com/path#fragment"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, `should be just a host "example.com"`)
			})
			Convey("Bad max_burst", func() {
				cfg.SubmitOptions.MaxBurst = -1
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("Bad burst_delay ", func() {
				cfg.SubmitOptions.BurstDelay.Seconds = -1
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
		})
	})
}
