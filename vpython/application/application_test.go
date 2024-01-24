// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package application

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseArguments(t *testing.T) {
	Convey("Test parse arguments", t, func() {
		ctx := context.Background()

		app := &Application{}
		app.Initialize(ctx)

		parseArgs := func(args ...string) error {
			app.Arguments = args
			So(app.ParseEnvs(ctx), ShouldBeNil)
			return app.ParseArgs(ctx)
		}

		Convey("Test log level", func() {
			err := parseArgs(
				"-vpython-log-level",
				"warning",
			)
			So(err, ShouldBeNil)
			ctx = app.SetLogLevel(ctx)
			So(logging.GetLevel(ctx), ShouldEqual, logging.Warning)
		})

		Convey("Test unknown argument", func() {
			const unknownErr = "failed to extract flags: unknown flag: vpython-test"

			// Care but only care arguments begin with "-" or "--".
			err := parseArgs("-vpython-test")
			So(err, ShouldBeError, unknownErr)
			err = parseArgs("--vpython-test")
			So(err, ShouldBeError, unknownErr)
			err = parseArgs("-vpython-root", "root", "vpython-test")
			So(err, ShouldBeNil)

			// All arguments after the script file should be bypassed.
			err = parseArgs("-vpython-test", "test.py")
			So(err, ShouldBeError, unknownErr)
			err = parseArgs("test.py", "-vpython-test")
			So(err, ShouldBeNil)

			// Stop parsing arguments when seen --
			err = parseArgs("--", "-vpython-test")
			So(err, ShouldBeNil)
		})

		Convey("Test cipd cache dir", func() {
			err := parseArgs("-vpython-root", "root", "vpython-test")
			So(err, ShouldBeNil)
			So(app.CIPDCacheDir, ShouldStartWith, "root")
		})

		Convey("Test cipd cache dir with env", func() {
			// Don't set cipd cache dir if env provides one
			app.Environments = append(app.Environments, fmt.Sprintf("%s=%s", cipd.EnvCacheDir, "something"))
			err := parseArgs("-vpython-root", "root", "vpython-test")
			So(err, ShouldBeNil)
			So(app.CIPDCacheDir, ShouldStartWith, "something")
		})
	})
}
