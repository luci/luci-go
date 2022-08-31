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

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
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
		failed_build := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				Project: "chromium/test",
				Bucket:  "ci",
				Builder: "android",
			},
			BuildFailureType: gfipb.BuildFailureType_COMPILE,
		}
		So(datastore.Put(c, failed_build), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compile_failure := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, failed_build),
		}
		So(datastore.Put(c, compile_failure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compile_failure_analysis := &model.CompileFailureAnalysis{
			CompileFailure:     datastore.KeyForObj(c, compile_failure),
			FirstFailedBuildId: 123,
		}
		So(datastore.Put(c, compile_failure_analysis), ShouldBeNil)
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
		based_failed_build := &model.LuciFailedBuild{
			Id: 122,
		}
		So(datastore.Put(c, based_failed_build), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		based_compile_failure := &model.CompileFailure{
			Id:    122,
			Build: datastore.KeyForObj(c, based_failed_build),
		}
		So(datastore.Put(c, based_compile_failure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		failed_build := &model.LuciFailedBuild{
			Id: 123,
		}
		So(datastore.Put(c, failed_build), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compile_failure := &model.CompileFailure{
			Id:               123,
			Build:            datastore.KeyForObj(c, failed_build),
			MergedFailureKey: datastore.KeyForObj(c, based_compile_failure),
		}
		So(datastore.Put(c, compile_failure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compile_failure_analysis := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, based_compile_failure),
		}
		So(datastore.Put(c, compile_failure_analysis), ShouldBeNil)
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
