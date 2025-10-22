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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

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

func TestValidateNotifyCulpritRevertRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("Validate NotifyCulpritRevert request", t, func(t *ftt.Test) {
		now := timestamppb.New(time.Now())

		t.Run("empty tree_name", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "",
				RevertLandTime:   now,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("tree_name"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("unspecified"))
		})

		t.Run("nil revert_land_time", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   nil,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("revert_land_time"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("unspecified"))
		})

		t.Run("empty culprit_review_url", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   now,
				CulpritReviewUrl: "",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("culprit_review_url"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("unspecified"))
		})

		t.Run("empty revert_review_url", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   now,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("revert_review_url"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("unspecified"))
		})

		t.Run("invalid tree_name pattern", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "invalid tree name!",
				RevertLandTime:   now,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("tree_name"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("must match pattern"))
		})

		t.Run("culprit_review_url too long", func(t *ftt.Test) {
			longURL := "https://example.com/" + string(make([]byte, 2100))
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   now,
				CulpritReviewUrl: longURL,
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("culprit_review_url"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("exceeds maximum length"))
		})

		t.Run("revert_review_url too long", func(t *ftt.Test) {
			longURL := "https://example.com/" + string(make([]byte, 2100))
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   now,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  longURL,
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("revert_review_url"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("exceeds maximum length"))
		})

		t.Run("revert_land_time too far in future", func(t *ftt.Test) {
			futureTime := timestamppb.New(time.Now().Add(1 * time.Minute))
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   futureTime,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("revert_land_time"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("must not be more than"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("in the future"))
		})

		t.Run("valid request", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   now,
				CulpritReviewUrl: "https://example.com/culprit",
				RevertReviewUrl:  "https://example.com/revert",
			}
			err := validateNotifyCulpritRevertRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestNotifyCulpritRevert(t *testing.T) {
	t.Parallel()

	ftt.Run("NotifyCulpritRevert", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		server := &TreeCloserServer{}
		now := time.Now().UTC()
		nowPb := timestamppb.New(now)

		t.Run("invalid request - missing fields", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{}
			_, err := server.NotifyCulpritRevert(c, req)
			assert.Loosely(t, err, should.NotBeNil)
			// Check the appstatus code directly since the error hasn't gone through GRPCifyAndLog
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
		})

		t.Run("successfully creates CulpritRevertEvent", func(t *ftt.Test) {
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   nowPb,
				CulpritReviewUrl: "https://chromium-review.googlesource.com/c/chromium/src/+/12345",
				RevertReviewUrl:  "https://chromium-review.googlesource.com/c/chromium/src/+/12346",
			}

			res, err := server.NotifyCulpritRevert(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)

			// Verify the event was created in datastore
			event := &config.CulpritRevertEvent{TreeName: "chromium"}
			err = datastore.Get(c, event)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, event.TreeName, should.Equal("chromium"))
			assert.Loosely(t, event.RevertLandTime.Unix(), should.Equal(now.Unix()))
			assert.Loosely(t, event.CulpritReviewURL, should.Equal("https://chromium-review.googlesource.com/c/chromium/src/+/12345"))
			assert.Loosely(t, event.RevertReviewURL, should.Equal("https://chromium-review.googlesource.com/c/chromium/src/+/12346"))
			assert.Loosely(t, event.CreatedAt.IsZero(), should.BeFalse)
		})

		t.Run("overwrites existing event when new revert is more recent", func(t *ftt.Test) {
			// Create an initial event
			initialEvent := &config.CulpritRevertEvent{
				TreeName:         "chromium",
				RevertLandTime:   now.Add(-1 * time.Hour).UTC(),
				CulpritReviewURL: "https://example.com/old-culprit",
				RevertReviewURL:  "https://example.com/old-revert",
				CreatedAt:        now.Add(-1 * time.Hour).UTC(),
			}
			assert.Loosely(t, datastore.Put(c, initialEvent), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Send new notification with more recent revert time
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   nowPb,
				CulpritReviewUrl: "https://example.com/new-culprit",
				RevertReviewUrl:  "https://example.com/new-revert",
			}

			res, err := server.NotifyCulpritRevert(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)

			// Verify the event was overwritten
			event := &config.CulpritRevertEvent{TreeName: "chromium"}
			err = datastore.Get(c, event)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, event.CulpritReviewURL, should.Equal("https://example.com/new-culprit"))
			assert.Loosely(t, event.RevertReviewURL, should.Equal("https://example.com/new-revert"))
			assert.Loosely(t, event.RevertLandTime.Unix(), should.Equal(now.Unix()))
		})

		t.Run("does NOT overwrite existing event when new revert is older", func(t *ftt.Test) {
			// Create a recent event
			recentEvent := &config.CulpritRevertEvent{
				TreeName:         "chromium",
				RevertLandTime:   now.UTC(),
				CulpritReviewURL: "https://example.com/recent-culprit",
				RevertReviewURL:  "https://example.com/recent-revert",
				CreatedAt:        now.UTC(),
			}
			assert.Loosely(t, datastore.Put(c, recentEvent), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Try to send notification with older revert time
			olderTimePb := timestamppb.New(now.Add(-1 * time.Hour))
			req := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   olderTimePb,
				CulpritReviewUrl: "https://example.com/old-culprit",
				RevertReviewUrl:  "https://example.com/old-revert",
			}

			res, err := server.NotifyCulpritRevert(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)

			// Verify the event was NOT overwritten
			event := &config.CulpritRevertEvent{TreeName: "chromium"}
			err = datastore.Get(c, event)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, event.CulpritReviewURL, should.Equal("https://example.com/recent-culprit"))
			assert.Loosely(t, event.RevertReviewURL, should.Equal("https://example.com/recent-revert"))
			assert.Loosely(t, event.RevertLandTime.Unix(), should.Equal(now.Unix()))
		})

		t.Run("multiple trees can have separate events", func(t *ftt.Test) {
			req1 := &pb.NotifyCulpritRevertRequest{
				TreeName:         "chromium",
				RevertLandTime:   nowPb,
				CulpritReviewUrl: "https://example.com/chromium-culprit",
				RevertReviewUrl:  "https://example.com/chromium-revert",
			}
			req2 := &pb.NotifyCulpritRevertRequest{
				TreeName:         "angle",
				RevertLandTime:   nowPb,
				CulpritReviewUrl: "https://example.com/angle-culprit",
				RevertReviewUrl:  "https://example.com/angle-revert",
			}

			_, err := server.NotifyCulpritRevert(c, req1)
			assert.Loosely(t, err, should.BeNil)
			_, err = server.NotifyCulpritRevert(c, req2)
			assert.Loosely(t, err, should.BeNil)

			// Verify both events exist independently
			chromiumEvent := &config.CulpritRevertEvent{TreeName: "chromium"}
			err = datastore.Get(c, chromiumEvent)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chromiumEvent.CulpritReviewURL, should.Equal("https://example.com/chromium-culprit"))

			angleEvent := &config.CulpritRevertEvent{TreeName: "angle"}
			err = datastore.Get(c, angleEvent)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, angleEvent.CulpritReviewURL, should.Equal("https://example.com/angle-culprit"))
		})
	})
}
