package backend

import (
	"context"

	"go.chromium.org/gae/service/datastore"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/git"
)

type MiloService struct{}

func (s *MiloService) QueryBlamelist(ctx context.Context, req *milopb.QueryBlamelistRequest) (*milopb.QueryBlamelistResponse, error) {
	limit := int(req.PageSize)
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	startCommitID := req.GitilesCommit.Id
	if req.PageToken != "" {
		startCommitID = req.PageToken
	}

	commits, err := git.Get(ctx).Log(ctx, req.GitilesCommit.Host, req.GitilesCommit.Project, startCommitID, &git.LogOptions{Limit: limit + 1, WithFiles: true})
	if err != nil {
		return nil, err
	}

	// TODO(iannucci): This bit could be parallelized, but I think in the typical
	// case this will be fast enough.
	curGC := &buildbucketpb.GitilesCommit{Host: req.GitilesCommit.Host, Project: req.GitilesCommit.Project}
	q := datastore.NewQuery("BuildSummary").Eq("BuilderID", buildbucket.LegacyBuilderIDString(req.Builder))
	endCommitIndex := limit + 1
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
		nextPageToken = commits[endCommitIndex].Id
	}

	return &milopb.QueryBlamelistResponse{
		Commits:       commits[:endCommitIndex],
		NextPageToken: nextPageToken,
	}, nil
}
