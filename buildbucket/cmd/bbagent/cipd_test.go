// Copyright 2022 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging/memlogger"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
)

var successResult = &cipdOut{
	Result: map[string][]*cipdPkg{
		"path_a": {{Package: "pkg_a", InstanceID: "instance_a"}},
		"path_b": {{Package: "pkg_b", InstanceID: "instance_b"}}},
}

var testCase string

// fakeExecCommand mocks exec Command. It will trigger TestHelperProcess to
// return the right mocked output.
func fakeExecCommand(_ context.Context, command string, args ...string) *exec.Cmd {
	os.Environ()
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	tc := "TEST_CASE=" + testCase
	fakeResultsFilePath := "RESULTS_FILE=" + resultsFilePath
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", tc, fakeResultsFilePath}
	return cmd
}

// TestHelperProcess produces fake outputs based on the "TEST_CASE" env var when
// executed with the env var "GO_WANT_HELPER_PROCESS" set to 1.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	// check if it's a `cipd ensure` command.
	if !(args[0] == "cipd" && args[1] == "ensure") {
		fmt.Fprintf(os.Stderr, "Not a cipd ensure command: %s\n", args)
		os.Exit(1)
	}
	switch os.Getenv("TEST_CASE") {
	case "success":
		// Mock the generated json file of `cipd ensure` command.
		jsonRs, _ := json.Marshal(successResult)
		if err := ioutil.WriteFile(os.Getenv("RESULTS_FILE"), jsonRs, 0666); err != nil {
			fmt.Fprintf(os.Stderr, "Errors in preparing data for tests\n")
		}

	case "failure":
		os.Exit(1)
	}
}

func TestInstallCipdPackages(t *testing.T) {
	resultsFilePath = filepath.Join(t.TempDir(), "cipd_ensure_results.json")
	Convey("installCipdPackages", t, func() {
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		execCommandContext = fakeExecCommand
		defer func() { execCommandContext = exec.CommandContext }()

		build := &pb.Build{
			Id: 123,
			Infra: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"path_a": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin"},
								},
								"path_b": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_b", Version: "latest"}},
										},
									},
									OnPath: []string{"path_b/bin"},
								},
							},
						},
					},
				},
			},
			Input: &pb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			},
		}

		originalPathEnv := os.Getenv("PATH")
		Convey("success", func() {
			defer func() {
				_ = os.Setenv("PATH", originalPathEnv)
			}()
			testCase = "success"
			cwd, err := os.Getwd()
			So(err, ShouldBeNil)
			resolvedDataMap, err := installCipdPackages(ctx, build, cwd)
			So(err, ShouldBeNil)
			So(resolvedDataMap["path_a"], ShouldResembleProto, &pb.ResolvedDataRef{
				DataType: &pb.ResolvedDataRef_Cipd{
					Cipd: &pb.ResolvedDataRef_CIPD{
						Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_a"][0].Package, Version: successResult.Result["path_a"][0].InstanceID}},
					},
				},
			})
			So(resolvedDataMap["path_b"], ShouldResembleProto, &pb.ResolvedDataRef{
				DataType: &pb.ResolvedDataRef_Cipd{
					Cipd: &pb.ResolvedDataRef_CIPD{
						Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_b"][0].Package, Version: successResult.Result["path_b"][0].InstanceID}},
					},
				},
			})
			pathEnv := os.Getenv("PATH")
			So(strings.Contains(pathEnv, filepath.Join(cwd, "path_a/bin")), ShouldBeTrue)
			So(strings.Contains(pathEnv, filepath.Join(cwd, "path_b/bin")), ShouldBeTrue)
		})

		Convey("failure", func() {
			testCase = "failure"
			resolvedDataMap, err := installCipdPackages(ctx, build, ".")
			So(resolvedDataMap, ShouldBeNil)
			So(err, ShouldErrLike, "Failed to run cipd ensure command")
			So(os.Getenv("PATH"), ShouldEqual, originalPathEnv)
		})

		// Make sure the original $PATH env var is restored after running all tests.
		So(os.Getenv("PATH"), ShouldEqual, originalPathEnv)
	})
}
