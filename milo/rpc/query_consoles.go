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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/pagination/dscursor"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/milo/protoutil"
)

var queryConsolesPageTokenVault = dscursor.NewVault([]byte("luci.milo.v1.MiloInternal.QueryConsoles"))

var queryConsolesPageSize = PageSizeLimiter{
	Max:     100,
	Default: 25,
}

// QueryConsoles implements milopb.MiloInternal service
func (s *MiloInternalService) QueryConsoles(ctx context.Context, req *milopb.QueryConsolesRequest) (_ *milopb.QueryConsolesResponse, err error) {
	// Validate request.
	err = validatesQueryConsolesRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	allowed := true
	// Theoretically, we don't need to protect against unauthorized access since
	// we filters out forbidden projects in the datastore query below. Checking it
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
	cur, err := queryConsolesPageTokenVault.Cursor(ctx, req.PageToken)
	switch err {
	case pagination.ErrInvalidPageToken:
		return nil, appstatus.Error(codes.InvalidArgument, "invalid page token")
	case nil:
		// Continue
	default:
		return nil, err
	}

	pageSize := int(queryConsolesPageSize.Adjust(req.PageSize))

	isProjectAllowed := make(map[string]bool)

	// Construct query.
	q := datastore.NewQuery("Console")
	if req.GetPredicate().GetProject() != "" {
		q = q.Ancestor(datastore.MakeKey(ctx, "Project", req.Predicate.Project))
	} else {
		// Ordinal is only useful within a project. If the consoles are not limited
		// to a single project, sort them by projects first.
		q = q.Order("__key__")
	}
	if req.GetPredicate().GetBuilder() != nil {
		q = q.Eq("Builders", utils.LegacyBuilderIDString(req.Predicate.Builder))
	}
	q = q.Order("Ordinal").Start(cur)

	// Query consoles.
	consoles := make([]*projectconfigpb.Console, 0, pageSize)
	var nextCursor datastore.Cursor
	err = datastore.Run(ctx, q, func(con *projectconfig.Console, getCursor datastore.CursorCB) error {
		proj := con.ProjectID()

		isAllowed, ok := isProjectAllowed[proj]
		if !ok {
			var err error
			isAllowed, err = projectconfig.IsAllowed(ctx, proj)
			if err != nil {
				return err
			}
			isProjectAllowed[proj] = isAllowed
		}
		if !isAllowed {
			return nil
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
			Id:    con.ID,
			Realm: realm,
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
	nextPageToken, err := queryConsolesPageTokenVault.PageToken(ctx, nextCursor)
	if err != nil {
		return nil, err
	}

	return &milopb.QueryConsolesResponse{
		Consoles:      consoles,
		NextPageToken: nextPageToken,
	}, nil
}

func validatesQueryConsolesRequest(req *milopb.QueryConsolesRequest) error {
	err := protoutil.ValidateConsolePredicate(req.Predicate)
	if err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	if req.PageSize < 0 {
		return errors.New("page_size can not be negative")
	}

	return nil
}
