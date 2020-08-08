// Copyright 2020 The LUCI Authors.
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
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateCancel validates the given request.
func validateCancel(req *pb.CancelBuildRequest) error {
	switch {
	case req.GetId() == 0:
		return appstatus.Errorf(codes.InvalidArgument, "id is required")
	case req.SummaryMarkdown == "":
		return appstatus.Errorf(codes.InvalidArgument, "summary_markdown is required")
	}
	return nil
}

// CancelBuild handles a request to cancel a build. Implements pb.BuildsServer.
func (*Builds) CancelBuild(ctx context.Context, req *pb.CancelBuildRequest) (*pb.Build, error) {
	if err := validateCancel(req); err != nil {
		return nil, err
	}
	m, err := getFieldMask(req.Fields)
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid field mask")
	}

	bld, err := getBuild(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if err := perm.HasInBuilder(ctx, perm.BuildsCancel, bld.Proto.Builder); err != nil {
		return nil, err
	}

	// If the build has ended, there's nothing to cancel.
	if protoutil.IsEnded(bld.Proto.Status) {
		return bld.ToProto(ctx, m)
	}

	dsp := tasks.GetDispatcher(ctx)
	inf := &model.BuildInfra{
		ID:    1,
		Build: datastore.KeyForObj(ctx, bld),
	}
	stp := &model.BuildSteps{
		ID:    1,
		Build: inf.Build,
	}
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Get(ctx, bld, inf, stp); err != nil {
			switch merr, ok := err.(errors.MultiError); {
			case !ok:
				return errors.Annotate(err, "failed to fetch build: %d", bld.ID).Err()
			case merr[0] == datastore.ErrNoSuchEntity:
				return perm.NotFoundErr(ctx)
			case merr[0] != nil:
				return errors.Annotate(merr[0], "failed to fetch build: %d", bld.ID).Err()
			case merr[1] != nil && merr[1] != datastore.ErrNoSuchEntity:
				return errors.Annotate(merr[1], "failed to fetch build infra: %d", bld.ID).Err()
			case merr[2] != nil && merr[2] != datastore.ErrNoSuchEntity:
				return errors.Annotate(merr[2], "failed to fetch build steps: %d", bld.ID).Err()
			case protoutil.IsEnded(bld.Proto.Status):
				return nil
			case merr[2] == datastore.ErrNoSuchEntity:
				stp = nil
			}
		}
		if inf.Proto.Swarming.GetHostname() != "" && inf.Proto.Swarming.TaskId != "" {
			// TODO(crbug/1091604): Pass the realm if the build is realms-enabled.
			if err := dsp.CancelSwarmingTask(ctx, &taskdefs.CancelSwarmingTask{
				Hostname: inf.Proto.Swarming.Hostname,
				TaskId:   inf.Proto.Swarming.TaskId,
			}); err != nil {
				return errors.Annotate(err, "failed to enqueue swarming task cancellation task: %d", bld.ID).Err()
			}
		}

		bld.Leasee = nil
		bld.LeaseExpirationDate = time.Time{}
		bld.LeaseKey = 0
		bld.StatusChangedTime = clock.Now(ctx).UTC()

		bld.Proto.CanceledBy = string(auth.CurrentIdentity(ctx))
		bld.Proto.EndTime = google.NewTimestamp(bld.StatusChangedTime)
		bld.Proto.Status = pb.Status_CANCELED
		bld.Proto.SummaryMarkdown = req.SummaryMarkdown

		toPut := []interface{}{bld}

		if stp != nil {
			switch changed, err := stp.CancelIncomplete(ctx, bld.Proto.EndTime); {
			case err != nil:
				return errors.Annotate(err, "failed to fetch build steps: %d", bld.ID).Err()
			case changed:
				toPut = append(toPut, stp)
			}
		}

		if err := datastore.Put(ctx, toPut...); err != nil {
			return errors.Annotate(err, "failed to store build: %d", bld.ID).Err()
		}
		// TODO(crbug/1042991): Enqueue BigQuery, Pub/Sub, and ResultDB-related tasks.
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}

	return bld.ToProto(ctx, m)
}
