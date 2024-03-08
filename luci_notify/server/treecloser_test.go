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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apiconfig "go.chromium.org/luci/luci_notify/api/config"
	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateRequest(t *testing.T) {
	t.Parallel()
	Convey("Validate request", t, func() {
		req := &pb.CheckTreeCloserRequest{}
		So(validateRequest(req), ShouldNotBeNil)
		req.Project = "project"
		So(validateRequest(req), ShouldNotBeNil)
		req.Bucket = "bucket"
		So(validateRequest(req), ShouldNotBeNil)
		req.Builder = "builder"
		So(validateRequest(req), ShouldNotBeNil)
		req.Step = "step"
		So(validateRequest(req), ShouldBeNil)
	})
}

func TestStepMatchesRule(t *testing.T) {
	t.Parallel()
	Convey("Step Matches Rule", t, func() {
		stepName := "compile"
		So(stepMatchesRule(stepName, "", ""), ShouldBeTrue)
		So(stepMatchesRule(stepName, "compile", ""), ShouldBeTrue)
		So(stepMatchesRule(stepName, "noncompile", ""), ShouldBeFalse)
		So(stepMatchesRule(stepName, "", "compile"), ShouldBeFalse)
		So(stepMatchesRule(stepName, "", "noncompile"), ShouldBeTrue)
	})
}

func TestCheckTreeCloser(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("CheckTreeCloser", t, func() {
		project := &config.Project{
			Name: "chromium",
		}
		So(datastore.Put(c, project), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		builder := &config.Builder{
			ProjectKey: datastore.KeyForObj(c, project),
			ID:         "ci/Android ASAN",
		}
		So(datastore.Put(c, builder), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		treeCloser := &config.TreeCloser{
			BuilderKey: datastore.KeyForObj(c, builder),
			TreeCloser: apiconfig.TreeCloser{
				FailedStepRegexp: "compile",
			},
		}

		So(datastore.Put(c, treeCloser), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		server := &TreeCloserServer{}
		req := &pb.CheckTreeCloserRequest{}
		_, err := server.CheckTreeCloser(c, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)

		req = &pb.CheckTreeCloserRequest{
			Project: "x",
			Bucket:  "y",
			Builder: "z",
			Step:    "s",
		}
		res, err := server.CheckTreeCloser(c, req)
		So(err, ShouldBeNil)
		So(res.IsTreeCloser, ShouldEqual, false)

		req = &pb.CheckTreeCloserRequest{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "Android ASAN",
			Step:    "s",
		}
		res, err = server.CheckTreeCloser(c, req)
		So(err, ShouldBeNil)
		So(res.IsTreeCloser, ShouldEqual, false)

		req = &pb.CheckTreeCloserRequest{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "Android ASAN",
			Step:    "compile",
		}
		res, err = server.CheckTreeCloser(c, req)
		So(err, ShouldBeNil)
		So(res.IsTreeCloser, ShouldEqual, true)
	})
}
