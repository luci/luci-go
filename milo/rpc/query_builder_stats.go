// Copyright 2021 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

// QueryBuilderStats implements milopb.MiloInternal service
func (s *MiloInternalService) QueryBuilderStats(ctx context.Context, req *milopb.QueryBuilderStatsRequest) (_ *milopb.BuilderStats, err error) {
	// Validate request.
	err = validatesQueryBuilderStatsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Perform ACL check.
	realm := realms.Join(req.Builder.Project, req.Builder.Bucket)
	allowed, err := auth.HasPermission(ctx, bbperms.BuildsList, realm, nil)
	if err != nil {
		return nil, err
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the bucket")
	}

	legacyBuilderID := utils.LegacyBuilderIDString(req.Builder)
	stats := &milopb.BuilderStats{}

	err = parallel.FanOutIn(func(fetch chan<- func() error) {

		// Pending builds
		fetch <- func() error {
			q := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", legacyBuilderID).
				Eq("Summary.Status", milostatus.NotRun)
			pending, err := datastore.Count(ctx, q)
			stats.PendingBuildsCount = int32(pending)
			return err
		}

		// Running builds
		fetch <- func() error {
			q := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", legacyBuilderID).
				Eq("Summary.Status", milostatus.Running)
			running, err := datastore.Count(ctx, q)
			stats.RunningBuildsCount = int32(running)
			return err
		}
	})

	if err != nil {
		return nil, err
	}

	return stats, nil
}

func validatesQueryBuilderStatsRequest(req *milopb.QueryBuilderStatsRequest) error {
	if err := protoutil.ValidateRequiredBuilderID(req.Builder); err != nil {
		return errors.Annotate(err, "builder").Err()
	}
	return nil
}
