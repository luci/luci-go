package backend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/gae/service/datastore"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/git"
	"google.golang.org/protobuf/proto"
)

// MiloInternalService implements milopb.MiloInternal
type MiloInternalService struct{}

// QueryBlamelist implements milopb.MiloInternal service
func (s *MiloInternalService) QueryBlamelist(ctx context.Context, req *milopb.QueryBlamelistRequest) (*milopb.QueryBlamelistResponse, error) {
	startcommitID, hash, err := prepareQueryBlamelistRequest(req)
	if err != nil {
		return nil, errors.Annotate(err, "invalid argument").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	limit := int(req.PageSize)
	// query one more to get the start commit ID for the next page
	commits, err := git.Get(ctx).Log(ctx, req.GitilesCommit.Host, req.GitilesCommit.Project, startcommitID, &git.LogOptions{Limit: limit + 1, WithFiles: true})
	if err != nil {
		return nil, err
	}

	// TODO(iannucci): This bit could be parallelized, but I think in the typical
	// case this will be fast enough.
	curGC := &buildbucketpb.GitilesCommit{Host: req.GitilesCommit.Host, Project: req.GitilesCommit.Project}
	q := datastore.NewQuery("BuildSummary").Eq("BuilderID", buildbucket.LegacyBuilderIDString(req.Builder))
	endCommitIndex := len(commits)
	for i, commit := range commits[1:] { // skip the first commit... it's us!
		curGC.Id = commit.Id
		builds := []*model.BuildSummary{}
		if err = datastore.GetAll(ctx, q.Eq("BuildSet", protoutil.GitilesBuildSet(curGC)), &builds); err != nil {
			return nil, err
		}
		builds = model.FilterBuilds(builds, model.InfraFailure, model.Expired, model.Canceled)
		if len(builds) > 0 {
			endCommitIndex = i + 1 // since we skip the first one
			break
		}
	}

	nextPageToken := ""
	if endCommitIndex == limit+1 {
		endCommitIndex--
		// reserve the last commit as the pivot for the next page.
		nextPageToken, err = serializeQueryBlamelistPageToken(&milopb.QueryBlamelistPageToken{
			ReqHash:      hash,
			NextCommitId: commits[endCommitIndex].Id,
		})
		if err != nil {
			return nil, err
		}
	}

	return &milopb.QueryBlamelistResponse{
		Commits:       commits[:endCommitIndex],
		NextPageToken: nextPageToken,
	}, nil
}

func prepareQueryBlamelistRequest(req *milopb.QueryBlamelistRequest) (startCommitID string, hash string, err error) {
	if req.PageSize < 0 {
		return "", "", errors.Reason("page_size can not be negative").Err()
	} else if req.PageSize == 0 {
		req.PageSize = 100
	} else if req.PageSize > 1000 {
		req.PageSize = 1000
	}

	if req.GitilesCommit == nil {
		return "", "", errors.Reason("gitiles_commit is required").Err()
	}
	if req.GitilesCommit.Host == "" {
		return "", "", errors.Reason("gitiles_commit.host is required").Err()
	}
	if req.GitilesCommit.Project == "" {
		return "", "", errors.Reason("gitiles_commit.project is required").Err()
	}
	if req.GitilesCommit.Id == "" {
		return "", "", errors.Reason("gitiles_commit.id is required").Err()
	}

	hash = hashQueryBlamelistRequest(req)
	if req.PageToken != "" {
		token, err := parseQueryBlamelistPageToken(req.PageToken)
		if err != nil {
			return "", "", errors.Annotate(err, "unable to parse page_token").Err()
		}
		if hash != token.ReqHash {
			return "", "", errors.Reason("invalid token").Err()
		}
		return token.NextCommitId, hash, nil
	}

	return req.GitilesCommit.Id, hash, nil
}

// hashQueryBlamelistRequest produces a hash for QueryBlamelistRequest.
// PageToken and PageSize do not affect the generated hash.
func hashQueryBlamelistRequest(req *milopb.QueryBlamelistRequest) string {
	return fmt.Sprintf("%x", sha256.New().Sum([]byte(fmt.Sprintf("%v|%v", req.GitilesCommit, req.Builder))))
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
