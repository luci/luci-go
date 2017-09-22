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

	"go.chromium.org/luci/luci_notify/testutil"
)

func TestValidation(t *testing.T) {
	Convey(`Test Environment for validateConfig`, t, func() {
		testValidation := func(config string, expectFormat string, expectArgs ...interface{}) {
			cfg, err := testutil.ParseConfig(config)
			So(err, ShouldBeNil)
			err = validateConfig("projects/test", cfg)
			if expectFormat == "" {
				So(err, ShouldBeNil)
				return
			}
			expect := fmt.Sprintf(`in "projects/test" `+expectFormat, expectArgs...)
			So(err.Error(), ShouldResemble, expect)
		}

		Convey(`empty`, func() {
			testValidation(``, "")
		})

		Convey(`notifier missing name`, func() {
			testValidation(`notifiers {}`, "(notifier #1): "+requiredFieldError, "name")
		})

		Convey(`notifier bad name`, func() {
			testValidation(`
			notifiers {
				name: "A_fnbA*G2n"
			}`,
				"(notifier #1): "+invalidFieldError, "name")
		})

		Convey(`notifier dup name`, func() {
			testValidation(`
			notifiers {
				name: "good-name"
			}
			notifiers {
				name: "good-name"
			}
			`, "(notifier #2): "+uniqueFieldError, "name", "project")
		})

		Convey(`builder missing name`, func() {
			testValidation(`
			notifiers {
				name: "good-name"
				builders {
					bucket: "test.bucket"
				}
			}
			`, "(notifier #1 / builder #1): "+requiredFieldError, "name")
		})

		Convey(`builder missing bucket`, func() {
			testValidation(`
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
				}
			}
			`, "(notifier #1 / builder #1): "+requiredFieldError, "bucket")
		})

		Convey(`bad email address`, func() {
			testValidation(`
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
			}
			`, "(notifier #1 / notification #1): "+badEmailError, "@@@@@")
		})
	})
}
