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
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

const (
	// Keep the builders in cache for 10 mins to speed up repeated page loads and
	// reduce stress on buildbucket side.
	// But this also means newly added/removed builders would take 10 mins to
	// propagate.
	// Cache duration can be adjusted if needed.
	builderCacheDuration = 10 * time.Minute

	// Refresh the builders cache if the cache TTL falls below this threshold.
	builderCacheRefreshThreshold = builderCacheDuration - time.Minute
)

var listBuildersPageSize = PageSizeLimiter{
	Max:     10000,
	Default: 100,
}

var buildbucketBuildersCache = layered.RegisterCache(layered.Parameters[[]*buildbucketpb.BuilderID]{
	ProcessCacheCapacity: 64,
	GlobalNamespace:      "buildbucket-builders-v4",
	Marshal: func(item []*buildbucketpb.BuilderID) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) ([]*buildbucketpb.BuilderID, error) {
		res := make([]*buildbucketpb.BuilderID, 0)
		err := json.Unmarshal(blob, &res)
		return res, err
	},
})

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

	// Perform ACL check when the project is specified.
	if req.Project != "" {
		allowed, err := projectconfig.IsAllowed(ctx, req.Project)
		if err != nil {
			return nil, err
		}
		if !allowed {
			if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
				return nil, appstatus.Error(codes.Unauthenticated, "not logged in")
			}
			return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
		}
	}

	pageSize := int(listBuildersPageSize.Adjust(req.PageSize))

	if req.Group == "" {
		return s.listProjectBuilders(ctx, req.Project, pageSize, pageToken)
	}
	return s.listGroupBuilders(ctx, req.Project, req.Group, pageSize, pageToken)
}

func (s *MiloInternalService) listProjectBuilders(ctx context.Context, project string, pageSize int, pageToken *milopb.ListBuildersPageToken) (_ *milopb.ListBuildersResponse, err error) {
	res := &milopb.ListBuildersResponse{
		Builders: make([]*buildbucketpb.BuilderItem, 0, pageSize),
	}

	// First, query buildbucket and return all builders defined in the project.
	if pageToken == nil || pageToken.NextBuildbucketBuilderIndex != 0 {
		builders, err := s.GetAllVisibleBuilders(ctx, project)
		if err != nil {
			return nil, err
		}

		pageStart := int(pageToken.GetNextBuildbucketBuilderIndex())
		pageEnd := pageStart + pageSize
		if pageEnd > len(builders) {
			pageEnd = len(builders)
		}

		for _, builder := range builders[pageStart:pageEnd] {
			res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{
				Id: builder,
			})
		}

		// If there are more internal builders, populate `res.NextPageToken` and
		// return.
		if len(builders) > pageEnd {
			nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
				NextBuildbucketBuilderIndex: int32(pageEnd),
			})
			if err != nil {
				return nil, err
			}
			res.NextPageToken = nextPageToken
		}
	}

	if project == "" {
		return res, nil
	}

	// Then, return external builders referenced in the project consoles.
	remaining := pageSize - len(res.Builders)
	if remaining > 0 {
		project := &projectconfig.Project{ID: project}
		if err := datastore.Get(ctx, project); err != nil {
			return nil, err
		}

		externalBuilders := make([]*buildbucketpb.BuilderID, len(project.ExternalBuilderIDs))
		for i, externalBuilderID := range project.ExternalBuilderIDs {
			externalBuilders[i], err = utils.ParseBuilderID(externalBuilderID)
			if err != nil {
				return nil, err
			}
		}

		externalBuilders, err = buildbucket.FilterVisibleBuilders(ctx, externalBuilders, "")
		if err != nil {
			return nil, err
		}

		pageStart := int(pageToken.GetNextMiloBuilderIndex())
		pageEnd := pageStart + remaining
		if pageEnd > len(externalBuilders) {
			pageEnd = len(externalBuilders)
		}

		for _, builder := range externalBuilders[pageStart:pageEnd] {
			res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{
				Id: builder,
			})
		}

		if len(externalBuilders) > pageEnd {
			nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
				NextMiloBuilderIndex: int32(pageEnd),
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
	con := projectconfig.Console{Parent: projKey, ID: group}
	switch err := datastore.Get(ctx, &con); err {
	case nil:
	case datastore.ErrNoSuchEntity:
		return nil, appstatus.Error(codes.NotFound, "group not found")
	default:
		return nil, errors.Fmt("error getting console %s in project %s: %w", group, project, err)
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
		builders[i], err = utils.ParseLegacyBuilderID(bid)
		if err != nil {
			return nil, err
		}
	}

	builders, err = buildbucket.FilterVisibleBuilders(ctx, builders, "")
	if err != nil {
		return nil, err
	}
	pageStart := int(pageToken.GetNextMiloBuilderIndex())
	pageEnd := pageStart + pageSize
	if pageEnd > len(builders) {
		pageEnd = len(builders)
	}

	for _, builder := range builders[pageStart:pageEnd] {
		res.Builders = append(res.Builders, &buildbucketpb.BuilderItem{
			Id: builder,
		})
	}

	if len(builders) > pageEnd {
		nextPageToken, err := serializeListBuildersPageToken(&milopb.ListBuildersPageToken{
			NextMiloBuilderIndex: int32(pageEnd),
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
		return errors.New("page_size can not be negative")
	case req.Project == "" && req.Group != "":
		return errors.New("project is required when group is specified")
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
		return nil, errors.Fmt("unable to parse page_token: %w", err)
	}

	// Should not have NextBuildbucketPageToken and NextMiloBuilderIndex at the
	// same time.
	if token.NextBuildbucketBuilderIndex != 0 && token.NextMiloBuilderIndex != 0 {
		return nil, errors.New("invalid page_token")
	}

	// NextBuildbucketPageToken should only be defined when listing all builders
	// in the project.
	if req.Group != "" && token.NextBuildbucketBuilderIndex != 0 {
		return nil, errors.New("invalid page_token")
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

// GetAllVisibleBuilders returns all cached buildbucket builders. If the cache expired,
// refresh it with Milo's credential.
func (s *MiloInternalService) GetAllVisibleBuilders(c context.Context, project string, opt ...layered.Option) ([]*buildbucketpb.BuilderID, error) {
	settings, err := s.GetSettings(c)
	if err != nil {
		return nil, err
	}
	host := settings.GetBuildbucket().GetHost()
	if host == "" {
		return nil, errors.New("buildbucket host is missing in config")
	}

	builders, err := buildbucketBuildersCache.GetOrCreate(c, host, func() (v []*buildbucketpb.BuilderID, exp time.Duration, err error) {
		start := time.Now()

		buildersClient, err := s.GetBuildersClient(c, host, auth.AsSelf)
		if err != nil {
			return nil, 0, err
		}

		// Get all the Builder IDs from buildbucket.
		bids := make([]*buildbucketpb.BuilderID, 0)
		req := &buildbucketpb.ListBuildersRequest{PageSize: 1000}
		for {
			r, err := buildersClient.ListBuilders(c, req)
			if err != nil {
				return nil, 0, err
			}

			for _, builder := range r.Builders {
				bids = append(bids, builder.Id)
			}

			if r.NextPageToken == "" {
				break
			}
			req.PageToken = r.NextPageToken
		}

		logging.Infof(c, "listing all builders from buildbucket took %v", time.Since(start))

		return bids, builderCacheDuration, nil
	}, opt...)
	if err != nil {
		return nil, err
	}

	return buildbucket.FilterVisibleBuilders(c, builders, project)
}

// UpdateBuilderCache updates the builders cache if the cache TTL falls below
// builderCacheRefreshThreshold.
func (s *MiloInternalService) UpdateBuilderCache(c context.Context) error {
	_, err := s.GetAllVisibleBuilders(c, "", layered.WithMinTTL(builderCacheRefreshThreshold))
	return err
}
