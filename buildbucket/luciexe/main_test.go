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

package luciexe

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
	t.Parallel()

	Convey("Main", t, func(c C) {
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

		tempDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		run := func(executableName string, argsText string) (finalBuild *pb.Build, updateRequests []*pb.UpdateBuildRequest) {
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

			r := runner{
				localLogFile: filepath.Join(tempDir, "logs"),
				UpdateBuild: func(ctx context.Context, req *pb.UpdateBuildRequest) error {
					updateRequests = append(updateRequests, req)
					reqJSON, err := indentedJSONPB(req)
					c.So(err, ShouldBeNil)
					logging.Infof(ctx, "UpdateBuildRequest: %s", reqJSON)
					return nil
				},
			}
			finalBuild, err = r.Run(ctx, args)
			So(err, ShouldBeNil)

			So(updateRequests, ShouldNotBeEmpty)
			So(updateRequests[len(updateRequests)-1].Build, ShouldResembleProto, finalBuild)
			return
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
			finalBuild, _ := run("echo", `
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
			So(finalBuild, ShouldResembleProtoText, `
				id: 1
				builder {
					project: "chromium"
					bucket: "try"
					builder: "linux-rel"
				}
				status: SUCCESS
			`)
		})

		Convey("final UpdateBuild does not set status to SUCCESS", func() {
			_, updateBuildRequests := run("echo", dummyInputBuild)
			finalUpdate := updateBuildRequests[len(updateBuildRequests)-1]
			So(finalUpdate.UpdateMask.Paths, ShouldNotContain, "build.status")
		})

		Convey("final UpdateBuild set status to INFRA_FAILURE", func() {
			_, updateBuildRequests := run("infra_fail", dummyInputBuild)
			finalUpdate := updateBuildRequests[len(updateBuildRequests)-1]
			So(finalUpdate.UpdateMask.Paths, ShouldContain, "build.status")
			So(finalUpdate.Build.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
		})
	})
}
