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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apiconfig "go.chromium.org/luci/luci_notify/api/config"
	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/config"
)

func TestValidateRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("Validate request", t, func(t *ftt.Test) {
		req := &pb.CheckTreeCloserRequest{}
		assert.Loosely(t, validateRequest(req), should.NotBeNil)
		req.Project = "project"
		assert.Loosely(t, validateRequest(req), should.NotBeNil)
		req.Bucket = "bucket"
		assert.Loosely(t, validateRequest(req), should.NotBeNil)
		req.Builder = "builder"
		assert.Loosely(t, validateRequest(req), should.NotBeNil)
		req.Step = "step"
		assert.Loosely(t, validateRequest(req), should.BeNil)
	})
}

func TestStepMatchesRule(t *testing.T) {
	t.Parallel()
	ftt.Run("Step Matches Rule", t, func(t *ftt.Test) {
		stepName := "compile"
		assert.Loosely(t, stepMatchesRule(stepName, "", ""), should.BeTrue)
		assert.Loosely(t, stepMatchesRule(stepName, "compile", ""), should.BeTrue)
		assert.Loosely(t, stepMatchesRule(stepName, "noncompile", ""), should.BeFalse)
		assert.Loosely(t, stepMatchesRule(stepName, "", "compile"), should.BeFalse)
		assert.Loosely(t, stepMatchesRule(stepName, "", "noncompile"), should.BeTrue)
	})
}

func TestCheckTreeCloser(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("CheckTreeCloser", t, func(t *ftt.Test) {
		project := &config.Project{
			Name: "chromium",
		}
		assert.Loosely(t, datastore.Put(c, project), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		builder := &config.Builder{
			ProjectKey: datastore.KeyForObj(c, project),
			ID:         "ci/Android ASAN",
		}
		assert.Loosely(t, datastore.Put(c, builder), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		treeCloser := &config.TreeCloser{
			BuilderKey: datastore.KeyForObj(c, builder),
			TreeCloser: apiconfig.TreeCloser{
				FailedStepRegexp: "compile",
			},
		}

		assert.Loosely(t, datastore.Put(c, treeCloser), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		server := &TreeCloserServer{}
		req := &pb.CheckTreeCloserRequest{}
		_, err := server.CheckTreeCloser(c, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))

		req = &pb.CheckTreeCloserRequest{
			Project: "x",
			Bucket:  "y",
			Builder: "z",
			Step:    "s",
		}
		res, err := server.CheckTreeCloser(c, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.IsTreeCloser, should.Equal(false))

		req = &pb.CheckTreeCloserRequest{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "Android ASAN",
			Step:    "s",
		}
		res, err = server.CheckTreeCloser(c, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.IsTreeCloser, should.Equal(false))

		req = &pb.CheckTreeCloserRequest{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "Android ASAN",
			Step:    "compile",
		}
		res, err = server.CheckTreeCloser(c, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.IsTreeCloser, should.Equal(true))
	})
}
