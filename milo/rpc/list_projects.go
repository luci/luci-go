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
	"encoding/base64"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/milo/internal/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

var listProjectsPageSize = PageSizeLimiter{
	Max:     10000,
	Default: 100,
}

// ListProjects implements milopb.MiloInternal service
func (s *MiloInternalService) ListProjects(ctx context.Context, req *milopb.ListProjectsRequest) (_ *milopb.ListProjectsResponse, err error) {
	// Validate request.
	err = validateListProjectsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Validate and get page token.
	pageToken, err := validateListProjectsPageToken(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	pageSize := int(listProjectsPageSize.Adjust(req.PageSize))

	return s.listProjects(ctx, pageSize, pageToken)
}

func (s *MiloInternalService) listProjects(ctx context.Context, pageSize int, pageToken *milopb.ListProjectsPageToken) (_ *milopb.ListProjectsResponse, err error) {
	res := &milopb.ListProjectsResponse{
		Projects: make([]*milopb.ProjectListItem, 0, pageSize),
	}

	projects, err := projectconfig.GetVisibleProjects(ctx)
	if err != nil {
		return nil, errors.Fmt("getting visible projects: %w", err)
	}

	pageStart := int(pageToken.GetNextProjectIndex())
	pageEnd := pageStart + pageSize
	if pageEnd > len(projects) {
		pageEnd = len(projects)
	}

	for _, project := range projects[pageStart:pageEnd] {
		res.Projects = append(res.Projects, &milopb.ProjectListItem{Id: project.ID, LogoUrl: project.LogoURL})
	}

	// If there are more projects, populate `res.NextPageToken`.
	if len(projects) > pageEnd {
		nextPageToken, err := serializeListProjectsPageToken(&milopb.ListProjectsPageToken{
			NextProjectIndex: int32(pageEnd),
		})
		if err != nil {
			return nil, err
		}
		res.NextPageToken = nextPageToken
	}
	return res, nil
}

func validateListProjectsRequest(req *milopb.ListProjectsRequest) error {
	switch {
	case req.PageSize < 0:
		return errors.New("page_size can not be negative")
	default:
		return nil
	}
}

func validateListProjectsPageToken(req *milopb.ListProjectsRequest) (*milopb.ListProjectsPageToken, error) {
	if req.PageToken == "" {
		return nil, nil
	}

	token, err := parseListProjectsPageToken(req.PageToken)
	if err != nil {
		return nil, errors.Fmt("unable to parse page_token: %w", err)
	}

	if token.NextProjectIndex < 0 {
		return nil, errors.New("page_token index cannot be negative")
	}

	return token, nil
}

func parseListProjectsPageToken(tokenStr string) (token *milopb.ListProjectsPageToken, err error) {
	bytes, err := base64.RawStdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, err
	}
	token = &milopb.ListProjectsPageToken{}
	err = proto.Unmarshal(bytes, token)
	return
}

func serializeListProjectsPageToken(token *milopb.ListProjectsPageToken) (string, error) {
	bytes, err := proto.Marshal(token)
	return base64.RawStdEncoding.EncodeToString(bytes), err
}
