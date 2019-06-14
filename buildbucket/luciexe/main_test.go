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

// +build !windows

package luciexe

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMain(t *testing.T) {
	t.Parallel()

	Convey("Main", t, func(c C) {
		ctx := context.Background()

		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)

		nowTS, err := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		So(err, ShouldBeNil)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

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

		tempDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		run := func(executableName string, argsText string) []*pb.UpdateBuildRequest {
			args := &pb.RunnerArgs{}
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

			var ret []*pb.UpdateBuildRequest
			r := runner{
				localLogFile: filepath.Join(tempDir, "logs"),
				UpdateBuild: func(ctx context.Context, req *pb.UpdateBuildRequest) error {
					ret = append(ret, req)
					reqJSON, err := indentedJSONPB(req)
					c.So(err, ShouldBeNil)
					logging.Infof(ctx, "UpdateBuildRequest: %s", reqJSON)
					return nil
				},
			}
			err = r.Run(ctx, args)
			So(err, ShouldBeNil)

			So(ret, ShouldNotBeEmpty)
			return ret
		}

		dummyInputBuild := `
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
		`

		Convey("echo", func() {
			updates := run("success", `
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
			// Assert the final update.
			So(updates[len(updates)-1], ShouldResembleProtoText, `
				build {
					id: 1
					builder {
						project: "chromium"
						bucket: "try"
						builder: "linux-rel"
					}
					status: SUCCESS
				}
				update_mask {
					paths: "build.steps"
					paths: "build.output.properties"
					paths: "build.output.gitiles_commit"
					paths: "build.summary_markdown"
				}
			`)
		})

		Convey("final UpdateBuild does not set status to SUCCESS", func() {
			updates := run("success", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.UpdateMask.Paths, ShouldNotContain, "build.status")
		})

		Convey("final UpdateBuild set status to INFRA_FAILURE", func() {
			updates := run("infra_failure", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.UpdateMask.Paths, ShouldContain, "build.status")
			So(final.Build.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
		})

		Convey("Cancels pending steps", func() {
			updates := run("pending_step", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.Build.Steps[0].Status, ShouldEqual, pb.Status_CANCELED)
			So(final.Build.Steps[0].EndTime, ShouldResembleProto, nowTS)
		})
	})
}
