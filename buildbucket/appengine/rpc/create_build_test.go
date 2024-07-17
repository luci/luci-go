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

package rpc

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func validCreateBuildRequest() *pb.CreateBuildRequest {
	return &pb.CreateBuildRequest{
		Build: &pb.Build{
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Infra: &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					PayloadPath: "kitchen-checkout",
					CacheDir:    "cache",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Source: &pb.BuildInfra_Buildbucket_Agent_Source{
							DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
								Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
									Package: "infra/tools/luci/bbagent/${platform}",
									Version: "canary-version",
									Server:  "cipd server",
								},
							},
						},
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"path_a": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a/${platform}", Version: "latest"}},
										},
									},
									OnPath: []string{"path_a/bin", "path_a"},
								},
								"path_b": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_b/${platform}", Version: "latest"}},
										},
									},
									OnPath: []string{"path_b/bin", "path_b"},
								},
							},
						},
					},
				},
				Swarming: &pb.BuildInfra_Swarming{
					Hostname: "host",
					Priority: 25,
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "pool",
							Value: "example.pool",
						},
					},
					TaskServiceAccount: "example@account.com",
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{
							Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path: "builder",
							WaitForWarmCache: &durationpb.Duration{
								Seconds: 240,
							},
						},
					},
				},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "host",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{
					Hostname: "host",
				},
			},
			Input: &pb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "value",
							},
						},
					},
				},
				GerritChanges: []*pb.GerritChange{
					{
						Host:     "h1",
						Project:  "b",
						Change:   1,
						Patchset: 1,
					},
				},
				Experiments: []string{"customized.exp.name", "luci.wellknown.exp"},
			},
			Exe: &pb.Executable{
				Cmd: []string{"recipes"},
			},
		},
		RequestId: "request_id",
	}
}

func TestValidateCreateBuildRequest(t *testing.T) {
	t.Parallel()
	Convey("validateCreateBuildRequest", t, func() {
		req := validCreateBuildRequest()
		wellknownExps := stringset.NewFromSlice("luci.wellknown.exp")
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
			Acls: []*pb.Acl{
				{
					Identity: "user:caller@example.com",
					Role:     pb.Acl_SCHEDULER,
				},
			},
			Name: "bucket",
			Constraints: &pb.Bucket_Constraints{
				Pools:           []string{"example.pool"},
				ServiceAccounts: []string{"example@account.com"},
			},
		})

		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Backends: []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "chromium-swarm.appspot.com",
				},
			},
			Experiment: &pb.ExperimentSettings{
				Experiments: []*pb.ExperimentSettings_Experiment{
					{
						Name: "luci.wellknown.exp",
					},
				},
			},
		}), ShouldBeNil)

		Convey("works", func() {
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldBeNil)
		})

		Convey("mask", func() {
			req.Mask = &pb.BuildMask{
				Fields: &fieldmaskpb.FieldMask{
					Paths: []string{
						"invalid",
					},
				},
			}
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldErrLike, `invalid mask`)
		})

		Convey("RequestID", func() {
			req.RequestId = "request/id"
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldErrLike, `request_id cannot contain '/'`)
		})

		Convey("Build", func() {
			Convey("Builder", func() {
				Convey("invalid Builder", func() {
					req.Build.Builder = &pb.BuilderID{
						Project: "project",
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: builder: bucket is required`)
				})
				Convey("w/o Builder", func() {
					req.Build.Builder = nil
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `.build.builder: required`)
				})
			})

			Convey("Exe", func() {
				Convey("cipd_package not specified", func() {
					req.Build.Exe = &pb.Executable{
						Cmd: []string{"recipes"},
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldBeNil)
				})
				Convey("cipd_package", func() {
					req.Build.Exe = &pb.Executable{
						CipdPackage: "{Invalid}/${platform}",
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: exe: cipd_package`)
				})
				Convey("cipd_version", func() {
					req.Build.Exe = &pb.Executable{
						CipdPackage: "valid/package/name/${platform}",
						CipdVersion: "+100",
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: exe: cipd_version`)
				})
				Convey("exe doesn't match agent", func() {
					req.Build.Exe = &pb.Executable{
						CipdPackage: "valid/package/name/${platform}",
						CipdVersion: "version",
					}
					req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
						"payload_path": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					}
					Convey("payload in agentInput is not a CIPD Package", func() {
						req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"payload_path": {
									DataType: &pb.InputDataRef_Cas{
										Cas: &pb.InputDataRef_CAS{
											CasInstance: "projects/project/instances/instance",
											Digest: &pb.InputDataRef_CAS_Digest{
												Hash:      "hash",
												SizeBytes: 1,
											},
										},
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: exe: not match build.infra.buildbucket.agent`)
					})
					Convey("different package", func() {
						req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"payload_path": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "another", Version: "latest"}},
										},
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: exe: cipd_package does not match build.infra.buildbucket.agent`)
					})
					Convey("different version", func() {
						req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{
								"payload_path": {
									DataType: &pb.InputDataRef_Cipd{
										Cipd: &pb.InputDataRef_CIPD{
											Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "valid/package/name/${platform}", Version: "latest"}},
										},
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: exe: cipd_version does not match build.infra.buildbucket.agent`)
					})
				})
			})

			Convey("Input", func() {
				Convey("gerrit_changes", func() {
					req.Build.Input.GerritChanges = []*pb.GerritChange{
						{
							Host: "host",
						},
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: input: gerrit_changes`)
				})
				Convey("gitiles_commit", func() {
					req.Build.Input.GitilesCommit = &pb.GitilesCommit{
						Host: "host",
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: input: gitiles_commit`)
				})
				Convey("properties", func() {
					req.Build.Input.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"$recipe_engine/buildbucket": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: input: properties`)
				})
				Convey("experiments", func() {
					req.Build.Input.Experiments = []string{"luci.not.wellknown"}
					_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
					So(err, ShouldErrLike, `build: input: experiment "luci.not.wellknown"`)
				})
			})

			Convey("Tags", func() {
				req.Build.Tags = []*pb.StringPair{
					{
						Key:   "build_address",
						Value: "value2",
					},
				}
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, `build: tags`)
			})

			Convey("Infra", func() {

				Convey("buildbucket", func() {
					Convey("host", func() {
						req.Build.Infra.Buildbucket.Hostname = "wrong.appspot.com"
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: buildbucket: incorrect hostname, want: app.appspot.com, got: wrong.appspot.com`)
					})
					Convey("agent", func() {
						Convey("input", func() {
							Convey("package", func() {
								req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"path_a": {
											DataType: &pb.InputDataRef_Cipd{
												Cipd: &pb.InputDataRef_CIPD{
													Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "", Version: "latest"}},
												},
											},
										},
									},
								}
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: input: [path_a]: [0]: cipd.package`)
							})
							Convey("version", func() {
								req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"path_a": {
											DataType: &pb.InputDataRef_Cipd{
												Cipd: &pb.InputDataRef_CIPD{
													Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "a/package/${platform}", Version: ""}},
												},
											},
										},
									},
								}
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: input: [path_a]: [0]: cipd.version`)
							})
							Convey("cas instance", func() {
								req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"path_a": {
											DataType: &pb.InputDataRef_Cas{
												Cas: &pb.InputDataRef_CAS{
													CasInstance: "instance",
												},
											},
										},
									},
								}
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: input: [path_a]: cas.cas_instance`)
							})
							Convey("cas digest", func() {
								req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"path_a": {
											DataType: &pb.InputDataRef_Cas{
												Cas: &pb.InputDataRef_CAS{
													CasInstance: "projects/project/instances/instance",
												},
											},
										},
									},
								}
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: input: [path_a]: cas.digest`)
							})
							Convey("cas digest size", func() {
								req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"path_a": {
											DataType: &pb.InputDataRef_Cas{
												Cas: &pb.InputDataRef_CAS{
													CasInstance: "projects/project/instances/instance",
													Digest: &pb.InputDataRef_CAS_Digest{
														Hash:      "hash",
														SizeBytes: -1,
													},
												},
											},
										},
									},
								}
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: input: [path_a]: cas.digest.size_bytes`)
							})
						})
						Convey("source", func() {
							Convey("package", func() {
								req.Build.Infra.Buildbucket.Agent.Source.GetCipd().Package = "cipd/package"
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: source: cipd.package`)
							})
							Convey("version", func() {
								req.Build.Infra.Buildbucket.Agent.Source.GetCipd().Version = "+100"
								_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
								So(err, ShouldErrLike, `build: infra: buildbucket: agent: source: cipd.version`)
							})
						})
						Convey("purposes", func() {
							req.Build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
								Data: map[string]*pb.InputDataRef{
									"path_a": {
										DataType: &pb.InputDataRef_Cipd{
											Cipd: &pb.InputDataRef_CIPD{
												Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a/${platform}", Version: "latest"}},
											},
										},
										OnPath: []string{"path_a/bin", "path_a"},
									},
								},
							}
							req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
								"path_b": pb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY,
							}
							_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
							So(err, ShouldErrLike, `build: infra: buildbucket: agent: purposes`)
						})
					})
				})

				Convey("swarming", func() {
					Convey("hostname", func() {
						req.Build.Infra.Swarming.Hostname = "https://host"
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: swarming: hostname: must not contain "://"`)
					})
					Convey("priority", func() {
						req.Build.Infra.Swarming.Priority = 500
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: swarming: priority must be in [0, 255]`)
					})
					Convey("task_dimensions", func() {
						Convey("empty key", func() {
							req.Build.Infra.Swarming.TaskDimensions = []*pb.RequestedDimension{
								{
									Key: "",
								},
							}
							_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
							So(err, ShouldErrLike, `build: infra: swarming: task_dimensions: [0]: key must be specified`)
						})
						Convey("empty value", func() {
							req.Build.Infra.Swarming.TaskDimensions = []*pb.RequestedDimension{
								{
									Key:   "key",
									Value: "",
								},
							}
							_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
							So(err, ShouldErrLike, `build: infra: swarming: task_dimensions: [0]: value must be specified`)
						})
						Convey("expiration", func() {
							req.Build.Infra.Swarming.TaskDimensions = []*pb.RequestedDimension{
								{
									Key:   "key",
									Value: "value",
									Expiration: &durationpb.Duration{
										Seconds: 200,
									},
								},
							}
							_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
							So(err, ShouldErrLike, `build: infra: swarming: task_dimensions: [0]: expiration:`)
						})
					})
				})

				Convey("logdog", func() {
					Convey("hostname", func() {
						req.Build.Infra.Logdog.Hostname = "https://host"
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: logdog: hostname: must not contain "://"`)
					})
				})

				Convey("resultdb", func() {
					Convey("hostname", func() {
						req.Build.Infra.Resultdb.Hostname = "https://host"
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: resultdb: hostname: must not contain "://"`)
					})
				})
				Convey("backend", func() {
					Convey("swarmingAndBackendBothSet", func() {
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: can only have one of backend or swarming in build infra. both were provided`)
					})
					Convey("targetIsValid", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldBeNil)
					})
					Convey("targetIsNotValid", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://v8-swarm",
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: backend: provided backend target was not in global config`)
					})
					Convey("task_dimensions", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
							TaskDimensions: []*pb.RequestedDimension{
								{
									Key: "",
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: backend: task_dimensions: [0]: key must be specified`)
					})
					Convey("caches", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
							Caches: []*pb.CacheEntry{
								{
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: backend: caches: 0th cache: name unspecified`)
					})
					Convey("priority too large", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
							Config: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"priority": {
										Kind: &structpb.Value_NumberValue{NumberValue: 500},
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: backend: config: priority must be in [0, 255]`)
					})
					Convey("priority not number", func() {
						req.Build.Infra.Swarming = nil
						req.Build.Infra.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://chromium-swarm",
								},
							},
							Config: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"priority": {
										Kind: &structpb.Value_StringValue{StringValue: "a"},
									},
								},
							},
						}
						_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
						So(err, ShouldErrLike, `build: infra: backend: config: priority must be a number`)
					})
				})
			})

			Convey("output_only fields are cleared", func() {
				req.Build.Id = 87654321
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldBeNil)
				So(req.Build.Id, ShouldEqual, 0)
			})
		})

		Convey("BucketConstraints", func() {
			Convey("no bucket constraints", func() {
				testutil.PutBucket(ctx, "project", "bucket", nil)

				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
					},
				})
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, `constraints for project:bucket not found`)
			})
			Convey("pool not allowed", func() {
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Constraints: &pb.Bucket_Constraints{
						Pools:           []string{"different.pool"},
						ServiceAccounts: []string{"example@account.com"},
					}})

				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
					},
				})
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, `build.infra.swarming.dimension['pool']: example.pool not allowed`)
			})
			Convey("service account not allowed", func() {
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Constraints: &pb.Bucket_Constraints{
						Pools:           []string{"example.pool"},
						ServiceAccounts: []string{"different@account.com"},
					}})

				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
					},
				})
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, `build.infra.swarming.task_service_account: example@account.com not allowed`)
			})
		})

		Convey("invalid request", func() {
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
				Constraints: &pb.Bucket_Constraints{
					Pools:           []string{"example.pool"},
					ServiceAccounts: []string{"example@account.com"},
				}})

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
				},
			})
			req.RequestId = "request/id"
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldErrLike, `request_id cannot contain '/'`)
		})

		Convey("CreateBuild specified output_only fields are cleared", func() {
			req.Build.Status = pb.Status_SCHEDULED
			req.Build.SummaryMarkdown = "random string"
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldBeNil)
			So(req.Build.Status, ShouldEqual, pb.Status_STATUS_UNSPECIFIED)
			So(req.Build.SummaryMarkdown, ShouldEqual, "")
		})

		Convey("CreateBuild ensures required fields", func() {
			Convey("top level required fields are ensured", func() {
				req.Build.Infra = nil
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, ".build.infra: required")
			})

			Convey("sub fields are required if their upper level is non nil", func() {
				req.Build.Infra.Resultdb = nil
				_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldBeNil)

				req.Build.Infra.Resultdb = &pb.BuildInfra_ResultDB{}
				_, err = validateCreateBuildRequest(ctx, wellknownExps, req)
				So(err, ShouldErrLike, ".build.infra.resultdb.hostname: required")
			})
		})
	})
}

func TestCreateBuild(t *testing.T) {
	Convey("CreateBuild", t, func() {
		srv := &Builds{}
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		store := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"key": {Active: []byte("stuff")},
			},
		}
		ctx = secrets.Use(ctx, store)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})
		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Backends: []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "chromium-swarm.appspot.com",
				},
			},
			Experiment: &pb.ExperimentSettings{
				Experiments: []*pb.ExperimentSettings_Experiment{
					{
						Name: "luci.wellknown.exp",
					},
				},
			},
		}), ShouldBeNil)

		req := validCreateBuildRequest()

		Convey("with parent", func() {
			Convey("parent ended", func() {
				pTok, err := buildtoken.GenerateToken(ctx, 97654321, pb.TokenBody_BUILD)
				So(err, ShouldBeNil)

				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, pTok))

				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
						{Realm: "project:bucket", Permission: bbperms.BuildsGet},
					},
				})
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Constraints: &pb.Bucket_Constraints{
						Pools:           []string{"example.pool"},
						ServiceAccounts: []string{"example@account.com"},
					}})
				testutil.PutBuilder(ctx, "project", "bucket", "parent", "")
				So(datastore.Put(ctx, &model.Build{
					ID: 97654321,
					Proto: &pb.Build{
						Id: 97654321,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "parent",
						},
						Status: pb.Status_SUCCESS,
					},
					UpdateToken: pTok,
				}), ShouldBeNil)
				_, err = srv.CreateBuild(ctx, req)
				So(err, ShouldErrLike, `build parent: 97654321 has ended, cannot add child to it`)
			})

			Convey("pass", func() {
				pTok, _ := buildtoken.GenerateToken(ctx, 97654321, pb.TokenBody_BUILD)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, pTok))

				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
						{Realm: "project:bucket", Permission: bbperms.BuildsGet},
					},
				})
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Constraints: &pb.Bucket_Constraints{
						Pools:           []string{"example.pool"},
						ServiceAccounts: []string{"example@account.com"},
					}})
				testutil.PutBuilder(ctx, "project", "bucket", "parent", "")
				pBld := &model.Build{
					ID: 97654321,
					Proto: &pb.Build{
						Id: 97654321,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "parent",
						},
						Status: pb.Status_STARTED,
					},
					UpdateToken: pTok,
				}
				So(datastore.Put(ctx, pBld), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, pBld),
					Proto: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{
							TaskId: "544239050",
						},
					},
				}), ShouldBeNil)
				req.Mask = &pb.BuildMask{
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{
							"id",
							"tags",
							"ancestor_ids",
						},
					},
				}
				b, err := srv.CreateBuild(ctx, req)
				So(b, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 3)
				So(protoutil.StringPairMap(b.Tags).Format(), ShouldResemble, []string{"parent_task_id:544239051"})
				So(b.AncestorIds, ShouldResemble, []int64{97654321})
			})
		})

		Convey("passes", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
				},
			})
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
				Constraints: &pb.Bucket_Constraints{
					Pools:           []string{"example.pool"},
					ServiceAccounts: []string{"example@account.com"},
				}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			b, err := srv.CreateBuild(ctx, req)
			So(b, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldHaveLength, 3)

			// Check datastore.
			bld := &model.Build{ID: b.Id}
			bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, &model.Build{ID: b.Id})}
			So(datastore.Get(ctx, bld, bs), ShouldBeNil)
			So(bs.Status, ShouldEqual, pb.Status_SCHEDULED)
			So(bs.BuildAddress, ShouldEqual, fmt.Sprintf("project/bucket/builder/b%d", b.Id))
		})

		Convey("passes with backend", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "project:bucket", Permission: bbperms.BuildsCreate},
				},
			})
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
				Constraints: &pb.Bucket_Constraints{
					Pools:           []string{"example.pool"},
					ServiceAccounts: []string{"example@account.com"},
				}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "swarming://chromium-swarm")
			req.Build.Infra.Swarming = nil
			req.Build.Infra.Backend = &pb.BuildInfra_Backend{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Target: "swarming://chromium-swarm",
						Id:     "1",
					},
				},
			}

			b, err := srv.CreateBuild(ctx, req)
			So(err, ShouldBeNil)
			So(b, ShouldNotBeNil)
			So(sch.Tasks(), ShouldHaveLength, 3)

			// Check datastore.
			bld := &model.Build{ID: b.Id}
			So(datastore.Get(ctx, bld), ShouldBeNil)
		})

		Convey("fails", func() {
			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "project:bucket", Permission: bbperms.BuildersGet},
					},
				})
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Constraints: &pb.Bucket_Constraints{
						Pools:           []string{"example.pool"},
						ServiceAccounts: []string{"example@account.com"},
					}})
				testutil.PutBuilder(ctx, "project", "bucket", "builder", "")
				bld, err := srv.CreateBuild(ctx, req)
				So(bld, ShouldBeNil)
				So(err, ShouldErrLike, `does not have permission "buildbucket.builds.create" in bucket "project/bucket"`)
			})
		})
	})
}
