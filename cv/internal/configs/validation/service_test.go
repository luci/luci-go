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
	"strings"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMigrationConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Config", t, func() {
		vctx := &validation.Context{Context: ctx}
		configSet := "services/commit-queue"
		path := "migration-settings.cfg"

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateMigrationSettings(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		const okConfig = `
			# Realistic config.
			api_hosts {
			  host: "luci-change-verifier-dev.appspot.com"
			  project_regexp: "cq-test"
			}

			api_hosts {
			  host: "luci-change-verifier.appspot.com"
			  project_regexp: "infra(-internal)?"
			  project_regexp_exclude: "cq-test.+"
			}

			use_cv_start_message {
			  project_regexp: "cq-test.+"
			  project_regexp_exclude: "cq-test-bad"
			}
		`

		Convey("OK", func() {
			Convey("fully loaded", func() {
				So(validateMigrationSettings(vctx, configSet, path, []byte(okConfig)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("empty", func() {
				So(validateMigrationSettings(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Catches regexp bugs", func() {
			badConfig := strings.Replace(okConfig, `project_regexp_exclude: "cq-test-bad"`,
				`project_regexp_exclude: "(where is closing bracket?"`, 1)
			So(validateMigrationSettings(vctx, configSet, path, []byte(badConfig)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "error parsing regexp")
		})
	})
}

func TestListenerConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Config", t, func() {
		vctx := &validation.Context{Context: ctx}
		configSet := "services/luci-change-verifier"
		path := "listener-settings.cfg"

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateListenerSettings(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		Convey("OK", func() {
			cfg := []byte(`
				gerrit_subscriptions {
					host: "chromium-review.googlesource.com"
					receive_settings: {
						num_goroutines: 100
						max_outstanding_messages: 5000
					}
				}
				gerrit_subscriptions {
					host: "pigweed-review.googlesource.com"
					subscription_id: "pigweed_gerrit"
					receive_settings: {
						num_goroutines: 100
						max_outstanding_messages: 5000
					}
				}
			`)
			Convey("fully loaded", func() {
				So(validateListenerSettings(vctx, configSet, path, []byte(cfg)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("empty", func() {
				So(validateListenerSettings(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Fail", func() {
			Convey("Duplicate subscription ID", func() {
				cfg := []byte(`
					gerrit_subscriptions {
						host: "example.org"
					}
					gerrit_subscriptions {
						host: "example2.org"
						subscription_id: "example.org"
					}
				`)
				So(validateListenerSettings(vctx, configSet, path, []byte(cfg)), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "duplicate subscription ID")
			})

			Convey("invalid enabled_project_regexps", func() {
				cfg := []byte(`
					enabled_project_regexps: "(123"
				`)
				So(validateListenerSettings(vctx, configSet, path, []byte(cfg)), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "missing closing")
			})

			Convey("invalid disabled_project_regexps", func() {
				cfg := []byte(`
					disabled_project_regexps: "(123"
				`)
				So(validateListenerSettings(vctx, configSet, path, []byte(cfg)), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "missing closing")
			})
		})
	})
}
