// Copyright 2023 The LUCI Authors.
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
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbprotoutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/pagination/dscursor"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/milo/internal/bbv1"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/milo/protoutil"
)

var queryConsoleSnapshotsPageTokenVault = dscursor.NewVault([]byte("luci.milo.v1.MiloInternal.QueryConsoleSnapshots"))

var queryConsoleSnapshotsPageSize = PageSizeLimiter{
	Max:     100,
	Default: 25,
}

// QueryConsoleSnapshots implements milopb.MiloInternal service
func (s *MiloInternalService) QueryConsoleSnapshots(ctx context.Context, req *milopb.QueryConsoleSnapshotsRequest) (_ *milopb.QueryConsoleSnapshotsResponse, err error) {
	// Validate request.
	err = validateQueryConsoleSnapshotsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	allowed := true
	// Theoretically, we don't need to protect against unauthorized access since
	// we filter out forbidden projects in the datastore query below. Checking it
	// here allows us to return 404 instead of 200 with an empty response, also
	// prevents unnecessary datastore query.
	if allowed && req.GetPredicate().GetProject() != "" {
		allowed, err = projectconfig.IsAllowed(ctx, req.Predicate.Project)
		if err != nil {
			return nil, err
		}
	}
	// Without this, user may be able to tell which accessible console contains an
	// external builder that they don't have access to. This is not necessarily
	// wrong as the access model around this is not well defined yet. But it's
	// safer to use a stricter restriction.
	if allowed && req.GetPredicate().GetBuilder() != nil {
		allowed, err = projectconfig.IsAllowed(ctx, req.Predicate.Builder.Project)
		if err != nil {
			return nil, err
		}
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in ")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
	}

	// Decode cursor from page token.
	cur, err := queryConsoleSnapshotsPageTokenVault.Cursor(ctx, req.PageToken)
	if errors.Is(err, pagination.ErrInvalidPageToken) {
		return nil, appstatus.Error(codes.InvalidArgument, "invalid page token")
	}
	if err != nil {
		return nil, err
	}

	pageSize := int(queryConsoleSnapshotsPageSize.Adjust(req.PageSize))

	isProjectAllowed := make(map[string]bool)
	checkProjectIsAllowed := func(proj string) (bool, error) {
		isAllowed, ok := isProjectAllowed[proj]
		if !ok {
			var err error
			isAllowed, err = projectconfig.IsAllowed(ctx, proj)
			if err != nil {
				return isAllowed, err
			}
			isProjectAllowed[proj] = isAllowed
		}

		return isAllowed, nil
	}

	allowedRealms, err := auth.QueryRealms(ctx, bbperms.BuildsList, "", nil)
	if err != nil {
		return nil, err
	}
	allowedRealmsSet := stringset.NewFromSlice(allowedRealms...)

	// Construct console query.
	q := datastore.NewQuery("Console").Ancestor(datastore.MakeKey(ctx, "Project", req.Predicate.Project))
	if req.GetPredicate().GetBuilder() != nil {
		q = q.Eq("Builders", utils.LegacyBuilderIDString(req.Predicate.Builder))
	}
	q = q.Order("Ordinal").Start(cur)

	// Query consoles.
	consoles := make([]*projectconfigpb.Console, 0, pageSize)
	var nextCursor datastore.Cursor
	err = datastore.Run(ctx, q, func(con *projectconfig.Console, getCursor datastore.CursorCB) error {
		// Resolve external console.
		if con.Def.ExternalId != "" {
			// If the user doesn't have access to the original project, skip the
			// external console.
			sourceProj := con.ProjectID()
			if allowed, err := checkProjectIsAllowed(sourceProj); err != nil || !allowed {
				return err
			}

			con.Parent = datastore.MakeKey(ctx, "Project", con.Def.ExternalProject)
			con.ID = con.Def.ExternalId
			if err = datastore.Get(ctx, con); err != nil {
				return errors.Annotate(err, "failed to resolve external console").Err()
			}
		}

		proj := con.ProjectID()
		if allowed, err := checkProjectIsAllowed(proj); err != nil || !allowed {
			return err
		}

		// Use the project:@root as realm if the realm is not yet defined for the
		// console.
		// TODO(crbug/1110314): remove this once all consoles have their realm
		// populated. Also implement realm based authentication (instead of project
		// based).
		realm := proj + ":@root"
		if con.Realm != "" {
			realm = con.Realm
		}

		consoles = append(consoles, &projectconfigpb.Console{
			Id:       con.ID,
			Name:     con.Def.Name,
			RepoUrl:  con.Def.RepoUrl,
			Realm:    realm,
			Builders: con.Def.AllowedBuilders(allowedRealmsSet),
		})

		if len(consoles) == pageSize {
			nextCursor, err = getCursor()
			if err != nil {
				return err
			}

			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Construct the next page token.
	nextPageToken, err := queryConsoleSnapshotsPageTokenVault.PageToken(ctx, nextCursor)
	if err != nil {
		return nil, err
	}

	// Keep a map (builder ID -> builder summary) to track duplicates and
	// enable easier look up.
	builderSummariesMap := map[string]*model.BuilderSummary{}

	// Get a list of unique builder summaries.
	builderSummariesList := []*model.BuilderSummary{}
	for _, con := range consoles {
		for _, builder := range con.Builders {
			bid := bbprotoutil.FormatBuilderID(builder.Id)
			legacyBid := utils.LegacyBuilderIDString(builder.Id)
			_, ok := builderSummariesMap[bid]
			if !ok {
				bs := &model.BuilderSummary{BuilderID: legacyBid}
				builderSummariesMap[bid] = bs
				builderSummariesList = append(builderSummariesList, bs)
			}
		}
	}

	// Load all the unique builder summaries from the datastore.
	err = datastore.Get(ctx, builderSummariesList)
	var errs errors.MultiError
	if errors.As(err, &errs) {
		criticalErrs := errors.NewLazyMultiError(len(errs))
		for i, e := range errs {
			// It's Ok if a builder has no record.
			// Filter out and ignore `datastore.ErrNoSuchEntity`.
			if errors.Is(e, datastore.ErrNoSuchEntity) {
				e = nil
			}
			criticalErrs.Assign(i, e)
		}
		if err := criticalErrs.Get(); err != nil {
			return nil, err
		}
	}

	// Construct console snapshots from the builder summaries.
	consoleSnapshots := make([]*milopb.ConsoleSnapshot, 0, len(consoles))
	for _, con := range consoles {
		builderSnapshots := make([]*milopb.BuilderSnapshot, 0, len(con.Builders))
		for _, builder := range con.Builders {
			builderID := bbprotoutil.FormatBuilderID(builder.Id)
			builderSummary := builderSummariesMap[builderID]

			// The builder has no record because it's new or has been inactive for too
			// long. Use a nil build to represent that.
			if builderSummary.LastFinishedBuildID == "" {
				builderSnapshots = append(builderSnapshots, &milopb.BuilderSnapshot{
					Builder: builder.Id,
					Build:   nil,
				})
				continue
			}

			buildAddress := strings.TrimPrefix(builderSummary.LastFinishedBuildID, "buildbucket/")
			buildID, _, _, _, buildNum, err := bbv1.ParseBuildAddress(buildAddress)
			if err != nil {
				return nil, err
			}

			builderSnapshots = append(builderSnapshots, &milopb.BuilderSnapshot{
				Builder: builder.Id,
				Build: &buildbucketpb.Build{
					Id:      buildID,
					Builder: builder.Id,
					Number:  int32(buildNum),
					Status:  builderSummary.LastFinishedStatus.ToBuildbucket(),
				},
			})
		}
		consoleSnapshots = append(consoleSnapshots, &milopb.ConsoleSnapshot{
			Console:          con,
			BuilderSnapshots: builderSnapshots,
		})
	}

	return &milopb.QueryConsoleSnapshotsResponse{
		Snapshots:     consoleSnapshots,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryConsoleSnapshotsRequest(req *milopb.QueryConsoleSnapshotsRequest) error {
	err := protoutil.ValidateConsolePredicate(req.Predicate)
	if err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	if req.GetPredicate().GetProject() == "" {
		return errors.Reason("predicate.project is required").Err()
	}

	if req.PageSize < 0 {
		return errors.Reason("page_size can not be negative").Err()
	}

	return nil
}

func init() {
	bbperms.BuildsList.AddFlags(realms.UsedInQueryRealms)
}
