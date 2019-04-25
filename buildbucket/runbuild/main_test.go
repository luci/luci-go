// Copyright 2019 The LUCI Authors.
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

package runbuild

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/logging"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMain(t *testing.T) {
	Convey("Main", t, func() {
		ctx := context.Background()

		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)

		// Setup a fake local auth.
		ctx = lucictx.SetLocalAuth(ctx, &lucictx.LocalAuth{
			RPCPort: 1234,
			Accounts: []lucictx.LocalAuthAccount{
				{
					ID:    "task",
					Email: "task@example.com",
				},
				{
					ID:    "system",
					Email: "system@example.com",
				},
			},
			DefaultAccountID: "task",
		})

		ctx = gologger.StdConfig.Use(ctx)

		tempDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		run := func(executableName string, argsText string) *pb.Build {
			args := &pb.RunBuildArgs{}
			err := proto.UnmarshalText(argsText, args)
			So(err, ShouldBeNil)

			args.WorkDir = filepath.Join(tempDir, "w")
			args.ExecutablePath = filepath.Join(tempDir, "user_executable.exe")
			args.CacheDir = filepath.Join(tempDir, "cache")

			goBuild := exec.Command(
				"go", "build",
				"-o", args.ExecutablePath,
				fmt.Sprintf("testdata/%s.go", executableName),
				"testdata/lib.go")
			So(goBuild.Run(), ShouldBeNil)

			runner := buildRunner{localLogFile: filepath.Join(tempDir, "logs")}
			build, err := runner.Run(ctx, args)
			So(err, ShouldBeNil)
			return build
		}

		Convey("echo", func() {
			build := run("echo", `
				buildbucket_host: "buildbucket.example.com"
				logdog_host: "logdog.example.com"
				build {
					id: 1
					builder {
						project: "chromium"
						bucket: "try"
						builder: "linux-rel"
					}
				}
			`)
			So(build, ShouldResembleProtoText, `
				id: 1
				builder {
					project: "chromium"
					bucket: "try"
					builder: "linux-rel"
				}
			`)
		})
	})
}
