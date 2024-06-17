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

package validation

import (
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidationRules(t *testing.T) {
	t.Parallel()

	Convey("Validation Rules", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "luci-change-verifier")
		r := validation.NewRuleSet()
		r.Vars.Register("appid", func(context.Context) (string, error) { return "luci-change-verifier", nil })

		addRules(r)

		patterns, err := r.ConfigPatterns(ctx)
		So(err, ShouldBeNil)
		So(len(patterns), ShouldEqual, 2)
		Convey("project-scope cq.cfg", func() {
			So(patterns[0].ConfigSet.Match("projects/xyz"), ShouldBeTrue)
			So(patterns[0].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeFalse)
			So(patterns[0].Path.Match("commit-queue.cfg"), ShouldBeTrue)
		})
		Convey("service-scope listener-settings.cfg", func() {
			So(patterns[1].ConfigSet.Match("services/luci-change-verifier"), ShouldBeTrue)
			So(patterns[1].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeFalse)
			So(patterns[1].Path.Match("listener-settings.cfg"), ShouldBeTrue)
		})
		Convey("Dev", func() {
			ctx = memory.UseWithAppID(context.Background(), "luci-change-verifier-dev")
			patterns, err := r.ConfigPatterns(ctx)
			So(err, ShouldBeNil)
			So(patterns[0].Path.Match("commit-queue-dev.cfg"), ShouldBeTrue)
		})
	})
}

func mustHaveOnlySeverity(err error, severity validation.Severity) error {
	So(err, ShouldNotBeNil)
	for _, e := range err.(*validation.Error).Errors {
		s, ok := validation.SeverityTag.In(e)
		So(ok, ShouldBeTrue)
		So(s, ShouldEqual, severity)
	}
	return err
}

func mustWarn(err error) error {
	return mustHaveOnlySeverity(err, validation.Warning)
}

func mustError(err error) error {
	return mustHaveOnlySeverity(err, validation.Blocking)
}
