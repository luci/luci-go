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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func TestCollect(t *testing.T) {
	t.Parallel()

	Convey("Collect", t, func() {
		Convey("writeBuildDetails", func() {
			file, err := ioutil.TempFile("", "builds")
			So(err, ShouldBeNil)
			file.Close()
			defer os.Remove(file.Name())

			writeBuildDetails([]int64{123, 456}, map[int64]*buildbucketpb.Build{
				123: {Id: 123},
				456: {Id: 456},
			}, file.Name())

			buf, err := ioutil.ReadFile(file.Name())
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "[{\"id\":\"123\"},{\"id\":\"456\"}]\n")
		})

		Convey("collectBuildDetails", func() {
			ctx := context.Background()
			sleepCalls := 0
			buildsMock := buildbucketpb.NewMockBuildsClient(gomock.NewController(t))

			batchMock := buildsMock.EXPECT().Batch(
				gomock.Any(),
				&buildbucketpb.BatchRequest{
					Requests: []*buildbucketpb.BatchRequest_Request{
						{
							Request: &buildbucketpb.BatchRequest_Request_GetBuild{
								GetBuild: &buildbucketpb.GetBuildRequest{
									Id:     123,
									Fields: getRequestFieldMask,
								},
							},
						},
						{
							Request: &buildbucketpb.BatchRequest_Request_GetBuild{
								GetBuild: &buildbucketpb.GetBuildRequest{
									Id:     456,
									Fields: getRequestFieldMask,
								},
							},
						},
					},
				},
			)

			Convey("all builds are ended from the start", func() {
				batchMock.Return(&buildbucketpb.BatchResponse{
					Responses: []*buildbucketpb.BatchResponse_Response{
						// The order of responses to batch request is intentionally
						// different from the order of requests. This should still be
						// working as builds are matched by returned ID.
						{
							Response: &buildbucketpb.BatchResponse_Response_GetBuild{
								GetBuild: &buildbucketpb.Build{
									Id:     456,
									Status: buildbucketpb.Status_SUCCESS,
								},
							},
						},
						{
							Response: &buildbucketpb.BatchResponse_Response_GetBuild{
								GetBuild: &buildbucketpb.Build{
									Id:     123,
									Status: buildbucketpb.Status_FAILURE,
								},
							},
						},
					},
				}, nil)

				collectBuildDetails(ctx, buildsMock, []int64{123, 456}, func() {
					sleepCalls++
				})

				So(sleepCalls, ShouldEqual, 0)
			})

			Convey("one build ended on a second request", func() {
				batchMock.Return(&buildbucketpb.BatchResponse{
					Responses: []*buildbucketpb.BatchResponse_Response{
						{
							Response: &buildbucketpb.BatchResponse_Response_GetBuild{
								GetBuild: &buildbucketpb.Build{
									Id:     123,
									Status: buildbucketpb.Status_SCHEDULED,
								},
							},
						},
						{
							Response: &buildbucketpb.BatchResponse_Response_GetBuild{
								GetBuild: &buildbucketpb.Build{
									Id:     456,
									Status: buildbucketpb.Status_SUCCESS,
								},
							},
						},
					},
				}, nil)

				buildsMock.EXPECT().Batch(
					gomock.Any(),
					&buildbucketpb.BatchRequest{
						Requests: []*buildbucketpb.BatchRequest_Request{
							{
								Request: &buildbucketpb.BatchRequest_Request_GetBuild{
									GetBuild: &buildbucketpb.GetBuildRequest{
										Id:     123,
										Fields: getRequestFieldMask,
									},
								},
							},
						},
					},
				).Return(&buildbucketpb.BatchResponse{
					Responses: []*buildbucketpb.BatchResponse_Response{
						{
							Response: &buildbucketpb.BatchResponse_Response_GetBuild{
								GetBuild: &buildbucketpb.Build{
									Id:     123,
									Status: buildbucketpb.Status_INFRA_FAILURE,
								},
							},
						},
					},
				}, nil)

				collectBuildDetails(ctx, buildsMock, []int64{123, 456}, func() {
					sleepCalls++
				})

				So(sleepCalls, ShouldEqual, 1)
			})
		})
	})
}
