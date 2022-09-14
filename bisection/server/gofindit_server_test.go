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

// package server implements the server to handle pRPC requests.
package server

import (
	"context"
	"testing"

	"go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
)

func TestQueryAnalysis(t *testing.T) {
	t.Parallel()
	server := &GoFinditServer{}
	c := memory.Use(context.Background())

	Convey("No BuildFailure Info", t, func() {
		req := &gfipb.QueryAnalysisRequest{}
		_, err := server.QueryAnalysis(c, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)
	})

	Convey("No bbid", t, func() {
		req := &gfipb.QueryAnalysisRequest{BuildFailure: &gfipb.BuildFailure{}}
		_, err := server.QueryAnalysis(c, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)
	})

	Convey("Unsupported step", t, func() {
		req := &gfipb.QueryAnalysisRequest{
			BuildFailure: &gfipb.BuildFailure{
				FailedStepName: "some step",
				Bbid:           123,
			},
		}
		_, err := server.QueryAnalysis(c, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.Unimplemented)
	})

	Convey("No analysis found", t, func() {
		req := &gfipb.QueryAnalysisRequest{
			BuildFailure: &gfipb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}
		_, err := server.QueryAnalysis(c, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.NotFound)
	})

	Convey("Analysis found", t, func() {
		// Prepares datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				Project: "chromium/test",
				Bucket:  "ci",
				Builder: "android",
			},
			BuildFailureType: gfipb.BuildFailureType_COMPILE,
		}
		So(datastore.Put(c, failedBuild), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, failedBuild),
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			CompileFailure:     datastore.KeyForObj(c, compileFailure),
			FirstFailedBuildId: 119,
		}
		So(datastore.Put(c, compileFailureAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		req := &gfipb.QueryAnalysisRequest{
			BuildFailure: &gfipb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}

		res, err := server.QueryAnalysis(c, req)
		So(err, ShouldBeNil)
		So(len(res.Analyses), ShouldEqual, 1)

		analysis := res.Analyses[0]
		So(analysis.Builder, ShouldResemble, &buildbucketpb.BuilderID{
			Project: "chromium/test",
			Bucket:  "ci",
			Builder: "android",
		})
		So(analysis.BuildFailureType, ShouldEqual, gfipb.BuildFailureType_COMPILE)
	})

	Convey("Analysis found for a similar failure", t, func() {
		// Prepares datastore
		basedFailedBuild := &model.LuciFailedBuild{
			Id: 122,
		}
		So(datastore.Put(c, basedFailedBuild), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		basedCompileFailure := &model.CompileFailure{
			Id:    122,
			Build: datastore.KeyForObj(c, basedFailedBuild),
		}
		So(datastore.Put(c, basedCompileFailure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		failedBuild := &model.LuciFailedBuild{
			Id: 123,
		}
		So(datastore.Put(c, failedBuild), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:               123,
			Build:            datastore.KeyForObj(c, failedBuild),
			MergedFailureKey: datastore.KeyForObj(c, basedCompileFailure),
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, basedCompileFailure),
		}
		So(datastore.Put(c, compileFailureAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		req := &gfipb.QueryAnalysisRequest{
			BuildFailure: &gfipb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}

		res, err := server.QueryAnalysis(c, req)
		So(err, ShouldBeNil)
		So(len(res.Analyses), ShouldEqual, 1)
	})

}

func TestListAnalyses(t *testing.T) {
	t.Parallel()
	server := &GoFinditServer{}

	Convey("List existing analyses", t, func() {
		// Set up context and AEAD so that page tokens can be generated
		c := memory.Use(context.Background())
		keyHandle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		testAEAD, err := aead.New(keyHandle)
		So(err, ShouldBeNil)
		c = secrets.SetPrimaryTinkAEADForTest(c, testAEAD)

		// Prepares datastore
		failureAnalysis1 := &model.CompileFailureAnalysis{
			Id:         1,
			CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
		}
		failureAnalysis2 := &model.CompileFailureAnalysis{
			Id:         2,
			CreateTime: (&timestamppb.Timestamp{Seconds: 102}).AsTime(),
		}
		failureAnalysis3 := &model.CompileFailureAnalysis{
			Id:         3,
			CreateTime: (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
		}
		failureAnalysis4 := &model.CompileFailureAnalysis{
			Id:         4,
			CreateTime: (&timestamppb.Timestamp{Seconds: 103}).AsTime(),
		}
		So(datastore.Put(c, failureAnalysis1), ShouldBeNil)
		So(datastore.Put(c, failureAnalysis2), ShouldBeNil)
		So(datastore.Put(c, failureAnalysis3), ShouldBeNil)
		So(datastore.Put(c, failureAnalysis4), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("Invalid page size", func() {
			req := &gfipb.ListAnalysesRequest{
				PageSize: -5,
			}
			_, err := server.ListAnalyses(c, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)
		})

		Convey("Specifying page size is optional", func() {
			req := &gfipb.ListAnalysesRequest{}
			res, err := server.ListAnalyses(c, req)
			So(err, ShouldBeNil)
			So(len(res.Analyses), ShouldEqual, 4)

			Convey("Next page token is empty if there are no more analyses", func() {
				So(res.NextPageToken, ShouldEqual, "")
			})
		})

		Convey("Response is limited by the page size", func() {
			req := &gfipb.ListAnalysesRequest{
				PageSize: 3,
			}
			res, err := server.ListAnalyses(c, req)
			So(err, ShouldBeNil)
			So(len(res.Analyses), ShouldEqual, req.PageSize)
			So(res.NextPageToken, ShouldNotEqual, "")

			Convey("Returned analyses are sorted correctly", func() {
				So(res.Analyses[0].AnalysisId, ShouldEqual, 4)
				So(res.Analyses[1].AnalysisId, ShouldEqual, 2)
				So(res.Analyses[2].AnalysisId, ShouldEqual, 3)

				Convey("Page token will get the next page of analyses", func() {
					req = &gfipb.ListAnalysesRequest{
						PageSize:  3,
						PageToken: res.NextPageToken,
					}
					res, err = server.ListAnalyses(c, req)
					So(err, ShouldBeNil)
					So(len(res.Analyses), ShouldEqual, 1)
					So(res.Analyses[0].AnalysisId, ShouldEqual, 1)
				})
			})
		})
	})
}
