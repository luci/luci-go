// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/luci_notify/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey(`Test Environment for validateProjectConfig`, t, func() {
		testValidation := func(env, config, expectFormat string, expectArgs ...interface{}) {
			Convey(env, func() {
				cfg, err := testutil.ParseProjectConfig(config)
				So(err, ShouldBeNil)
				ctx := &validation.Context{Context: context.Background()}
				validateProjectConfig(ctx, cfg)
				err = ctx.Finalize()
				if expectFormat == "" {
					So(err, assertions.ShouldErrLike)
					return
				}
				expect := fmt.Sprintf(expectFormat, expectArgs...)
				So(err, assertions.ShouldErrLike, expect)
			})
		}
		testValidation(`empty`, ``, "")

		testValidation(`builder missing name`, `
			notifiers {
				name: "good-name"
				builders {
					bucket: "test.bucket"
				}
			}`,
			requiredFieldError, "name")

		testValidation(`builder missing bucket`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
				}
			}`,
			requiredFieldError, "bucket")

		testValidation(`builder bad repo`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
					repository: "bad://x.notgooglesource.com/+/hello"
				}
			}`,
			badRepoURLError, "bad://x.notgooglesource.com/+/hello")

		testValidation(`bad email address`, `
			notifiers {
				name: "good-name"
				notifications {
					on_change: true
					email {
						recipients: "@@@@@"
					}
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			badEmailError, "@@@@@")

		testValidation(`duplicate builders in notifier`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			duplicateBuilderError, "test.bucket/i-am-a-builder")

		testValidation(`duplicate builders in project`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}
			notifiers {
				name: "good-name-again"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			duplicateBuilderError, "test.bucket/i-am-a-builder")

		testValidation(`different bucketname same builders OK`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket3"
				}
			}
			notifiers {
				name: "good-name-again"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket2"
				}
			}`,
			"", "")
	})

	Convey(`Test Environment for validateSettings`, t, func() {
		testValidation := func(env, config, expectFormat string, expectArgs ...interface{}) {
			Convey(env, func() {
				cfg, err := testutil.ParseSettings(config)
				So(err, ShouldBeNil)
				ctx := &validation.Context{Context: context.Background()}
				ctx.SetFile("settings.cfg")
				validateSettings(ctx, cfg)
				err = ctx.Finalize()
				if expectFormat == "" {
					So(err, assertions.ShouldErrLike)
					return
				}
				expect := fmt.Sprintf(expectFormat, expectArgs...)
				So(err, assertions.ShouldErrLike, expect)
			})
		}
		testValidation(`empty`, ``, requiredFieldError, "milo_host")
		testValidation(`bad hostname`, `milo_host: "9mNRn29%^^%#"`, invalidFieldError, "milo_host")
		testValidation(`good`, `milo_host: "luci-milo.example.com"`, "")
	})

	Convey("email template filename validation", t, func() {
		ctx := &validation.Context{Context: gaetesting.TestingContextWithAppID("luci-notify")}
		validFileContent := []byte("a\n\nb")

		Convey("valid", func() {
			validateEmailTemplateFile(ctx, "projects/x", "luci-notify/email-templates/a.template", validFileContent)
			So(ctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid char", func() {
			validateEmailTemplateFile(ctx, "projects/x", "luci-notify/email-templates/A.template", validFileContent)
			So(ctx.Finalize(), ShouldErrLike, "does not match")
		})
	})
}
