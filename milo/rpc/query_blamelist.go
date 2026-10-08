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
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/buildbucket/protoutil"
	blamelist "go.chromium.org/luci/common/blamelist/chromium"
	"go.chromium.org/luci/common/errors"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

var queryBlamelistPageSize = PageSizeLimiter{
	Max:     1000,
	Default: 100,
}

// recentBuildsPageSize is the number of recent builds to fetch from Buildbucket
// when scanning for the nearest smaller version build.
const recentBuildsPageSize = 100

// QueryBlamelist implements milopb.MiloInternal service
func (s *MiloInternalService) QueryBlamelist(ctx context.Context, req *milopb.QueryBlamelistRequest) (_ *milopb.QueryBlamelistResponse, err error) {
	startRev, pageToken, err := prepareQueryBlamelistRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	allowed, err := projectconfig.IsAllowed(ctx, req.GetBuilder().GetProject())
	if err != nil {
		return nil, err
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in ")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
	}

	pageSize := int(queryBlamelistPageSize.Adjust(req.PageSize))

	targetVer, isVersionedTag := blamelist.ParseVersionedTag(
		req.GitilesCommit.Ref,
	)

	excludeAncestorsOf := pageToken.GetExcludeAncestorsOf()
	if isVersionedTag && pageToken == nil {
		builds, err := s.searchRecentBuilds(
			ctx, req.Builder, recentBuildsPageSize,
		)
		if err != nil {
			return nil, err
		}

		prevBuild, ok := blamelist.FindBaselineBuild(
			targetVer, builds,
		)
		if !ok {
			return &milopb.QueryBlamelistResponse{}, nil
		}
		excludeAncestorsOf = getGitilesCommitID(prevBuild)
		if excludeAncestorsOf == "" {
			return &milopb.QueryBlamelistResponse{}, nil
		}
	}

	// Fetch one more commit to check whether there are more commits in the
	// blamelist.
	gitilesClient, err := s.GetGitilesClient(ctx, req.GitilesCommit.Host, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, fmt.Errorf("get gitiles client: %w", err)
	}
	logReq := &gitiles.LogRequest{
		Project:            req.GitilesCommit.Project,
		Committish:         startRev,
		ExcludeAncestorsOf: excludeAncestorsOf,
		PageSize:           int32(pageSize + 1),
		TreeDiff:           true,
	}
	logRes, err := gitilesClient.Log(ctx, logReq)
	if err != nil {
		// Pass through status codes while preserving message format expected by tests.
		status, ok := status.FromError(err)
		if ok {
			return nil, appstatus.Errorf(status.Code(), "retrieving log from gitiles: %s", status.Message())
		}
		return nil, fmt.Errorf("retrieve gitiles log: %w", err)
	}
	commits := logRes.Log

	blameLength := len(commits)
	if !isVersionedTag {
		blameLength, err = s.findStandardBlameLength(ctx, req, commits)
		if err != nil {
			return nil, err
		}
	}

	nextPageToken := ""
	if blameLength >= pageSize+1 {
		blameLength = pageSize
		nextPageToken, err = serializeQueryBlamelistPageToken(
			&milopb.QueryBlamelistPageToken{
				NextCommitId:       commits[blameLength].Id,
				ExcludeAncestorsOf: excludeAncestorsOf,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	var precedingCommit *gitpb.Commit
	if blameLength < len(commits) {
		precedingCommit = commits[blameLength]
	} else if excludeAncestorsOf != "" {
		precedingCommit = &gitpb.Commit{Id: excludeAncestorsOf}
	}

	return &milopb.QueryBlamelistResponse{
		Commits:         commits[:blameLength],
		NextPageToken:   nextPageToken,
		PrecedingCommit: precedingCommit,
	}, nil
}

// findStandardBlameLength traverses commits and queries Datastore for
// associated builds to find where the blamelist ends.
func (s *MiloInternalService) findStandardBlameLength(ctx context.Context, req *milopb.QueryBlamelistRequest, commits []*gitpb.Commit) (int, error) {
	q := datastore.NewQuery("BuildSummary").Eq("BuilderID", utils.LegacyBuilderIDString(req.Builder))
	blameLength := len(commits)
	m := sync.Mutex{}

	// Find the first other commit that has an associated build and update
	// blameLength.
	err := parallel.WorkPool(8, func(c chan<- func() error) {
		// Skip the first commit, it should always be included in the blamelist.
		for i, commit := range commits[1:] {
			newBlameLength := i + 1 // +1 since we skipped the first one.

			m.Lock()
			foundBuild := newBlameLength >= blameLength
			m.Unlock()

			// We have already found a build before this commit, no point looking
			// further.
			if foundBuild {
				break
			}

			curGC := &buildbucketpb.GitilesCommit{Host: req.GitilesCommit.Host, Project: req.GitilesCommit.Project, Id: commit.Id}
			c <- func() error {
				// Check whether this commit has an associated build.
				hasAssociatedBuild := false
				for build, err := range datastore.RunQuery[*model.BuildSummary](ctx, q.Eq("BlamelistPins", protoutil.GitilesBuildSet(curGC))).Results {
					if err != nil {
						return err
					}
					switch build.Summary.Status {
					case milostatus.InfraFailure, milostatus.Expired, milostatus.Canceled:
					default:
						hasAssociatedBuild = true
						break
					}
					if hasAssociatedBuild {
						break
					}
				}

				if hasAssociatedBuild {
					m.Lock()
					if newBlameLength < blameLength {
						blameLength = newBlameLength
					}
					m.Unlock()
				}
				return nil
			}
		}
	})
	if err != nil {
		return 0, err
	}
	return blameLength, nil
}

func getGitilesCommitID(b *buildbucketpb.Build) string {
	out := b.GetOutput().GetGitilesCommit()
	in := b.GetInput().GetGitilesCommit()
	switch {
	case out.GetId() != "":
		return out.GetId()
	case in.GetId() != "":
		return in.GetId()
	case out.GetRef() != "":
		return out.GetRef()
	default:
		return in.GetRef()
	}
}

// buildsClient returns a Buildbucket BuildsClient configured with the host
// from settings and the given authority.
func (s *MiloInternalService) buildsClient(ctx context.Context, as auth.RPCAuthorityKind) (bbgrpcpb.BuildsClient, error) {
	if s.GetSettings == nil {
		return nil, errors.New("GetSettings is not configured")
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	bbHost := settings.GetBuildbucket().GetHost()
	if bbHost == "" {
		return nil, errors.New("buildbucket host is missing in config")
	}
	if s.GetBuildsClient == nil {
		return nil, errors.New("GetBuildsClient is not configured")
	}
	client, err := s.GetBuildsClient(ctx, bbHost, as)
	if err != nil {
		return nil, fmt.Errorf("get builds client: %w", err)
	}
	return client, nil
}

// searchRecentBuilds queries Buildbucket for recent builds of a builder.
func (s *MiloInternalService) searchRecentBuilds(ctx context.Context, builder *buildbucketpb.BuilderID, pageSize int32) ([]*buildbucketpb.Build, error) {
	client, err := s.buildsClient(ctx, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, err
	}
	res, err := client.SearchBuilds(ctx, &buildbucketpb.SearchBuildsRequest{
		Predicate: &buildbucketpb.BuildPredicate{
			Builder: builder,
		},
		Mask: &buildbucketpb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"id", "builder", "input", "output.gitiles_commit", "status",
				},
			},
		},
		PageSize: pageSize,
	})
	if err != nil {
		if status, ok := status.FromError(err); ok {
			return nil, appstatus.Errorf(
				status.Code(),
				"searching builds from buildbucket: %s",
				status.Message(),
			)
		}
		return nil, fmt.Errorf("search builds: %w", err)
	}
	return res.Builds, nil
}

// prepareQueryBlamelistRequest
//   - validates the request params.
//   - extracts start startRev from page token or gittles commit.
func prepareQueryBlamelistRequest(req *milopb.QueryBlamelistRequest) (startRev string, pageToken *milopb.QueryBlamelistPageToken, err error) {
	switch {
	case req.PageSize < 0:
		return "", nil, errors.New("page_size can not be negative")
	case req.GitilesCommit == nil:
		return "", nil, errors.New("gitiles_commit is required")
	case req.GitilesCommit.Host == "":
		return "", nil, errors.New("gitiles_commit.host is required")
	case !strings.HasSuffix(req.GitilesCommit.Host, ".googlesource.com"):
		return "", nil, errors.New("gitiles_commit.host must be a subdomain of .googlesource.com")
	case req.GitilesCommit.Project == "":
		return "", nil, errors.New("gitiles_commit.project is required")
	case req.GitilesCommit.Id == "" && req.GitilesCommit.Ref == "":
		return "", nil, errors.New("either gitiles_commit.id or gitiles_commit.ref needs to be specified")
	}

	if err := protoutil.ValidateRequiredBuilderID(req.Builder); err != nil {
		return "", nil, errors.Fmt("builder: %w", err)
	}

	if req.PageToken != "" {
		token, err := parseQueryBlamelistPageToken(req.PageToken)
		if err != nil {
			return "", nil, errors.Fmt("unable to parse page_token: %w", err)
		}
		return token.NextCommitId, token, nil
	}

	if req.GitilesCommit.Id == "" {
		return req.GitilesCommit.Ref, nil, nil
	}

	return req.GitilesCommit.Id, nil, nil
}

func parseQueryBlamelistPageToken(tokenStr string) (token *milopb.QueryBlamelistPageToken, err error) {
	bytes, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, err
	}
	token = &milopb.QueryBlamelistPageToken{}
	err = proto.Unmarshal(bytes, token)
	return
}

func serializeQueryBlamelistPageToken(token *milopb.QueryBlamelistPageToken) (string, error) {
	bytes, err := proto.Marshal(token)
	return base64.StdEncoding.EncodeToString(bytes), err
}
