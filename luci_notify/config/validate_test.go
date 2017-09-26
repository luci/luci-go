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
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/luci_notify/testutil"
)

func TestValidation(t *testing.T) {
	Convey(`Test Environment for validateProjectConfig`, t, func() {
		testValidation := func(env, config, expectFormat string, expectArgs ...interface{}) {
			Convey(env, func() {
				cfg, err := testutil.ParseProjectConfig(config)
				So(err, ShouldBeNil)
				err = validateProjectConfig("projects/test", cfg)
				if expectFormat == "" {
					So(err, assertions.ShouldErrLike)
					return
				}
				expect := fmt.Sprintf(expectFormat, expectArgs...)
				So(err, assertions.ShouldErrLike, expect)
			})
		}
		testValidation(`empty`, ``, "")

		testValidation(`notifier missing name`, `notifiers {}`, requiredFieldError, "name")

		testValidation(`notifier bad name`, `
			notifiers {
				name: "A_fnbA*G2n"
			}`,
			invalidFieldError, "name")

		testValidation(`notifier dup name`, `
			notifiers {
				name: "good-name"
			}
			notifiers {
				name: "good-name"
			}`,
			uniqueFieldError, "name", "project")

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
	})

	Convey(`Test Environment for validateSettings`, t, func() {
		testValidation := func(env, config, expectFormat string, expectArgs ...interface{}) {
			Convey(env, func() {
				cfg, err := testutil.ParseSettings(config)
				So(err, ShouldBeNil)
				err = validateSettings(cfg)
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
}
