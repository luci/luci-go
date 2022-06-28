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

package backend

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

var listBuildersPageSize = PageSizeLimiter{
	Max:     1000,
	Default: 100,
}

// ListBuilders implements milopb.MiloInternal service
func (s *MiloInternalService) ListBuilders(ctx context.Context, req *milopb.ListBuildersRequest) (_ *milopb.ListBuildersResponse, err error) {
	// Validate request.
	err = validateListBuildersRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Validate and get page token.
	pageToken, err := validateListBuildersPageToken(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Perform ACL check.
	allowed, err := common.IsAllowed(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
	}

	pageSize := int(listBuildersPageSize.Adjust(req.PageSize))

	if req.Group == "" {
		return s.listProjectBuilders(ctx, req.Project, pageSize, pageToken)
	}
	return s.listGroupBuilders(ctx, req.Project, req.Group, pageSize, pageToken)
}

func (s *MiloInternalService) listProjectBuilders(ctx context.Context, project string, pageSize int, pageToken *milopb.ListBuildersPageToken) (_ *milopb.ListBuildersResponse, err error) {
	res := &milopb.ListBuildersResponse{}

	// First, query buildbucket and return all builders defined in the project.
	if pageToken == nil || pageToken.NextBuildbucketPageToken != "" {
		buildersClient, err := s.GetBuildersClient(ctx, auth.AsCredentialsForwarder)
		if err != nil {
			return nil, err
		}

		bbRes, err := buildersClient.ListBuilders(ctx, &buildbucketpb.ListBuildersRequest{
			Project:   project,
			PageSize:  int32(pageSize),
			PageToken: pageToken.GetNextBuildbucketPageToken(),
		})
		if err != nil {
			return nil, err
		}

		for _, builder := range bbRes.Builders {
			res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{
				Id: builder.Id,
			})
		}

		// If there are more internal builders, populate `res.NextPageToken` and
		// return.
		if bbRes.NextPageToken != "" {
			nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
				NextBuildbucketPageToken: bbRes.NextPageToken,
			})
			if err != nil {
				return nil, err
			}

			res.NextPageToken = nextPageToken
			return res, nil
		}
	}

	// Then, return external builders referenced in the project consoles.
	cachedPerms := make(map[string]bool)
	remaining := pageSize - len(res.Builders)
	if remaining > 0 {
		project := &common.Project{ID: project}
		if err := datastore.Get(ctx, project); err != nil {
			return nil, err
		}

		externalBuilders := make([]*buildbucketpb.BuilderID, len(project.ExternalBuilderIDs))
		for i, externalBuilderID := range project.ExternalBuilderIDs {
			externalBuilders[i], err = common.ParseBuilderID(externalBuilderID)
			if err != nil {
				return nil, err
			}
		}

		// Append up to `remaining` external builders to `res.Builders`.
		i := int(pageToken.GetNextMiloBuilderIndex())
		for ; i < len(project.ExternalBuilderIDs) && remaining > 0; i++ {
			bid := externalBuilders[i]

			realm := realms.Join(bid.Project, bid.Bucket)
			allowed, ok := cachedPerms[realm]
			if !ok {
				allowed, err = auth.HasPermission(ctx, bbperms.BuildersList, realm, nil)
				if err != nil {
					return nil, err
				}
				cachedPerms[realm] = allowed
			}

			if allowed {
				res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{Id: bid})
				remaining--
			}
		}

		// Populate `res.NextPageToken`.
		if i < len(project.ExternalBuilderIDs) {
			nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
				NextMiloBuilderIndex: int32(i),
			})
			if err != nil {
				return nil, err
			}

			res.NextPageToken = nextPageToken
		}
	}

	return res, nil
}

func (s *MiloInternalService) listGroupBuilders(ctx context.Context, project string, group string, pageSize int, pageToken *milopb.ListBuildersPageToken) (_ *milopb.ListBuildersResponse, err error) {
	res := &milopb.ListBuildersResponse{}

	projKey := datastore.MakeKey(ctx, "Project", project)
	con := common.Console{Parent: projKey, ID: group}
	switch err := datastore.Get(ctx, &con); err {
	case nil:
	case datastore.ErrNoSuchEntity:
		return nil, appstatus.Error(codes.NotFound, "group not found")
	default:
		return nil, errors.Annotate(err, "error getting console %s in project %s", group, project).Err()
	}

	// Sort con.Builders. with Internal builders come before external builders.
	internalBuilderIDPrefix := "buildbucket/luci." + project + "."
	sort.Slice(con.Builders, func(i, j int) bool {
		builder1InternalFlag := 1
		builder2InternalFlag := 1
		if strings.HasPrefix(con.Builders[i], internalBuilderIDPrefix) {
			builder1InternalFlag = 0
		}
		if strings.HasPrefix(con.Builders[j], internalBuilderIDPrefix) {
			builder2InternalFlag = 0
		}

		if builder1InternalFlag != builder2InternalFlag {
			return builder1InternalFlag < builder2InternalFlag
		}

		return con.Builders[i] < con.Builders[j]
	})

	builders := make([]*buildbucketpb.BuilderID, len(con.Builders))
	for i, bid := range con.Builders {
		builders[i], err = common.ParseLegacyBuilderID(bid)
		if err != nil {
			return nil, err
		}
	}

	// Populate `res.Builders`.
	cachedPerms := make(map[string]bool)
	i := int(pageToken.GetNextMiloBuilderIndex())
	remaining := pageSize
	for ; i < len(con.Builders) && remaining > 0; i++ {
		bid := builders[i]
		realm := realms.Join(bid.Project, bid.Bucket)
		allowed, ok := cachedPerms[realm]
		if !ok {
			allowed, err = auth.HasPermission(ctx, bbperms.BuildersList, realm, nil)
			if err != nil {
				return nil, err
			}
			cachedPerms[realm] = allowed
		}

		if allowed {
			res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{Id: bid})
			remaining--
		}
	}

	// Populate `res.NextPageToken`.
	if i < len(con.Builders) {
		nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
			NextMiloBuilderIndex: int32(i),
		})
		if err != nil {
			return nil, err
		}

		res.NextPageToken = nextPageToken
	}

	return res, nil
}

func validateListBuildersRequest(req *milopb.ListBuildersRequest) error {
	switch {
	case req.PageSize < 0:
		return errors.Reason("page_size can not be negative").Err()
	case req.Project == "":
		return errors.Reason("project is required").Err()
	default:
		return nil
	}
}

func validateListBuildersPageToken(req *milopb.ListBuildersRequest) (*milopb.ListBuildersPageToken, error) {
	if req.PageToken == "" {
		return nil, nil
	}

	token, err := parseListBuildersPageToken(req.PageToken)
	if err != nil {
		return nil, errors.Annotate(err, "unable to parse page_token").Err()
	}

	// Should not have NextBuildbucketPageToken and NextMiloBuilderIndex at the
	// same time.
	if token.NextBuildbucketPageToken != "" && token.NextMiloBuilderIndex != 0 {
		return nil, errors.Reason("invalid page_token").Err()
	}

	// NextBuildbucketPageToken should only be defined when listing all builders
	// in the project.
	if req.Group != "" && token.NextBuildbucketPageToken != "" {
		return nil, errors.Reason("invalid page_token").Err()
	}

	return token, nil
}

func parseListBuildersPageToken(tokenStr string) (token *milopb.ListBuildersPageToken, err error) {
	bytes, err := base64.RawStdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, err
	}
	token = &milopb.ListBuildersPageToken{}
	err = proto.Unmarshal(bytes, token)
	return
}

func serializeListBuildersPageToken(token *milopb.ListBuildersPageToken) (string, error) {
	bytes, err := proto.Marshal(token)
	return base64.RawStdEncoding.EncodeToString(bytes), err
}
