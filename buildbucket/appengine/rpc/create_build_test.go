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
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateCreateBuildRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	Convey("validateCreateBuildRequest", t, func() {
		req := &pb.CreateBuildRequest{
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
												Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_a", Version: "latest"}},
											},
										},
										OnPath: []string{"path_a/bin", "path_a"},
									},
									"path_b": {
										DataType: &pb.InputDataRef_Cipd{
											Cipd: &pb.InputDataRef_CIPD{
												Specs: []*pb.InputDataRef_CIPD_PkgSpec{{Package: "pkg_b", Version: "latest"}},
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
								Key:   "key",
								Value: "value",
							},
						},
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
		wellknownExps := stringset.NewFromSlice("luci.wellknown.exp")

		Convey("works", func() {
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldBeNil)
		})

		Convey("output_only fields are cleared", func() {
			req.Build.Id = 87654321
			_, err := validateCreateBuildRequest(ctx, wellknownExps, req)
			So(err, ShouldBeNil)
			So(req.Build.Id, ShouldEqual, 0)
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
