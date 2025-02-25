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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

var successResult = &cipdOut{
	Result: map[string][]*cipdPkg{
		"path_a":        {{Package: "pkg_a", InstanceID: "instance_a"}},
		"path_b":        {{Package: "pkg_b", InstanceID: "instance_b"}},
		kitchenCheckout: {{Package: "package", InstanceID: "instance_k"}},
	},
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
	if !(args[0][len(args[0])-4:] == "cipd" && args[1] == "ensure") {
		fmt.Fprintf(os.Stderr, "Not a cipd ensure command: %s\n", args)
		os.Exit(1)
	}
	switch os.Getenv("TEST_CASE") {
	case "success":
		// Mock the generated json file of `cipd ensure` command.
		jsonRs, _ := json.Marshal(successResult)
		if err := os.WriteFile(os.Getenv("RESULTS_FILE"), jsonRs, 0666); err != nil {
			fmt.Fprintf(os.Stderr, "Errors in preparing data for tests\n")
		}

	case "failure":
		os.Exit(1)
	}
}

func TestPrependPath(t *testing.T) {
	originalPathEnv := os.Getenv("PATH")
	ftt.Run("prependPath", t, func(t *ftt.Test) {
		defer func() {
			_ = os.Setenv("PATH", originalPathEnv)
		}()

		build := &bbpb.Build{
			Id: 123,
			Infra: &bbpb.BuildInfra{
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					Agent: &bbpb.BuildInfra_Buildbucket_Agent{
						Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*bbpb.InputDataRef{
								"path_a": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin", "path_a"},
								},
								"path_b": {
									DataType: &bbpb.InputDataRef_Cas{
										Cas: &bbpb.InputDataRef_CAS{
											CasInstance: "projects/project/instances/instance",
											Digest: &bbpb.InputDataRef_CAS_Digest{
												Hash:      "hash",
												SizeBytes: 1,
											},
										},
									},
									OnPath: []string{"path_b/bin", "path_b"},
								},
							},
							CipdSource: map[string]*bbpb.InputDataRef{
								"cipd": {
									OnPath: []string{"cipd"},
								},
							},
						},
						Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{},
					},
				},
			},
			Input: &bbpb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			},
		}

		cwd, err := os.Getwd()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, prependPath(build, cwd), should.BeNil)
		pathEnv := os.Getenv("PATH")
		var expectedPath []string
		for _, p := range []string{"path_a", "path_a/bin", "path_b", "path_b/bin"} {
			expectedPath = append(expectedPath, filepath.Join(cwd, p))
		}
		assert.Loosely(t, strings.Contains(pathEnv, strings.Join(expectedPath, string(os.PathListSeparator))), should.BeTrue)
	})
}

func TestInstallCipdPackages(t *testing.T) {
	t.Parallel()
	resultsFilePath = filepath.Join(t.TempDir(), "cipd_ensure_results.json")
	caseBase := "cache"
	ftt.Run("installCipdPackages", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		logs := logging.Get(ctx).(*memlogger.MemLogger)
		execCommandContext = fakeExecCommand
		defer func() { execCommandContext = exec.CommandContext }()

		build := &bbpb.Build{
			Id: 123,
			Infra: &bbpb.BuildInfra{
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					Agent: &bbpb.BuildInfra_Buildbucket_Agent{
						Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*bbpb.InputDataRef{
								"path_a": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin", "path_a"},
								},
								"path_b": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_b", Version: "latest"}},
										},
									},
									OnPath: []string{"path_b/bin", "path_b"},
								},
							},
						},
						Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{},
					},
				},
			},
			Input: &bbpb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			},
		}

		t.Run("without named cache", func(t *ftt.Test) {
			t.Run("success", func(t *ftt.Test) {
				testCase = "success"
				cwd, err := os.Getwd()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, installCipdPackages(ctx, build, cwd, caseBase), should.BeNil)
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData["path_a"], should.Match(&bbpb.ResolvedDataRef{
					DataType: &bbpb.ResolvedDataRef_Cipd{
						Cipd: &bbpb.ResolvedDataRef_CIPD{
							Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_a"][0].Package, Version: successResult.Result["path_a"][0].InstanceID}},
						},
					},
				}))
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData["path_b"], should.Match(&bbpb.ResolvedDataRef{
					DataType: &bbpb.ResolvedDataRef_Cipd{
						Cipd: &bbpb.ResolvedDataRef_CIPD{
							Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_b"][0].Package, Version: successResult.Result["path_b"][0].InstanceID}},
						},
					},
				}))
			})

			t.Run("failure", func(t *ftt.Test) {
				testCase = "failure"
				err := installCipdPackages(ctx, build, ".", caseBase)
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData, should.BeNil)
				assert.Loosely(t, err, should.ErrLike("Failed to run cipd ensure command"))
			})
		})

		t.Run("with named cache", func(t *ftt.Test) {
			build.Infra.Buildbucket.Agent.CipdPackagesCache = &bbpb.CacheEntry{
				Name: "cipd_cache_hash",
				Path: "cipd_cache",
			}
			testCase = "success"
			cwd, err := os.Getwd()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, installCipdPackages(ctx, build, cwd, caseBase), should.BeNil)
			assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData["path_a"], should.Match(&bbpb.ResolvedDataRef{
				DataType: &bbpb.ResolvedDataRef_Cipd{
					Cipd: &bbpb.ResolvedDataRef_CIPD{
						Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_a"][0].Package, Version: successResult.Result["path_a"][0].InstanceID}},
					},
				},
			}))
			assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData["path_b"], should.Match(&bbpb.ResolvedDataRef{
				DataType: &bbpb.ResolvedDataRef_Cipd{
					Cipd: &bbpb.ResolvedDataRef_CIPD{
						Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result["path_b"][0].Package, Version: successResult.Result["path_b"][0].InstanceID}},
					},
				},
			}))
			assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
				logging.Info, fmt.Sprintf(`Setting $CIPD_CACHE_DIR to %q`, filepath.Join(cwd, caseBase, "cipd_cache"))))
		})

		t.Run("handle kitchenCheckout", func(t *ftt.Test) {
			t.Run("kitchenCheckout not in agent input", func(t *ftt.Test) {
				testCase = "success"
				build.Exe = &bbpb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
				}
				cwd, err := os.Getwd()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, installCipdPackages(ctx, build, cwd, "cache"), should.BeNil)
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Purposes[kitchenCheckout], should.Equal(bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD))
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData[kitchenCheckout], should.Match(&bbpb.ResolvedDataRef{
					DataType: &bbpb.ResolvedDataRef_Cipd{
						Cipd: &bbpb.ResolvedDataRef_CIPD{
							Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result[kitchenCheckout][0].Package, Version: successResult.Result[kitchenCheckout][0].InstanceID}},
						},
					},
				}))
			})
			t.Run("kitchenCheckout in agent input", func(t *ftt.Test) {
				testCase = "success"
				build.Exe = &bbpb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
				}
				build.Infra.Buildbucket.Agent.Input.Data[kitchenCheckout] = &bbpb.InputDataRef{
					DataType: &bbpb.InputDataRef_Cipd{
						Cipd: &bbpb.InputDataRef_CIPD{
							Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "package", Version: "version"}},
						},
					},
				}
				build.Infra.Buildbucket.Agent.Purposes = map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
					kitchenCheckout: bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				}
				cwd, err := os.Getwd()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, installCipdPackages(ctx, build, cwd, "cache"), should.BeNil)
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Purposes[kitchenCheckout], should.Equal(bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD))
				assert.Loosely(t, build.Infra.Buildbucket.Agent.Output.ResolvedData[kitchenCheckout], should.Match(&bbpb.ResolvedDataRef{
					DataType: &bbpb.ResolvedDataRef_Cipd{
						Cipd: &bbpb.ResolvedDataRef_CIPD{
							Specs: []*bbpb.ResolvedDataRef_CIPD_PkgSpec{{Package: successResult.Result[kitchenCheckout][0].Package, Version: successResult.Result[kitchenCheckout][0].InstanceID}},
						},
					},
				}))
			})
		})
	})
}

func TestInstallCipd(t *testing.T) {
	t.Parallel()
	ftt.Run("InstallCipd", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = memlogger.Use(ctx)
		logs := logging.Get(ctx).(*memlogger.MemLogger)
		build := &bbpb.Build{
			Id: 123,
			Infra: &bbpb.BuildInfra{
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					Agent: &bbpb.BuildInfra_Buildbucket_Agent{
						Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
							CipdSource: map[string]*bbpb.InputDataRef{
								"cipd": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Server: "chrome-infra-packages.appspot.com",
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{
												{
													Package: "infra/tools/cipd/${platform}",
													Version: "latest",
												},
											},
										},
									},
									OnPath: []string{"cipd"},
								},
							},
							Data: map[string]*bbpb.InputDataRef{
								"path_a": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin", "path_a"},
								},
								"path_b": {
									DataType: &bbpb.InputDataRef_Cipd{
										Cipd: &bbpb.InputDataRef_CIPD{
											Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_b", Version: "latest"}},
										},
									},
									OnPath: []string{"path_b/bin", "path_b"},
								},
							},
						},
						Output: &bbpb.BuildInfra_Buildbucket_Agent_Output{},
					},
				},
			},
			Input: &bbpb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			},
		}
		tempDir := t.TempDir()
		cacheBase := "cache"
		cipdURL := "https://chrome-infra-packages.appspot.com/client?platform=linux-amd64&version=latest"
		t.Run("without cache", func(t *ftt.Test) {
			err := installCipd(ctx, build, tempDir, cacheBase, "linux-amd64")
			assert.Loosely(t, err, should.BeNil)
			// check to make sure cipd is correctly saved in the directory
			cipdDir := filepath.Join(tempDir, "cipd")
			files, err := os.ReadDir(cipdDir)
			assert.Loosely(t, err, should.BeNil)
			for _, file := range files {
				assert.Loosely(t, file.Name(), should.Equal("cipd"))
			}
			// check the cipd path in set in PATH
			pathEnv := os.Getenv("PATH")
			assert.Loosely(t, strings.Contains(pathEnv, "cipd"), should.BeTrue)
			assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
				logging.Info, fmt.Sprintf("Install CIPD client from URL: %s into %s", cipdURL, cipdDir)))
		})

		t.Run("with cache", func(t *ftt.Test) {
			build.Infra.Buildbucket.Agent.CipdClientCache = &bbpb.CacheEntry{
				Name: "cipd_client_hash",
				Path: "cipd_client",
			}
			cipdCacheDir := filepath.Join(tempDir, cacheBase, "cipd_client")
			err := os.MkdirAll(cipdCacheDir, 0750)
			assert.Loosely(t, err, should.BeNil)

			t.Run("hit", func(t *ftt.Test) {
				// create an empty file as if it's the cipd client.
				err := os.WriteFile(filepath.Join(cipdCacheDir, "cipd"), []byte(""), 0644)
				assert.Loosely(t, err, should.BeNil)
				err = installCipd(ctx, build, tempDir, cacheBase, "linux-amd64")
				assert.Loosely(t, err, should.BeNil)
				// check to make sure cipd is correctly saved in the directory
				files, err := os.ReadDir(cipdCacheDir)
				assert.Loosely(t, err, should.BeNil)
				for _, file := range files {
					assert.Loosely(t, file.Name(), should.Equal("cipd"))
				}
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldNotHaveLog)(
					logging.Info, fmt.Sprintf("Install CIPD client from URL: %s into %s", cipdURL, cipdCacheDir)))

				// check the cipd path in set in PATH
				pathEnv := os.Getenv("PATH")
				assert.Loosely(t, strings.Contains(pathEnv, strings.Join([]string{cipdCacheDir, filepath.Join(cipdCacheDir, "bin")}, string(os.PathListSeparator))), should.BeTrue)
			})

			t.Run("miss", func(t *ftt.Test) {
				err := installCipd(ctx, build, tempDir, cacheBase, "linux-amd64")
				assert.Loosely(t, err, should.BeNil)
				// check to make sure cipd is correctly saved in the directory
				files, err := os.ReadDir(cipdCacheDir)
				assert.Loosely(t, err, should.BeNil)
				for _, file := range files {
					assert.Loosely(t, file.Name(), should.Equal("cipd"))
				}
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
					logging.Info, fmt.Sprintf("Install CIPD client from URL: %s into %s", cipdURL, cipdCacheDir)))

				// check the cipd path in set in PATH
				pathEnv := os.Getenv("PATH")
				assert.Loosely(t, strings.Contains(pathEnv, strings.Join([]string{cipdCacheDir, filepath.Join(cipdCacheDir, "bin")}, string(os.PathListSeparator))), should.BeTrue)
			})
		})
	})
}
