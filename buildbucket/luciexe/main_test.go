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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/oauth2/google"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"
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

		// Setup a fake local auth. It is a real localhost server, it needs real
		// clock, so set it up before mocking time.
		fakeAuth := localauth.Server{
			TokenGenerators: map[string]localauth.TokenGenerator{
				"task": &authtest.FakeTokenGenerator{
					Email:  "task@example.com",
					Prefix: "task_token_",
				},
				"system": &authtest.FakeTokenGenerator{
					Email:  "system@example.com",
					Prefix: "system_token_",
				},
			},
			DefaultAccountID: "task",
		}
		la, err := fakeAuth.Start(ctx)
		So(err, ShouldBeNil)
		defer fakeAuth.Stop(ctx)
		ctx = lucictx.SetLocalAuth(ctx, la)

		nowTS, err := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		So(err, ShouldBeNil)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		tempDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		runSubtest := func(subtestName, argsText string) []*pb.UpdateBuildRequest {
			args := &pb.RunnerArgs{}
			err := proto.UnmarshalText(argsText, args)
			So(err, ShouldBeNil)

			// We spawn ourselves, see https://npf.io/2015/06/testing-exec-command/
			args.ExecutablePath = os.Args[0]
			args.WorkDir = filepath.Join(tempDir, "w")
			args.CacheDir = filepath.Join(tempDir, "cache")

			var ret []*pb.UpdateBuildRequest
			r := runner{
				localLogFile:  filepath.Join(tempDir, "logs"),
				testExtraArgs: []string{"-test.run=TestSubprocessDispatcher"},
				testExtraEnv:  []string{"LUCI_EXE_SUBTEST_NAME=" + subtestName},
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
			luci_system_account: "system"
		`

		Convey("Echo", func() {
			updates := runSubtest("testSuccess", `
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

		Convey("Final UpdateBuild does not set status to SUCCESS", func() {
			updates := runSubtest("testSuccess", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.UpdateMask.Paths, ShouldNotContain, "build.status")
		})

		Convey("Final UpdateBuild set status to INFRA_FAILURE", func() {
			updates := runSubtest("testInfraFailure", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.UpdateMask.Paths, ShouldContain, "build.status")
			So(final.Build.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
		})

		Convey("Cancels pending steps", func() {
			updates := runSubtest("testPendingStep", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.Build.Steps[0].Status, ShouldEqual, pb.Status_CANCELED)
			So(final.Build.Steps[0].EndTime, ShouldResembleProto, nowTS)
		})

		Convey("Auth context works", func() {
			updates := runSubtest("testAuthContext", dummyInputBuild)
			final := updates[len(updates)-1]
			So(final.Build.Status, ShouldEqual, pb.Status_SUCCESS)
		})
	})
}

func TestSubprocessDispatcher(t *testing.T) {
	subtest := os.Getenv("LUCI_EXE_SUBTEST_NAME")
	if subtest == "" {
		// Note: this code path is executed when running "go test ." normally. We
		// silently do nothing.
		return
	}

	st := subtests[subtest]
	if st == nil {
		t.Fatalf("no such subtest %q", subtest)
	}

	if err := client.Init(); err != nil {
		panic(err)
	}
	st(t)
	if err := client.Close(); err != nil {
		panic(err)
	}
}

////////////////////////////////////////////////////////////////////////////////
// These test* routines are individual subtests launched in a separate process.
//
// See TestSubprocessDispatcher for the environment and global vars they get.

var subtests = map[string]func(t *testing.T){
	"testAuthContext":  testAuthContext,
	"testInfraFailure": testInfraFailure,
	"testPendingStep":  testPendingStep,
	"testSuccess":      testSuccess,
}

var client = Client{
	BuildTimestamp: testclock.TestRecentTimeUTC,
}

func writeBuild(build *pb.Build) {
	if err := client.WriteBuild(build); err != nil {
		panic(err)
	}
}

// Verifies auth context is setup correctly.
func testAuthContext(t *testing.T) {
	checkContextAuth := func(ctx context.Context, expectedEmail, expectedToken string) {
		a := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
			Method: auth.LUCIContextMethod,
		})

		email, err := a.GetEmail()
		So(err, ShouldBeNil)
		So(email, ShouldEqual, expectedEmail)

		// Note: we do not log AccessToken in case we somehow mistakenly pick up
		// the real auth context with real tokens on the bots.
		tok, err := a.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken == expectedToken, ShouldBeTrue)
	}

	Convey("Task account is available", t, func() {
		ctx := context.Background()
		checkContextAuth(ctx, "task@example.com", "task_token_0")
	})

	Convey("System account is available", t, func() {
		ctx, err := lucictx.SwitchLocalAccount(context.Background(), "system")
		So(err, ShouldBeNil)
		checkContextAuth(ctx, "system@example.com", "system_token_0")
	})

	Convey("Git config is set", t, func() {
		gitHome := os.Getenv("INFRA_GIT_WRAPPER_HOME")
		So(gitHome, ShouldNotEqual, "")

		cfg, err := ioutil.ReadFile(filepath.Join(gitHome, ".gitconfig"))
		So(err, ShouldBeNil)

		So(string(cfg), ShouldContainSubstring, "email = task@example.com")
		So(string(cfg), ShouldContainSubstring, "helper = luci")
	})

	Convey("GCE metadata server is faked", t, func() {
		ts := google.ComputeTokenSource("default")
		tok, err := ts.Token()
		So(err, ShouldBeNil)
		// Note: we do not log AccessToken in case we somehow mistakenly pick up
		// real GCE server.
		So(tok.AccessToken == "task_token_1", ShouldBeTrue)
	})

	// Report the test result through Build proto.
	build := client.InitBuild
	if t.Failed() {
		build.Status = pb.Status_FAILURE
	} else {
		build.Status = pb.Status_SUCCESS
	}
	writeBuild(build)
}

// Marks the build as INFRA_FAILURE and exits.
func testInfraFailure(t *testing.T) {
	build := client.InitBuild

	// Final build must have a terminal status.
	build.Status = pb.Status_INFRA_FAILURE
	writeBuild(build)
}

// Emits a successful build with an incomplete step.
func testPendingStep(t *testing.T) {
	build := client.InitBuild

	// Final build must have a terminal status.
	build.Status = pb.Status_INFRA_FAILURE
	build.Steps = append(build.Steps, &pb.Step{
		Name: "pending step",
	})
	writeBuild(build)
}

// Marks the build as SUCCESS and exits.
func testSuccess(t *testing.T) {
	build := client.InitBuild

	// Final build must have a terminal status.
	build.Status = pb.Status_SUCCESS
	writeBuild(build)
}
